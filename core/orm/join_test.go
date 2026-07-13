package orm

import (
	"strings"
	"testing"
)

func TestJoinSelecionaSoTabelaBase(t *testing.T) {
	q := newTestQuery("postgres").
		Join("users", "users.id = produto_testes.user_id").
		Where("users.is_active = ?", true)
	sqlStr, _ := q.buildSelect(0)

	want := "SELECT produto_testes.* FROM produto_testes INNER JOIN users ON users.id = produto_testes.user_id WHERE (users.is_active = $1)"
	if sqlStr != want {
		t.Errorf("sql:\n got: %s\nwant: %s", sqlStr, want)
	}
}

func TestLeftJoin(t *testing.T) {
	q := newTestQuery("sqlite").LeftJoin("users", "users.id = produto_testes.user_id")
	sqlStr, _ := q.buildSelect(0)
	if !strings.Contains(sqlStr, "LEFT JOIN users ON") {
		t.Errorf("esperava LEFT JOIN, recebeu: %s", sqlStr)
	}
}

func TestJoinValidaOn(t *testing.T) {
	q := newTestQuery("sqlite").Join("users", "1=1; DROP TABLE x")
	if q.err == nil {
		t.Fatal("ON com injeção deveria gerar erro")
	}
	q2 := newTestQuery("sqlite").Join("users; --", "users.id = t.user_id")
	if q2.err == nil {
		t.Fatal("tabela com injeção deveria gerar erro")
	}
}

func TestJoinBloqueadoEmUpdateDelete(t *testing.T) {
	err := newTestQuery("sqlite").
		Join("users", "users.id = produto_testes.user_id").
		Where("id = ?", 1).
		Update(map[string]any{"nome": "x"})
	if err == nil || !strings.Contains(err.Error(), "JOIN") {
		t.Errorf("update com join deveria falhar, recebeu %v", err)
	}

	err = newTestQuery("sqlite").
		Join("users", "users.id = produto_testes.user_id").
		Where("id = ?", 1).
		Delete()
	if err == nil || !strings.Contains(err.Error(), "JOIN") {
		t.Errorf("delete com join deveria falhar, recebeu %v", err)
	}
}

func TestJoinEmCountEExists(t *testing.T) {
	// Verifica só a construção do FROM (execução exigiria banco).
	q := newTestQuery("sqlite").Join("users", "users.id = produto_testes.user_id")
	var sb strings.Builder
	sb.WriteString("SELECT COUNT(*)")
	q.writeFrom(&sb)
	if !strings.Contains(sb.String(), "INNER JOIN users") {
		t.Errorf("Count deveria incluir o JOIN: %s", sb.String())
	}
}

func TestGroupByKey(t *testing.T) {
	type coment struct{ PostID int64 }
	items := []coment{{1}, {2}, {1}, {3}, {1}}
	g := groupByKey(items, func(c *coment) int64 { return c.PostID })
	if len(g[1]) != 3 || len(g[2]) != 1 || len(g[3]) != 1 {
		t.Errorf("agrupamento errado: %v", g)
	}
}

func TestDedupe(t *testing.T) {
	out := dedupe([]int64{5, 3, 5, 1, 3})
	if len(out) != 3 || out[0] != int64(5) || out[1] != int64(3) || out[2] != int64(1) {
		t.Errorf("dedupe deveria preservar ordem da primeira ocorrência: %v", out)
	}
}
