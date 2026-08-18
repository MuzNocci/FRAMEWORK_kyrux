package admin

// Prova o comportamento documentado do cache de IsStaff/IsAdmin em
// requireStaff: dentro do TTL, uma revogação feita direto no banco (fora
// do app) ainda não tem efeito (cache); depois do TTL expirar, a próxima
// requisição reconsulta o banco e barra o acesso — a janela é limitada e
// previsível (staffCheckTTL), não "revogação nunca" nem "sempre imediata".

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kyrux/core/database"
	"kyrux/core/orm"
	"kyrux/core/router"
	"kyrux/core/security/auth"
	"kyrux/core/security/session"

	_ "modernc.org/sqlite"
)

func TestRequireStaffCacheiaDentroDoTTLERevalidaDepois(t *testing.T) {
	auth.SetDBEnabled(true)
	defer auth.SetDBEnabled(false)

	db, err := database.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	defer db.Close()
	if err := orm.EnsureSQLiteTable[auth.User](db); err != nil {
		t.Fatalf("criar tabela users: %v", err)
	}

	user := &auth.User{Username: "staffuser", IsStaff: true, IsActive: true}
	if err := user.SetPassword("senha12345678"); err != nil {
		t.Fatalf("hash senha: %v", err)
	}
	if err := orm.Create(db, user); err != nil {
		t.Fatalf("criar usuário: %v", err)
	}

	dbm := database.NewManager()
	dbm.AddDB("default", db)

	store := session.NewStore(time.Hour)
	sess, err := store.New()
	if err != nil {
		t.Fatalf("criar sessão: %v", err)
	}
	sess.Set("user_id", user.ID)

	// Encolhe o TTL pra não esperar 5s de verdade — restaurado ao final.
	orig := staffCheckTTL
	staffCheckTTL = 50 * time.Millisecond
	defer func() { staffCheckTTL = orig }()

	guard := requireStaff(dbm, store, "/admin/")
	var reached bool
	handler := guard(func(ctx *router.Context) { reached = true })

	doRequest := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
		req.AddCookie(&http.Cookie{Name: session.CookieName(), Value: sess.ID})
		rec := httptest.NewRecorder()
		reached = false
		handler(&router.Context{Writer: rec, Request: req})
		return rec
	}

	// 1) primeira chamada: SELECT real, staff -> acesso concedido, e cacheado.
	doRequest()
	if !reached {
		t.Fatal("esperava acesso concedido na primeira chamada (usuário é staff)")
	}

	// 2) revoga IsStaff direto no banco — fora do app, simula um DBA ou
	// outro processo mexendo na tabela sem passar pela sessão atual.
	if err := orm.FromDB[auth.User](db).Where("id = ?", user.ID).Update(map[string]any{"is_staff": false}); err != nil {
		t.Fatalf("revogar: %v", err)
	}

	// 3) ainda dentro do TTL: cache hit, continua permitindo — é a troca
	// documentada (revogação não é mais instantânea).
	doRequest()
	if !reached {
		t.Fatal("esperava acesso ainda concedido dentro do TTL (cache), mesmo após revogação")
	}

	// 4) espera o TTL expirar.
	time.Sleep(staffCheckTTL + 20*time.Millisecond)

	// 5) agora reconsulta o banco de verdade e barra.
	rec := doRequest()
	if reached {
		t.Fatal("esperava acesso NEGADO depois do TTL expirar (revogação deveria valer)")
	}
	if rec.Code != http.StatusFound {
		t.Errorf("esperava redirect (302) pro login, recebeu %d", rec.Code)
	}
}

func TestRequireStaffSemSessaoRedireciona(t *testing.T) {
	auth.SetDBEnabled(true)
	defer auth.SetDBEnabled(false)

	db, err := database.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	defer db.Close()
	if err := orm.EnsureSQLiteTable[auth.User](db); err != nil {
		t.Fatalf("criar tabela users: %v", err)
	}

	dbm := database.NewManager()
	dbm.AddDB("default", db)
	store := session.NewStore(time.Hour)

	guard := requireStaff(dbm, store, "/admin/")
	var reached bool
	handler := guard(func(ctx *router.Context) { reached = true })

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rec := httptest.NewRecorder()
	handler(&router.Context{Writer: rec, Request: req})

	if reached {
		t.Fatal("esperava acesso negado sem sessão nenhuma")
	}
	if rec.Code != http.StatusFound {
		t.Errorf("esperava redirect (302) pro login, recebeu %d", rec.Code)
	}
}

// TestRequireStaffCacheMantemNomeCompleto prova que o cache de staffCheck
// (dentro do TTL) não derruba FirstName/LastName do usuário — sem isso,
// b.User (nome ao lado do botão Sair) cairia pro username a cada
// requisição em cache, mesmo com nome completo cadastrado.
func TestRequireStaffCacheMantemNomeCompleto(t *testing.T) {
	auth.SetDBEnabled(true)
	defer auth.SetDBEnabled(false)

	db, err := database.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	defer db.Close()
	if err := orm.EnsureSQLiteTable[auth.User](db); err != nil {
		t.Fatalf("criar tabela users: %v", err)
	}

	user := &auth.User{Username: "staffuser", FirstName: "Ana", LastName: "Silva", IsStaff: true, IsActive: true}
	if err := user.SetPassword("senha12345678"); err != nil {
		t.Fatalf("hash senha: %v", err)
	}
	if err := orm.Create(db, user); err != nil {
		t.Fatalf("criar usuário: %v", err)
	}

	dbm := database.NewManager()
	dbm.AddDB("default", db)
	store := session.NewStore(time.Hour)
	sess, err := store.New()
	if err != nil {
		t.Fatalf("criar sessão: %v", err)
	}
	sess.Set("user_id", user.ID)

	guard := requireStaff(dbm, store, "/admin/")
	var got *auth.User
	handler := guard(func(ctx *router.Context) { got = ctxUser(ctx) })

	doRequest := func() {
		req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
		req.AddCookie(&http.Cookie{Name: session.CookieName(), Value: sess.ID})
		handler(&router.Context{Writer: httptest.NewRecorder(), Request: req})
	}

	doRequest() // 1ª chamada: SELECT real, popula o cache
	if got.FullName() != "Ana Silva" {
		t.Fatalf("1ª chamada (sem cache): esperava %q, recebeu %q", "Ana Silva", got.FullName())
	}

	doRequest() // 2ª chamada: dentro do TTL, vem do staffCheck cacheado
	if got.FullName() != "Ana Silva" {
		t.Errorf("2ª chamada (cache hit): esperava %q, recebeu %q — FirstName/LastName não sobreviveram ao cache", "Ana Silva", got.FullName())
	}
}
