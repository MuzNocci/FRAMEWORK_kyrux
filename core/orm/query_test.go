package orm

import (
	"kyrux/core/database"
	"testing"
)

type produtoTeste struct {
	ID   int64  `kyrux:"pk"`
	Nome string `kyrux:"size:100"`
}

func newTestQuery(driver string) *Query[produtoTeste] {
	return &Query[produtoTeste]{
		db:   &database.DB{Driver: driver},
		meta: metaOf[produtoTeste](),
	}
}

// TestBuildSelectLimitPlaceholder garante que LIMIT/OFFSET viram placeholders:
// o SQL fica idêntico entre páginas e o cache de prepared statements funciona.
func TestBuildSelectLimitPlaceholder(t *testing.T) {
	q := newTestQuery("postgres").Where("nome = ?", "x").Limit(20).Offset(40)
	sqlStr, args := q.buildSelect(0)

	want := "SELECT * FROM produto_testes WHERE nome = $1 LIMIT $2 OFFSET $3"
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
