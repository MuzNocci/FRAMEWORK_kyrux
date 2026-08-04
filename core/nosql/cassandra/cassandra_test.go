package cassandra

// Testes de integração real contra um Cassandra de verdade (container
// Docker local). Pulados (t.Skip) se o servidor não estiver acessível.

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gocql/gocql"
)

func gocqlTestUUID(t *testing.T) gocql.UUID {
	t.Helper()
	id, err := gocql.RandomUUID()
	if err != nil {
		t.Fatalf("gerar uuid: %v", err)
	}
	return id
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type produtoRow struct {
	ID    string  `json:"id"`
	Nome  string  `json:"nome"`
	Preco float64 `json:"preco"`
}

func openTestClient(t *testing.T) *Client {
	t.Helper()
	host := envOr("KYRUX_TEST_CASSANDRA_HOST", "127.0.0.1")

	// Conecta sem keyspace primeiro pra poder criar o keyspace de teste.
	admin, err := New([]string{host}, "")
	if err != nil {
		t.Skipf("cassandra indisponível em %s: %v", host, err)
	}
	ctx := context.Background()
	if err := admin.Exec(ctx, `CREATE KEYSPACE IF NOT EXISTS kyrux_test
		WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}`); err != nil {
		admin.Close()
		t.Fatalf("create keyspace: %v", err)
	}
	admin.Close()

	c, err := New([]string{host}, "kyrux_test")
	if err != nil {
		t.Skipf("cassandra (keyspace) indisponível: %v", err)
	}
	t.Cleanup(c.Close)

	if err := c.Exec(ctx, "DROP TABLE IF EXISTS produtos"); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := c.Exec(ctx, `CREATE TABLE produtos (
		id uuid PRIMARY KEY,
		nome text,
		preco double
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return c
}

func TestCassandraExecInsertSelectMap(t *testing.T) {
	c := openTestClient(t)
	ctx := context.Background()

	id1 := gocqlTestUUID(t)
	if err := c.Exec(ctx, "INSERT INTO produtos (id, nome, preco) VALUES (?, ?, ?)", id1, "Caneca", 29.9); err != nil {
		t.Fatalf("exec insert: %v", err)
	}

	rows, err := c.SelectMap(ctx, "SELECT nome, preco FROM produtos WHERE id = ?", id1)
	if err != nil {
		t.Fatalf("selectmap: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("esperava 1 linha, recebeu %d", len(rows))
	}
	if rows[0]["nome"] != "Caneca" {
		t.Errorf("esperava nome=Caneca, recebeu %v", rows[0]["nome"])
	}
}

func TestCassandraSelectGeneric(t *testing.T) {
	c := openTestClient(t)
	ctx := context.Background()

	id1 := gocqlTestUUID(t)
	if err := c.Exec(ctx, "INSERT INTO produtos (id, nome, preco) VALUES (?, ?, ?)", id1, "Mochila", 199.9); err != nil {
		t.Fatalf("exec insert: %v", err)
	}

	// Select genérico não decodifica bem uuid nativo em string simples sem
	// conversão — usamos texto (nome/preco) que já roundtrippam limpo via JSON.
	rows, err := Select[struct {
		Nome  string  `json:"nome"`
		Preco float64 `json:"preco"`
	}](ctx, c, "SELECT nome, preco FROM produtos WHERE id = ?", id1)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("esperava 1 linha, recebeu %d", len(rows))
	}
	if rows[0].Nome != "Mochila" || rows[0].Preco != 199.9 {
		t.Errorf("linha incorreta: %+v", rows[0])
	}
}

func TestCassandraDelete(t *testing.T) {
	c := openTestClient(t)
	ctx := context.Background()

	id1 := gocqlTestUUID(t)
	if err := c.Exec(ctx, "INSERT INTO produtos (id, nome, preco) VALUES (?, ?, ?)", id1, "Temp", 1.0); err != nil {
		t.Fatalf("exec insert: %v", err)
	}
	if err := c.Exec(ctx, "DELETE FROM produtos WHERE id = ?", id1); err != nil {
		t.Fatalf("exec delete: %v", err)
	}
	rows, err := c.SelectMap(ctx, "SELECT nome FROM produtos WHERE id = ?", id1)
	if err != nil {
		t.Fatalf("selectmap: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("esperava 0 linhas após delete, recebeu %d", len(rows))
	}
}

func TestCassandraWhereForaDaPartitionKeyFalha(t *testing.T) {
	c := openTestClient(t)
	ctx := context.Background()

	// Documenta a restrição real do CQL: filtrar por uma coluna que não é a
	// partition key (sem índice, sem ALLOW FILTERING) é rejeitado pelo
	// próprio Cassandra — não é algo que este wrapper poderia "consertar".
	_, err := c.SelectMap(ctx, "SELECT id FROM produtos WHERE nome = ?", "Caneca")
	if err == nil {
		t.Error("esperava erro do Cassandra ao filtrar por coluna fora da partition key sem ALLOW FILTERING")
	} else if !strings.Contains(err.Error(), "ALLOW FILTERING") {
		t.Logf("erro recebido (aceitável, só documentando o comportamento): %v", err)
	}
}

func TestNewCassandraFalhaComHostInvalido(t *testing.T) {
	if _, err := New([]string{"127.0.0.1:1"}, ""); err == nil {
		t.Error("esperava erro ao conectar em endereço inválido")
	}
}
