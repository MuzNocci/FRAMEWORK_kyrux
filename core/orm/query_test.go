package orm

import (
	"math"
	"strings"
	"testing"
	"time"
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

func TestWhereInRejeitaListaGrandeDemais(t *testing.T) {
	vals := make([]any, maxWhereInSize+1)
	for i := range vals {
		vals[i] = i
	}
	q := newTestQuery("sqlite").WhereIn("id", vals...)
	if q.err == nil {
		t.Fatal("lista acima do limite deveria gerar erro")
	}

	// No limite exato, não deveria gerar erro.
	ok := newTestQuery("sqlite").WhereIn("id", vals[:maxWhereInSize]...)
	if ok.err != nil {
		t.Errorf("lista no limite não deveria gerar erro: %v", ok.err)
	}
}

// ── métodos tipados de Where ─────────────────────────────────────────────────

func TestWhereEq(t *testing.T) {
	q := newTestQuery("postgres").WhereEq("nome", "Maria")
	sqlStr, args := q.buildSelect(0)
	want := "SELECT * FROM produto_testes WHERE (nome = $1)"
	if sqlStr != want {
		t.Errorf("sql:\n got: %s\nwant: %s", sqlStr, want)
	}
	if len(args) != 1 || args[0] != "Maria" {
		t.Errorf("args: esperava [Maria], recebeu %v", args)
	}
}

func TestWhereComparacoes(t *testing.T) {
	cases := []struct {
		name string
		q    *Query[produtoTeste]
		want string
	}{
		{"Ne", newTestQuery("sqlite").WhereNe("preco", 10), "(preco <> ?)"},
		{"Gt", newTestQuery("sqlite").WhereGt("preco", 10), "(preco > ?)"},
		{"Gte", newTestQuery("sqlite").WhereGte("preco", 10), "(preco >= ?)"},
		{"Lt", newTestQuery("sqlite").WhereLt("preco", 10), "(preco < ?)"},
		{"Lte", newTestQuery("sqlite").WhereLte("preco", 10), "(preco <= ?)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sqlStr, _ := c.q.buildSelect(0)
			if !strings.Contains(sqlStr, c.want) {
				t.Errorf("esperava %q no sql, recebeu: %s", c.want, sqlStr)
			}
		})
	}
}

func TestWhereLikeEOr(t *testing.T) {
	q := newTestQuery("sqlite").WhereLike("nome", "%maria%").OrWhereLike("nome", "%joao%")
	sqlStr, args := q.buildSelect(0)
	want := "SELECT * FROM produto_testes WHERE (nome LIKE ?) OR (nome LIKE ?)"
	if sqlStr != want {
		t.Errorf("sql:\n got: %s\nwant: %s", sqlStr, want)
	}
	if len(args) != 2 || args[0] != "%maria%" || args[1] != "%joao%" {
		t.Errorf("args: esperava [%%maria%% %%joao%%], recebeu %v", args)
	}
}

func TestWhereNullENotNull(t *testing.T) {
	q := newTestQuery("sqlite").WhereNull("preco")
	sqlStr, _ := q.buildSelect(0)
	if !strings.Contains(sqlStr, "(preco IS NULL)") {
		t.Errorf("esperava IS NULL, recebeu: %s", sqlStr)
	}

	q2 := newTestQuery("sqlite").WhereNotNull("preco")
	sqlStr2, _ := q2.buildSelect(0)
	if !strings.Contains(sqlStr2, "(preco IS NOT NULL)") {
		t.Errorf("esperava IS NOT NULL, recebeu: %s", sqlStr2)
	}
}

func TestWhereTipadoRejeitaColunaInvalida(t *testing.T) {
	if q := newTestQuery("sqlite").WhereEq("id; DROP TABLE x", 1); q.err == nil {
		t.Fatal("WhereEq com injeção deveria gerar erro")
	}
	if q := newTestQuery("sqlite").WhereLike("id; DROP TABLE x", "%a%"); q.err == nil {
		t.Fatal("WhereLike com injeção deveria gerar erro")
	}
	if q := newTestQuery("sqlite").WhereNull("id; DROP TABLE x"); q.err == nil {
		t.Fatal("WhereNull com injeção deveria gerar erro")
	}
}

func TestWhereSQLEhAliasDeWhere(t *testing.T) {
	a := newTestQuery("sqlite").WhereSQL("preco > ?", 10)
	b := newTestQuery("sqlite").Where("preco > ?", 10)
	sqlA, _ := a.buildSelect(0)
	sqlB, _ := b.buildSelect(0)
	if sqlA != sqlB {
		t.Errorf("WhereSQL deveria gerar o mesmo SQL que Where:\n%s\n%s", sqlA, sqlB)
	}
}

func TestAscDesc(t *testing.T) {
	q := newTestQuery("sqlite").OrderBy(Desc("preco"), Asc("nome"))
	sqlStr, _ := q.buildSelect(0)
	want := "SELECT * FROM produto_testes ORDER BY preco DESC, nome ASC"
	if sqlStr != want {
		t.Errorf("sql:\n got: %s\nwant: %s", sqlStr, want)
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

// ── clampPaging (limites de Paginate/PaginateNoCount/PaginateAfter) ────────────

func TestClampPagingValoresNormais(t *testing.T) {
	page, pageSize := clampPaging(2, 20)
	if page != 2 || pageSize != 20 {
		t.Errorf("esperava (2, 20) sem alteração, recebeu (%d, %d)", page, pageSize)
	}
}

func TestClampPagingValoresInvalidosUsamDefault(t *testing.T) {
	page, pageSize := clampPaging(0, 0)
	if page != 1 || pageSize != 10 {
		t.Errorf("esperava (1, 10), recebeu (%d, %d)", page, pageSize)
	}
	page, pageSize = clampPaging(-5, -5)
	if page != 1 || pageSize != 10 {
		t.Errorf("valores negativos: esperava (1, 10), recebeu (%d, %d)", page, pageSize)
	}
}

func TestClampPagingLimitaPageSize(t *testing.T) {
	_, pageSize := clampPaging(1, 1_000_000)
	if pageSize != maxPageSize {
		t.Errorf("esperava pageSize limitado a %d, recebeu %d", maxPageSize, pageSize)
	}
}

func TestClampPagingNaoEstouraOffset(t *testing.T) {
	// page extremo não deveria fazer (page-1)*pageSize dar overflow /
	// virar negativo — clampPaging satura em vez de estourar.
	page, pageSize := clampPaging(math.MaxInt, 100)
	offset := (page - 1) * pageSize
	if offset < 0 {
		t.Errorf("offset não deveria ser negativo (overflow): page=%d pageSize=%d offset=%d", page, pageSize, offset)
	}
}

// ── PaginateAfter (keyset pagination) ───────────────────────────────────────

func TestPaginateAfterMontaSQL(t *testing.T) {
	// PaginateAfter precisa de um driver "de mentira" que devolve erro de
	// conexão — o que importa aqui é validar que a SQL/args são montados
	// corretamente antes da execução, então testamos via buildSelect
	// reproduzindo a mesma lógica que PaginateAfter usa internamente.
	q := newTestQuery("postgres")
	rq := *q
	rq.where = append([]whereClause{}, q.where...)
	rq.where = append(rq.where, whereClause{cond: "id > ?"})
	rq.args = append(append([]any{}, q.args...), int64(10))
	rq.orderBy = []string{"id ASC"}

	sqlStr, args := rq.buildSelect(6)
	want := "SELECT * FROM produto_testes WHERE (id > $1) ORDER BY id ASC LIMIT $2"
	if sqlStr != want {
		t.Errorf("sql:\n got: %s\nwant: %s", sqlStr, want)
	}
	if len(args) != 2 || args[0] != int64(10) || args[1] != 6 {
		t.Errorf("args: esperava [10 6], recebeu %v", args)
	}
}

func TestPaginateAfterRejeitaColunaInvalida(t *testing.T) {
	q := newTestQuery("sqlite")
	_, err := q.PaginateAfter("id; DROP TABLE x", nil, false, 10)
	if err == nil {
		t.Fatal("coluna com injeção deveria gerar erro")
	}
}

func TestPaginateAfterRejeitaColunaDesconhecida(t *testing.T) {
	q := newTestQuery("sqlite")
	_, err := q.PaginateAfter("coluna_que_nao_existe", nil, false, 10)
	if err == nil {
		t.Fatal("coluna fora do model deveria gerar erro")
	}
}

// TestPaginateAfterFimAFimSQLite roda PaginateAfter contra um SQLite real
// para garantir a integração completa: keyset avançando página a página até
// esgotar os resultados, sem pular nem repetir linhas.
func TestPaginateAfterFimAFimSQLite(t *testing.T) {
	db := openMemDB(t)
	if err := EnsureSQLiteTable[ddlTestUser](db); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ana", "bruno", "carla", "duda", "erik"} {
		if err := Create(db, &ddlTestUser{Username: name, CreatedAt: time.Now()}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	var all []string
	var cursor any
	for {
		page, err := FromDB[ddlTestUser](db).PaginateAfter("id", cursor, false, 2)
		if err != nil {
			t.Fatalf("paginateafter: %v", err)
		}
		for _, u := range page.Items {
			all = append(all, u.Username)
		}
		if !page.HasNext {
			break
		}
		cursor = page.NextCursor
	}

	want := []string{"ana", "bruno", "carla", "duda", "erik"}
	if len(all) != len(want) {
		t.Fatalf("esperava %d usuários, recebeu %d: %v", len(want), len(all), all)
	}
	for i := range want {
		if all[i] != want[i] {
			t.Errorf("posição %d: esperava %q, recebeu %q (%v)", i, want[i], all[i], all)
		}
	}
}
