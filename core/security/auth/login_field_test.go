package auth

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"kyrux/core/database"
	"kyrux/core/orm"
	"kyrux/core/security/crypton"
	"kyrux/core/security/session"
)

// usaCampoDeLogin troca o campo de login só durante o teste.
func usaCampoDeLogin(t *testing.T, field string) {
	t.Helper()
	col, name := loginColumn, loginFieldName
	t.Cleanup(func() { loginColumn, loginFieldName = col, name })
	if err := SetLoginField(field); err != nil {
		t.Fatalf("SetLoginField(%q): %v", field, err)
	}
}

func TestSetLoginField(t *testing.T) {
	usaCampoDeLogin(t, "")
	if loginFieldName != "Username" {
		t.Fatalf("vazio deveria manter o padrão da tag, veio %q", loginFieldName)
	}
	usaCampoDeLogin(t, " EMAIL ")
	if loginColumn != "email" || LoginFieldName() != "Email" {
		t.Fatalf("email: coluna %q, campo %q", loginColumn, LoginFieldName())
	}
	usaCampoDeLogin(t, "username")
	if loginColumn != "username" || LoginFieldName() != "Username" {
		t.Fatalf("username: coluna %q, campo %q", loginColumn, LoginFieldName())
	}
	if err := SetLoginField("telefone"); err == nil {
		t.Fatal("valor inválido deveria dar erro")
	}
	if loginFieldName != "Username" {
		t.Fatal("valor inválido não pode mudar o campo em uso")
	}
}

// bancoPostgres cria um schema descartável no PostgreSQL de KYRUX_TEST_POSTGRES_DSN,
// com a mesma tabela users das migrations. Sem a variável, o teste é pulado.
func bancoPostgres(t *testing.T) *database.DB {
	t.Helper()
	dsn := os.Getenv("KYRUX_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KYRUX_TEST_POSTGRES_DSN não definido")
	}
	crypton.SetPepper("teste-pepper-nao-usar-em-producao")
	admin, err := database.Open("postgres", dsn)
	if err != nil {
		t.Skipf("postgres indisponível: %v", err)
	}
	schema := fmt.Sprintf("kyrux_auth_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		admin.Close()
		t.Fatalf("criar schema: %v", err)
	}
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := database.Open("postgres", u.String())
	if err != nil {
		t.Fatalf("abrir schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE users (
		id BIGSERIAL PRIMARY KEY,
		uuid VARCHAR(36) NOT NULL DEFAULT '',
		first_name VARCHAR(150) NOT NULL DEFAULT '',
		last_name VARCHAR(150) NOT NULL DEFAULT '',
		username VARCHAR(150) NOT NULL DEFAULT '' UNIQUE,
		email VARCHAR(254) UNIQUE,
		password VARCHAR(128) NOT NULL DEFAULT '',
		user_group VARCHAR(100) NOT NULL DEFAULT '',
		is_admin BOOLEAN NOT NULL DEFAULT FALSE,
		is_staff BOOLEAN NOT NULL DEFAULT FALSE,
		is_active BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		t.Fatalf("criar tabela: %v", err)
	}
	SetDBEnabled(true)
	t.Cleanup(func() { SetDBEnabled(false) })
	return db
}

func criaConta(t *testing.T, db *database.DB, username, email, senha string) {
	t.Helper()
	u := &User{UUID: generateUUID(), Username: username, Email: &email, IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := u.SetPassword(senha); err != nil {
		t.Fatal(err)
	}
	if err := orm.Create(db, u); err != nil {
		t.Fatalf("criar conta: %v", err)
	}
}

func tentaEntrar(db *database.DB, login, senha string) error {
	store := session.NewStore(time.Minute)
	r := httptest.NewRequest("POST", "/entrar/", strings.NewReader(""))
	r.RemoteAddr = fmt.Sprintf("10.0.0.1:%d", time.Now().UnixNano()%60000)
	_, err := Login(db, store, httptest.NewRecorder(), r, login, senha)
	return err
}

func TestLoginPorUsuario(t *testing.T) {
	db := bancoPostgres(t)
	usaCampoDeLogin(t, "username")
	criaConta(t, db, "ana", "ana@exemplo.com", "senha-de-teste-1")

	if err := tentaEntrar(db, "ana", "senha-de-teste-1"); err != nil {
		t.Errorf("usuário deveria entrar: %v", err)
	}
	if err := tentaEntrar(db, "ana@exemplo.com", "senha-de-teste-1"); err == nil {
		t.Error("e-mail não pode entrar no modo username")
	}
	if err := tentaEntrar(db, "ana", "senha-errada"); err == nil {
		t.Error("senha errada não pode entrar")
	}
}

func TestLoginPorEmail(t *testing.T) {
	db := bancoPostgres(t)
	usaCampoDeLogin(t, "email")
	criaConta(t, db, "bia", "Bia@Exemplo.com", "senha-de-teste-2")

	for _, login := range []string{"bia@exemplo.com", "BIA@EXEMPLO.COM", "  Bia@Exemplo.com "} {
		if err := tentaEntrar(db, login, "senha-de-teste-2"); err != nil {
			t.Errorf("%q deveria entrar: %v", login, err)
		}
	}
	if err := tentaEntrar(db, "bia", "senha-de-teste-2"); err == nil {
		t.Error("usuário não pode entrar no modo email")
	}
}

// Duas contas criadas por e-mail não podem colidir na coluna username (única).
func TestEnsureSuperuserPorEmail(t *testing.T) {
	db := bancoPostgres(t)
	usaCampoDeLogin(t, "email")

	for _, email := range []string{"Um@Exemplo.com", "dois@exemplo.com"} {
		criado, err := EnsureSuperuser(db, email, "senha-provisoria")
		if err != nil || !criado {
			t.Fatalf("%s: criado=%v err=%v", email, criado, err)
		}
	}
	criado, err := EnsureSuperuser(db, "UM@exemplo.com", "outra-senha-qualquer")
	if err != nil || criado {
		t.Fatalf("mesmo e-mail não deveria recriar: criado=%v err=%v", criado, err)
	}
	if err := tentaEntrar(db, "um@exemplo.com", "senha-provisoria"); err != nil {
		t.Errorf("superusuário deveria entrar pelo e-mail: %v", err)
	}
	if _, err := EnsureSuperuser(db, "admin", "senha-provisoria"); err == nil {
		t.Error("valor sem @ deveria ser recusado no modo email")
	}
}

func TestEnsureSuperuserPorUsuario(t *testing.T) {
	db := bancoPostgres(t)
	usaCampoDeLogin(t, "username")

	criado, err := EnsureSuperuser(db, "chefe", "senha-provisoria")
	if err != nil || !criado {
		t.Fatalf("criado=%v err=%v", criado, err)
	}
	u, err := orm.FromDB[User](db).Where("username = ?", "chefe").First()
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != nil {
		t.Errorf("e-mail deveria ficar vazio, veio %q", *u.Email)
	}
}
