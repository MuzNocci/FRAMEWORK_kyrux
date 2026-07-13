package auth

import (
	"errors"
	"kyrux/core/database"
	"kyrux/core/orm"
	"kyrux/core/security/crypton"
	"kyrux/core/security/session"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"unicode"
)

// ErrAuthDisabled é retornado quando DB_ENABLED=false — nenhuma operação de auth é executada.
var ErrAuthDisabled = errors.New("auth: banco de dados desabilitado")

// dbEnabled controla se o sistema de auth está ativo.
// Definido pelo bootstrap via SetDBEnabled — nunca alterar diretamente.
var dbEnabled bool

// SetDBEnabled ativa ou desativa o sistema de auth conforme DB_ENABLED no .env.
// Deve ser chamado uma única vez no bootstrap, antes de qualquer request.
func SetDBEnabled(enabled bool) { dbEnabled = enabled }

// IsDBEnabled informa se o sistema de auth está ativo.
func IsDBEnabled() bool { return dbEnabled }

// loginColumn é a coluna SQL usada para buscar o usuário no Login.
// loginFieldName é o nome do campo Go (ex: "Username", "Email") marcado com login.
// Ambos são derivados das tags kyrux do struct User na inicialização do pacote.
// Não há configuração de runtime — alterar exige mudança no model + nova migration.
var (
	loginColumn    = detectLoginColumn()
	loginFieldName = detectLoginFieldName()
)

// LoginFieldName retorna o nome do campo Go (não a coluna SQL) marcado com login no model User.
// Usado pelo CLI para determinar qual campo é obrigatório na criação de usuários.
func LoginFieldName() string { return loginFieldName }

func detectLoginFieldName() string {
	t := reflect.TypeOf(User{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		for _, p := range strings.Split(f.Tag.Get("kyrux"), ",") {
			if strings.TrimSpace(p) == "login" {
				return f.Name
			}
		}
	}
	return "Username"
}

// detectLoginColumn lê as tags kyrux de User e retorna a coluna SQL do campo marcado
// com "login". Fallback: "username".
func detectLoginColumn() string {
	t := reflect.TypeOf(User{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		parts := strings.Split(f.Tag.Get("kyrux"), ",")
		isLogin := false
		col := authToSnake(f.Name)
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "login" {
				isLogin = true
			}
			if strings.HasPrefix(p, "column:") {
				col = strings.TrimPrefix(p, "column:")
			}
		}
		if isLogin {
			return col
		}
	}
	return "username"
}

// authToSnake converte CamelCase → snake_case sem importar o pacote orm.
func authToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// dummyHash equaliza o tempo de resposta quando o usuário não existe.
// Sem ele, "não encontrado" (~1 ms de query) vs "encontrado + Argon2" (~50 ms)
// revela por timing quais logins existem (user enumeration).
// Lazy: o primeiro uso gera o hash — evita custo de startup no CLI.
var (
	dummyHashOnce sync.Once
	dummyHash     string
)

// equalizeTiming roda uma verificação Argon2 descartável com o mesmo custo
// de um CheckPassword real. O resultado é ignorado de propósito.
func equalizeTiming(password string) {
	dummyHashOnce.Do(func() {
		if h, err := crypton.HashPassword("kyrux-timing-equalizer"); err == nil {
			dummyHash = h
		}
	})
	if dummyHash != "" {
		crypton.CheckPassword(password, dummyHash)
	}
}

// Login autentica o usuário pelo campo configurado (username ou email) e senha.
// Em caso de sucesso cria a sessão, grava o cookie e retorna a sessão.
// Os erros possíveis são ErrUserNotFound, ErrInactiveUser e ErrWrongPassword —
// na resposta ao cliente, prefira uma mensagem única ("credenciais inválidas")
// e reserve o erro específico para logs.
func Login(db *database.DB, store *session.Store, w http.ResponseWriter, r *http.Request, loginValue, password string) (*session.Session, error) {
	if !dbEnabled {
		return nil, ErrAuthDisabled
	}
	user, err := orm.FromDB[User](db).Where(loginColumn+" = ?", loginValue).First()
	if err != nil {
		equalizeTiming(password)
		return nil, ErrUserNotFound
	}
	// Verifica a senha antes de checar IsActive para que usuário inativo
	// e senha errada custem o mesmo tempo (sem vazar o estado da conta).
	passwordOK := user.CheckPassword(password)
	if !user.IsActive {
		return nil, ErrInactiveUser
	}
	if !passwordOK {
		return nil, ErrWrongPassword
	}

	// Apagar sessão anônima existente antes de criar a autenticada (session fixation).
	if old, ok := session.FromRequest(r, store); ok {
		store.Delete(old.ID)
	}

	sess, err := store.New()
	if err != nil {
		return nil, err
	}
	sess.Set("user_id", user.ID)
	sess.Set("username", user.Username)

	session.SetCookie(w, sess.ID, r.TLS != nil)
	return sess, nil
}

// Logout encerra a sessão ativa: remove do store e expira o cookie no cliente.
func Logout(store *session.Store, r *http.Request, w http.ResponseWriter) {
	if sess, ok := session.FromRequest(r, store); ok {
		store.Delete(sess.ID)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     session.CookieName(),
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})
}

// GetUser retorna o User associado à sessão ativa do request.
// Retorna ErrUserNotFound se não houver sessão válida ou o usuário não existir.
func GetUser(db *database.DB, store *session.Store, r *http.Request) (*User, error) {
	if !dbEnabled {
		return nil, ErrAuthDisabled
	}
	sess, ok := session.FromRequest(r, store)
	if !ok {
		return nil, ErrUserNotFound
	}
	v, ok := sess.Get("user_id")
	if !ok {
		return nil, ErrUserNotFound
	}
	id, ok := v.(int64)
	if !ok {
		return nil, ErrUserNotFound
	}
	return orm.FromDB[User](db).Where("id = ?", id).First()
}

// NextURL lê o parâmetro ?next= do request e valida que é uma URL relativa,
// evitando open redirect. Retorna fallback se o parâmetro estiver ausente ou inválido.
func NextURL(r *http.Request, fallback string) string {
	next := r.URL.Query().Get("next")
	// "/\..." também é bloqueado: browsers normalizam \ para /, o que
	// transformaria "/\evil.com" em "//evil.com" (redirect externo).
	if next == "" || !strings.HasPrefix(next, "/") ||
		strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return fallback
	}
	return next
}
