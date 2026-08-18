package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kyrux/core/database"
	"kyrux/core/orm"
	"kyrux/core/router"
	"kyrux/core/security/auth"
	"kyrux/core/security/session"

	_ "modernc.org/sqlite"
)

// ── BulkAction (registro) ───────────────────────────────────────────────────

type bulkTestProduto struct {
	ID    int64  `kyrux:"pk"`
	Nome  string `kyrux:"size:100"`
	Ativo bool
}

func TestBulkActionRegistraECompoeComDelete(t *testing.T) {
	resetRegistry()
	Register[bulkTestProduto]("bulk-test", "Bulk Test",
		BulkAction("ativar", "Ativar selecionados", func(db *database.DB, pks []any) error { return nil }))
	rm, _ := modelBySlug("bulk-test")
	if len(rm.bulkActions) != 1 || rm.bulkActions[0].Name != "ativar" {
		t.Fatalf("BulkAction não registrada corretamente: %+v", rm.bulkActions)
	}
}

func TestBulkActionNomeDeleteEhReservado(t *testing.T) {
	resetRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic ao registrar BulkAction com name \"delete\"")
		}
	}()
	Register[bulkTestProduto]("bulk-test", "Bulk Test",
		BulkAction("delete", "Excluir de novo", func(db *database.DB, pks []any) error { return nil }))
}

func TestBulkActionNomeDuplicadoPanica(t *testing.T) {
	resetRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para BulkAction duplicada")
		}
	}()
	Register[bulkTestProduto]("bulk-test", "Bulk Test",
		BulkAction("ativar", "Ativar", func(db *database.DB, pks []any) error { return nil }),
		BulkAction("ativar", "Ativar de novo", func(db *database.DB, pks []any) error { return nil }),
	)
}

// ── handleBulkAction (end-to-end sobre SQLite real) ─────────────────────────

func newBulkTestSite(t *testing.T) (*site, *database.DB) {
	t.Helper()
	resetRegistry()
	Register[bulkTestProduto]("bt-produtos", "Produtos Teste",
		BulkAction("ativar", "Ativar selecionados", func(db *database.DB, pks []any) error {
			return orm.FromDB[bulkTestProduto](db).WhereIn("id", pks...).Update(map[string]any{"ativo": true})
		}),
	)
	db, err := database.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := EnsureAllTables(db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	dbm := database.NewManager()
	dbm.AddDB("default", db)
	s := &site{dbm: dbm, store: session.NewStore(time.Hour), basePath: "/admin/"}
	return s, db
}

// newAuthedBulkCtx monta um *router.Context com um POST de form-urlencoded
// já autenticado (ctxUser) — equivalente ao que requireStaff monta antes de
// chamar o handler; os testes de handleBulkAction chamam o handler direto,
// então não passam pelo guard nem pelo CSRF global (cobertos em
// staffcheck_test.go e pela verificação manual da suíte de review).
func newAuthedBulkCtx(body string) *router.Context {
	req := httptest.NewRequest(http.MethodPost, "/admin/bt-produtos/lote/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := &router.Context{Writer: httptest.NewRecorder(), Request: req}
	ctx.SetParam("slug", "bt-produtos")
	ctx.Set(userCtxKey, &auth.User{ID: 1, Username: "admin", IsAdmin: true})
	return ctx
}

func TestHandleBulkActionExcluiSelecionados(t *testing.T) {
	s, db := newBulkTestSite(t)
	orm.Create(db, &bulkTestProduto{Nome: "A"})
	orm.Create(db, &bulkTestProduto{Nome: "B"})
	orm.Create(db, &bulkTestProduto{Nome: "C"})

	ctx := newAuthedBulkCtx("action=delete&ids=1&ids=2")
	s.handleBulkAction(ctx)

	rec := ctx.Writer.(*httptest.ResponseRecorder)
	if rec.Code != http.StatusFound {
		t.Fatalf("esperava 302, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	count, err := orm.FromDB[bulkTestProduto](db).Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("esperava 1 registro restante após excluir 2 de 3, recebeu %d", count)
	}
}

func TestHandleBulkActionCustomizada(t *testing.T) {
	s, db := newBulkTestSite(t)
	orm.Create(db, &bulkTestProduto{Nome: "A", Ativo: false})
	orm.Create(db, &bulkTestProduto{Nome: "B", Ativo: false})

	ctx := newAuthedBulkCtx("action=ativar&ids=1&ids=2")
	s.handleBulkAction(ctx)

	ativos, err := orm.FromDB[bulkTestProduto](db).WhereEq("ativo", true).Count()
	if err != nil {
		t.Fatal(err)
	}
	if ativos != 2 {
		t.Errorf("esperava 2 registros ativados, recebeu %d", ativos)
	}
}

func TestHandleBulkActionSemIDsRedirecionaSemAplicarNada(t *testing.T) {
	s, db := newBulkTestSite(t)
	orm.Create(db, &bulkTestProduto{Nome: "A"})

	ctx := newAuthedBulkCtx("action=delete")
	s.handleBulkAction(ctx)

	rec := ctx.Writer.(*httptest.ResponseRecorder)
	if rec.Code != http.StatusFound {
		t.Fatalf("esperava 302, recebeu %d", rec.Code)
	}
	count, _ := orm.FromDB[bulkTestProduto](db).Count()
	if count != 1 {
		t.Errorf("sem ids selecionados, nada deveria ter sido excluído; restam %d", count)
	}
}

func TestHandleBulkActionDesconhecidaRetorna400(t *testing.T) {
	s, _ := newBulkTestSite(t)
	ctx := newAuthedBulkCtx("action=hackear&ids=1")
	s.handleBulkAction(ctx)

	rec := ctx.Writer.(*httptest.ResponseRecorder)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("esperava 400 para ação desconhecida, recebeu %d", rec.Code)
	}
}

func TestHandleBulkActionPKInvalidaRetorna400(t *testing.T) {
	s, _ := newBulkTestSite(t)
	ctx := newAuthedBulkCtx("action=delete&ids=nao-numero")
	s.handleBulkAction(ctx)

	rec := ctx.Writer.(*httptest.ResponseRecorder)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("esperava 400 para PK inválida, recebeu %d", rec.Code)
	}
}

func TestHandleBulkActionSlugInexistenteRetorna404(t *testing.T) {
	s, _ := newBulkTestSite(t)
	req := httptest.NewRequest(http.MethodPost, "/admin/nao-existe/lote/", strings.NewReader("action=delete&ids=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := &router.Context{Writer: httptest.NewRecorder(), Request: req}
	ctx.SetParam("slug", "nao-existe")
	ctx.Set(userCtxKey, &auth.User{ID: 1, Username: "admin", IsAdmin: true})

	s.handleBulkAction(ctx)

	rec := ctx.Writer.(*httptest.ResponseRecorder)
	if rec.Code != http.StatusNotFound {
		t.Errorf("esperava 404 para slug inexistente, recebeu %d", rec.Code)
	}
}
