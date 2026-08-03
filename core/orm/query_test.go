package orm

import (
	"strings"
	"testing"
)

type produtoTeste struct {
	ID        int64  `kyrux:"pk"`
	Nome      string `kyrux:"size:100"`
	Preco     float64
	UpdatedAt string `kyrux:"column:updated_at,autonow"`
}

func newTestQuery(driver string) *Query[produtoTeste] {
	return &Query[produtoTeste]{
		driver: driver,
		meta:   metaOf[produtoTeste](),
	}
}

// TestBuildSelectLimitPlaceholder garante que LIMIT/OFFSET viram placeholders:
// o SQL fica idêntico entre páginas e o cache de prepared statements funciona.
func TestBuildSelectLimitPlaceholder(t *testing.T) {
	q := newTestQuery("postgres").Where("nome = ?", "x").Limit(20).Offset(40)
	sqlStr, args := q.buildSelect(0)

	want := "SELECT * FROM produto_testes WHERE (nome = $1) LIMIT $2 OFFSET $3"
	if sqlStr != want {
		t.Errorf("sql:\n got: %s\nwant: %s", sqlStr, want)
	}
	if len(args) != 3 || args[1] != 20 || args[2] != 40 {
		t.Errorf("args: esperava [x 20 40], recebeu %v", args)
	}

	// Páginas diferentes devem gerar o MESMO SQL (só os args mudam).
	q2 := newTestQuery("postgres").Where("nome = ?", "x").Limit(20).Offset(80)
	sql2, _ := q2.buildSelect(0)
	if sql2 != sqlStr {
		t.Errorf("SQL deveria ser idêntico entre páginas:\n%s\n%s", sqlStr, sql2)
	}
}

// TestBuildSelectPageIdenticalSQL cobre o caminho usado por Paginate.
func TestBuildSelectPageIdenticalSQL(t *testing.T) {
	a, argsA := newTestQuery("mysql").buildSelectPage(10, 0)
	b, argsB := newTestQuery("mysql").buildSelectPage(10, 50)
	if a != b {
		t.Errorf("SQL de páginas diferentes deveria ser idêntico:\n%s\n%s", a, b)
	}
	if argsA[len(argsA)-1] != 0 || argsB[len(argsB)-1] != 50 {
		t.Errorf("offsets errados: %v / %v", argsA, argsB)
	}
}

// TestBuildSelectSemLimite garante que sem Limit/Offset nada é anexado.
func TestBuildSelectSemLimite(t *testing.T) {
	sqlStr, args := newTestQuery("sqlite").buildSelect(0)
	want := "SELECT * FROM produto_testes"
	if sqlStr != want {
		t.Errorf("sql: got %q, want %q", sqlStr, want)
	}
	if len(args) != 0 {
		t.Errorf("args: esperava vazio, recebeu %v", args)
	}
}

func TestOrWhere(t *testing.T) {
	q := newTestQuery("sqlite").Where("tipo = ?", "a").OrWhere("tipo = ?", "b").Where("ativo = ?", true)
	sqlStr, _ := q.buildSelect(0)
	want := "SELECT * FROM produto_testes WHERE (tipo = ?) OR (tipo = ?) AND (ativo = ?)"
	if sqlStr != want {
		t.Errorf("sql:\n got: %s\nwant: %s", sqlStr, want)
	}
}

func TestWhereInExpandeSlice(t *testing.T) {
	ids := []int64{10, 20, 30}
	q := newTestQuery("postgres").WhereIn("id", ids)
	sqlStr, args := q.buildSelect(0)
	want := "SELECT * FROM produto_testes WHERE (id IN ($1, $2, $3))"
	if sqlStr != want {
		t.Errorf("sql:\n got: %s\nwant: %s", sqlStr, want)
	}
	if len(args) != 3 || args[0] != int64(10) || args[2] != int64(30) {
		t.Errorf("args: esperava [10 20 30], recebeu %v", args)
	}
}

func TestWhereInVazioNaoRetornaNada(t *testing.T) {
	q := newTestQuery("sqlite").WhereIn("id")
	sqlStr, _ := q.buildSelect(0)
	if !strings.Contains(sqlStr, "(1 = 0)") {
		t.Errorf("IN vazio deveria virar condição impossível, recebeu: %s", sqlStr)
	}
}

func TestWhereInRejeitaColunaInvalida(t *testing.T) {
	q := newTestQuery("sqlite").WhereIn("id; DROP TABLE x", 1)
	if q.err == nil {
		t.Fatal("coluna com injeção deveria gerar erro")
	}
}

func TestOrderByMultiplo(t *testing.T) {
	q := newTestQuery("sqlite").OrderBy("preco DESC", "nome ASC", "id")
	sqlStr, _ := q.buildSelect(0)
	want := "SELECT * FROM produto_testes ORDER BY preco DESC, nome ASC, id"
	if sqlStr != want {
		t.Errorf("sql:\n got: %s\nwant: %s", sqlStr, want)
	}

	// Cada termo é validado individualmente.
	bad := newTestQuery("sqlite").OrderBy("preco; DROP TABLE x")
	if bad.err == nil {
		t.Fatal("OrderBy com injeção deveria gerar erro")
	}
}

func TestDistinct(t *testing.T) {
	q := newTestQuery("sqlite").Select("nome").Distinct()
	sqlStr, _ := q.buildSelect(0)
	want := "SELECT DISTINCT nome FROM produto_testes"
	if sqlStr != want {
		t.Errorf("sql: got %q, want %q", sqlStr, want)
	}
}

func TestReverseOrder(t *testing.T) {
	meta := metaOf[produtoTeste]()

	// Sem OrderBy: PK DESC.
	got := reverseOrder(nil, meta)
	if len(got) != 1 || got[0] != "id DESC" {
		t.Errorf("sem orderBy: esperava [id DESC], recebeu %v", got)
	}

	// Inverte cada termo.
	got = reverseOrder([]string{"preco DESC", "nome ASC", "id"}, meta)
	want := []string{"preco ASC", "nome DESC", "id DESC"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("termo %d: esperava %q, recebeu %q", i, want[i], got[i])
		}
	}
}

// TestAutonowMeta garante que a tag autonow marca o campo e ganha default.
func TestAutonowMeta(t *testing.T) {
	meta := metaOf[produtoTeste]()
	f, ok := meta.ColToField["updated_at"]
	if !ok || !f.IsAutoNow {
		t.Fatal("updated_at deveria ter IsAutoNow=true")
	}
	if f.Default != "CURRENT_TIMESTAMP" {
		t.Errorf("autonow deveria implicar default CURRENT_TIMESTAMP, recebeu %q", f.Default)
	}
}

// ── Search (full-text) ──────────────────────────────────────────────────────

type artigoTeste struct {
	ID        int64  `kyrux:"pk"`
	Conteudo  string `kyrux:"fts"`
	SemIndice string
}

func newSearchTestQuery(driver string) *Query[artigoTeste] {
	return &Query[artigoTeste]{
		driver: driver,
		meta:   metaOf[artigoTeste](),
	}
}

func TestSearchColunaSemTagFTS(t *testing.T) {
	q := newSearchTestQuery("postgres").Search("sem_indice", "termo")
	if q.err == nil {
		t.Fatal("esperava erro ao buscar em coluna sem kyrux:\"fts\"")
	}
}

func TestSearchIdentificadorInvalido(t *testing.T) {
	q := newSearchTestQuery("postgres").Search("conteudo; DROP TABLE x", "termo")
	if q.err == nil {
		t.Fatal("esperava erro para identificador inválido")
	}
}

func TestSearchDriverNaoSuportado(t *testing.T) {
	q := newSearchTestQuery("sqlserver").Search("conteudo", "termo")
	if q.err == nil {
		t.Fatal("esperava erro em driver sem suporte a Search")
	}
}

func TestSearchPostgresSQL(t *testing.T) {
	q := newSearchTestQuery("postgres").Search("conteudo", "golang orm")
	sqlStr, args := q.buildSelect(0)

	wantWhere := "to_tsvector('portuguese', conteudo) @@ plainto_tsquery('portuguese', $1)"
	if !strings.Contains(sqlStr, wantWhere) {
		t.Errorf("WHERE esperado ausente:\n%s\nsql: %s", wantWhere, sqlStr)
	}
	wantOrder := "ORDER BY ts_rank(to_tsvector('portuguese', conteudo), plainto_tsquery('portuguese', $2)) DESC"
	if !strings.Contains(sqlStr, wantOrder) {
		t.Errorf("ORDER BY esperado ausente:\n%s\nsql: %s", wantOrder, sqlStr)
	}
	if len(args) != 2 || args[0] != "golang orm" || args[1] != "golang orm" {
		t.Errorf("args: esperava [golang orm, golang orm], recebeu %v", args)
	}
}

func TestSearchMySQLSQL(t *testing.T) {
	q := newSearchTestQuery("mysql").Search("conteudo", "golang")
	sqlStr, args := q.buildSelect(0)

	wantWhere := "MATCH(conteudo) AGAINST(? IN NATURAL LANGUAGE MODE)"
	if !strings.Contains(sqlStr, "WHERE ("+wantWhere+")") {
		t.Errorf("WHERE esperado ausente:\nsql: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "ORDER BY "+wantWhere+" DESC") {
		t.Errorf("ORDER BY esperado ausente:\nsql: %s", sqlStr)
	}
	if len(args) != 2 || args[0] != "golang" || args[1] != "golang" {
		t.Errorf("args: esperava [golang, golang], recebeu %v", args)
	}
}

func TestSearchSQLiteSQL(t *testing.T) {
	q := newSearchTestQuery("sqlite").Search("conteudo", "golang")
	sqlStr, args := q.buildSelect(0)

	for _, want := range []string{
		"INNER JOIN artigo_testes_conteudo_fts ON artigo_testes_conteudo_fts.rowid = artigo_testes.id",
		"WHERE (artigo_testes_conteudo_fts MATCH ?)",
		"ORDER BY artigo_testes_conteudo_fts.rank",
	} {
		if !strings.Contains(sqlStr, want) {
			t.Errorf("esperava %q no sql:\n%s", want, sqlStr)
		}
	}
	if len(args) != 1 || args[0] != "golang" {
		t.Errorf("args: esperava [golang] (sem rankArgs no sqlite), recebeu %v", args)
	}
}

// TestSearchComOrderByAcrescenta garante que um OrderBy chamado depois de
// Search vira critério de desempate, sem substituir a ordenação por
// relevância (que continua sendo o primeiro critério).
func TestSearchComOrderByAcrescenta(t *testing.T) {
	q := newSearchTestQuery("postgres").Search("conteudo", "x").OrderBy("id DESC")
	sqlStr, _ := q.buildSelect(0)
	if !strings.Contains(sqlStr, "ORDER BY ts_rank(to_tsvector('portuguese', conteudo), plainto_tsquery('portuguese', $2)) DESC, id DESC") {
		t.Errorf("esperava rank como critério primário e id DESC como desempate:\n%s", sqlStr)
	}
}
