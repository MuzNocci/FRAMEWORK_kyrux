package admin

import (
	"net/http"
	"net/http/httptest"
	"reflect"
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

// ── helpers puros ────────────────────────────────────────────────────────

func TestFieldValuesMapExcluiPK(t *testing.T) {
	type s struct {
		ID   int64
		Nome string
	}
	obj := s{ID: 1, Nome: "A"}
	v := reflect.ValueOf(&obj).Elem()
	fields := []adminField{
		{Column: "id", GoIndex: 0, IsPK: true},
		{Column: "nome", GoIndex: 1},
	}
	got := fieldValuesMap(v, fields)
	if _, ok := got["id"]; ok {
		t.Error("fieldValuesMap não deveria incluir a PK")
	}
	if got["nome"] != "A" {
		t.Errorf("esperava nome=A, recebeu %v", got["nome"])
	}
}

func TestRedactedChangesJSONMascaraHashEEncrypt(t *testing.T) {
	fields := []adminField{
		{Column: "nome"},
		{Column: "senha", IsHash: true},
		{Column: "cpf", IsEncrypt: true},
	}
	changes := map[string]any{"nome": "Maria", "senha": "segredo123", "cpf": "12345678900"}
	raw := redactedChangesJSON(fields, changes)

	if strings.Contains(raw, "segredo123") || strings.Contains(raw, "12345678900") {
		t.Fatalf("valores sensíveis não deveriam aparecer no JSON: %s", raw)
	}
	got := formatChanges(raw)
	if !strings.Contains(got, "senha: ***") || !strings.Contains(got, "cpf: ***") {
		t.Errorf("esperava senha/cpf redigidos como ***, recebeu %q", got)
	}
	if !strings.Contains(got, "nome: Maria") {
		t.Errorf("campo não sensível deveria aparecer normalmente, recebeu %q", got)
	}
}

func TestRedactedChangesJSONVazioParaMapVazio(t *testing.T) {
	if got := redactedChangesJSON(nil, nil); got != "" {
		t.Errorf("esperava string vazia para changes vazio, recebeu %q", got)
	}
}

func TestFormatChangesOrdenaCampos(t *testing.T) {
	got := formatChanges(`{"z_campo":"1","a_campo":"2"}`)
	want := "a_campo: 2, z_campo: 1"
	if got != want {
		t.Errorf("esperava ordem alfabética %q, recebeu %q", want, got)
	}
}

func TestFormatChangesVazioOuInvalido(t *testing.T) {
	if got := formatChanges(""); got != "" {
		t.Errorf("string vazia deveria devolver vazio, recebeu %q", got)
	}
	if got := formatChanges("{não é json"); got != "" {
		t.Errorf("JSON inválido deveria devolver vazio, recebeu %q", got)
	}
}

// ── logHistory / logBulkHistory (gravação real via SQLite) ─────────────────

type historyTestModel struct {
	ID    int64  `kyrux:"pk"`
	Nome  string `kyrux:"size:100"`
	Senha string `kyrux:"hash"`
}

func openHistoryTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := EnsureAllTables(db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	return db
}

func TestLogHistoryGravaEntrada(t *testing.T) {
	resetRegistry()
	Register[historyTestModel]("history-test", "History Test")
	rm, _ := modelBySlug("history-test")
	db := openHistoryTestDB(t)

	logHistory(db, rm, historyActor{UserID: 7, Username: "admin"}, "create", "1",
		map[string]any{"nome": "Produto A", "senha": "hash-real-nao-deveria-aparecer"})

	entry, err := orm.FromDB[HistoryLog](db).First()
	if err != nil {
		t.Fatalf("ler HistoryLog: %v", err)
	}
	if entry.ModelSlug != "history-test" || entry.Action != "create" || entry.RecordPK != "1" {
		t.Errorf("entrada incorreta: %+v", entry)
	}
	if entry.UserID != 7 || entry.Username != "admin" {
		t.Errorf("actor incorreto: %+v", entry)
	}
	if strings.Contains(entry.Changes, "hash-real") {
		t.Errorf("hash não deveria aparecer em Changes: %s", entry.Changes)
	}
}

func TestLogBulkHistoryGravaUmaEntradaPorPK(t *testing.T) {
	resetRegistry()
	Register[historyTestModel]("history-test", "History Test")
	rm, _ := modelBySlug("history-test")
	db := openHistoryTestDB(t)

	logBulkHistory(db, rm, historyActor{UserID: 1, Username: "admin"}, "delete", []string{"1", "2", "3"})

	count, err := orm.FromDB[HistoryLog](db).Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("esperava 3 entradas (uma por pk), recebeu %d", count)
	}
}

// ── integração: Create/Update/Delete registram histórico automaticamente ───

func TestCreateUpdateDeleteRegistramHistorico(t *testing.T) {
	resetRegistry()
	Register[historyTestModel]("history-test", "History Test")
	rm, _ := modelBySlug("history-test")
	db := openHistoryTestDB(t)
	actor := historyActor{UserID: 1, Username: "admin"}

	// create
	createReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("nome=A&senha=segredo123"))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq.ParseForm()
	if err := rm.create(db, createReq, actor); err != nil {
		t.Fatalf("create: %v", err)
	}

	obj, err := orm.FromDB[historyTestModel](db).First()
	if err != nil {
		t.Fatalf("ler registro criado: %v", err)
	}

	entries, err := orm.FromDB[HistoryLog](db).OrderBy("id ASC").All()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Action != "create" {
		t.Fatalf("esperava 1 entrada 'create', recebeu %+v", entries)
	}

	// update
	updateReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("nome=B"))
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateReq.ParseForm()
	pk := formatDisplayValue(reflect.ValueOf(obj.ID), adminField{})
	if err := rm.update(db, pk, updateReq, actor); err != nil {
		t.Fatalf("update: %v", err)
	}

	entries, err = orm.FromDB[HistoryLog](db).OrderBy("id ASC").All()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[1].Action != "update" || entries[1].RecordPK != pk {
		t.Fatalf("esperava 2ª entrada 'update' com pk %s, recebeu %+v", pk, entries)
	}

	// delete
	if err := rm.delete(db, pk, actor); err != nil {
		t.Fatalf("delete: %v", err)
	}
	entries, err = orm.FromDB[HistoryLog](db).OrderBy("id ASC").All()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[2].Action != "delete" || entries[2].RecordPK != pk {
		t.Fatalf("esperava 3ª entrada 'delete' com pk %s, recebeu %+v", pk, entries)
	}
}

func TestHandleBulkActionGravaHistorico(t *testing.T) {
	s, db := newBulkTestSite(t)
	orm.Create(db, &bulkTestProduto{Nome: "A"})
	orm.Create(db, &bulkTestProduto{Nome: "B"})

	ctx := newAuthedBulkCtx("action=delete&ids=1&ids=2")
	s.handleBulkAction(ctx)

	entries, err := orm.FromDB[HistoryLog](db).WhereEq("action", "delete").Count()
	if err != nil {
		t.Fatal(err)
	}
	if entries != 2 {
		t.Errorf("esperava 2 entradas de histórico para a exclusão em lote, recebeu %d", entries)
	}
}

// ── handleHistory ────────────────────────────────────────────────────────

func newHistorySite(t *testing.T) (*site, *database.DB) {
	t.Helper()
	resetRegistry()
	Register[historyTestModel]("ht-visible", "Visível")
	Register[historyTestModel]("ht-hidden", "Escondido", SuperuserOnly())
	db := openHistoryTestDB(t)
	dbm := database.NewManager()
	dbm.AddDB("default", db)
	s := &site{dbm: dbm, store: session.NewStore(time.Hour), basePath: "/admin/"}
	return s, db
}

func newHistoryCtx(query string, admin bool) *router.Context {
	req := httptest.NewRequest(http.MethodGet, "/admin/historico/?"+query, nil)
	ctx := &router.Context{Writer: httptest.NewRecorder(), Request: req}
	ctx.Set(userCtxKey, &auth.User{ID: 1, Username: "staff", IsStaff: true, IsAdmin: admin})
	return ctx
}

func TestHandleHistoryEscondeModelSuperuserOnlyDeNaoAdmin(t *testing.T) {
	s, db := newHistorySite(t)
	rmVisible, _ := modelBySlug("ht-visible")
	rmHidden, _ := modelBySlug("ht-hidden")
	actor := historyActor{UserID: 1, Username: "staff"}
	logHistory(db, rmVisible, actor, "create", "1", map[string]any{"nome": "A"})
	logHistory(db, rmHidden, actor, "create", "1", map[string]any{"nome": "B"})

	ctx := newHistoryCtx("", false)
	s.handleHistory(ctx)

	rec := ctx.Writer.(*httptest.ResponseRecorder)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<td>Visível</td>") {
		t.Error("entrada do model visível deveria aparecer como linha")
	}
	if strings.Contains(body, "Escondido") {
		t.Error("model SuperuserOnly não deveria aparecer em lugar nenhum da página (linha, sidebar ou filtro) para não-admin")
	}
}

func TestHandleHistoryMostraModelSuperuserOnlyParaAdmin(t *testing.T) {
	s, db := newHistorySite(t)
	rmHidden, _ := modelBySlug("ht-hidden")
	logHistory(db, rmHidden, historyActor{UserID: 1, Username: "staff"}, "create", "1", nil)

	ctx := newHistoryCtx("", true)
	s.handleHistory(ctx)

	rec := ctx.Writer.(*httptest.ResponseRecorder)
	if !strings.Contains(rec.Body.String(), "<td>Escondido</td>") {
		t.Error("admin deveria ver a linha de histórico do model SuperuserOnly")
	}
}

func TestHandleHistoryFiltraPorModel(t *testing.T) {
	s, db := newHistorySite(t)
	rmVisible, _ := modelBySlug("ht-visible")
	actor := historyActor{UserID: 1, Username: "staff"}
	logHistory(db, rmVisible, actor, "create", "1", nil)
	logBulkHistory(db, rmVisible, actor, "delete", []string{"2", "3"})

	ctx := newHistoryCtx("model=ht-visible", true)
	s.handleHistory(ctx)

	rec := ctx.Writer.(*httptest.ResponseRecorder)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d", rec.Code)
	}
	body := rec.Body.String()
	if got := strings.Count(body, "<td>Visível</td>"); got != 3 {
		t.Errorf("esperava 3 linhas (1 create + 2 delete), recebeu %d; corpo: %s", got, body)
	}
}
