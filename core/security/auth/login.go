package auth

import (
	"errors"
	"kyrux/core/database"
	"kyrux/core/orm"
	"kyrux/core/security/session"
	"net/http"
	"reflect"
	"strings"
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

// Login autentica o usuário pelo campo configurado (username ou email) e senha.
// Em caso de sucesso cria a sessão, grava o cookie e retorna a sessão.
// Os erros possíveis são ErrUserNotFound, ErrInactiveUser e ErrWrongPassword.
func Login(db *database.DB, store *session.Store, w http.ResponseWriter, r *http.Request, loginValue, password string) (*session.Session, error) {
	if !dbEnabled {
		return nil, ErrAuthDisabled
	}
	user, err := orm.FromDB[User](db).Where(loginColumn+" = ?", loginValue).First()
	if err != nil {
		return nil, ErrUserNotFound
	}
	if !user.IsActive {
		return nil, ErrInactiveUser
	}
	if !user.CheckPassword(password) {
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
	sess.Values["user_id"] = user.ID
	sess.Values["username"] = user.Username

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
	id, ok := sess.Values["user_id"].(int64)
	if !ok {
		return nil, ErrUserNotFound
	}
	return orm.FromDB[User](db).Where("id = ?", id).First()
}

// NextURL lê o parâmetro ?next= do request e valida que é uma URL relativa,
// evitando open redirect. Retorna fallback se o parâmetro estiver ausente ou inválido.
func NextURL(r *http.Request, fallback string) string {
	next := r.URL.Query().Get("next")
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return fallback
	}
	return next
}
