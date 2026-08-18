package orm

import (
	"errors"
	"sync"
	"testing"
	"time"

	"kyrux/core/database"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

// ── isUniqueViolation ────────────────────────────────────────────────────────

func TestIsUniqueViolationPostgres(t *testing.T) {
	if !isUniqueViolation(&pq.Error{Code: "23505"}) {
		t.Error("esperava true para SQLSTATE 23505 (unique_violation)")
	}
	if isUniqueViolation(&pq.Error{Code: "23503"}) { // foreign_key_violation
		t.Error("não deveria reportar unique violation para outro SQLSTATE")
	}
}

func TestIsUniqueViolationMySQL(t *testing.T) {
	if !isUniqueViolation(&mysql.MySQLError{Number: 1062}) { // ER_DUP_ENTRY
		t.Error("esperava true para o erro 1062 (ER_DUP_ENTRY)")
	}
	if isUniqueViolation(&mysql.MySQLError{Number: 1451}) { // FK violation
		t.Error("não deveria reportar unique violation para outro código de erro")
	}
}

func TestIsUniqueViolationSQLite(t *testing.T) {
	db := openMemDB(t)
	if err := EnsureSQLiteTable[ddlTestUser](db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO ddl_test_users (username, created_at) VALUES ('dup', CURRENT_TIMESTAMP)"); err != nil {
		t.Fatalf("primeiro insert: %v", err)
	}
	_, err := db.Exec("INSERT INTO ddl_test_users (username, created_at) VALUES ('dup', CURRENT_TIMESTAMP)")
	if err == nil {
		t.Fatal("esperava erro de índice único")
	}
	if !isUniqueViolation(err) {
		t.Errorf("isUniqueViolation deveria reconhecer a violação UNIQUE do sqlite, erro: %v", err)
	}
}

func TestIsUniqueViolationErroGenericoOuNil(t *testing.T) {
	if isUniqueViolation(nil) {
		t.Error("nil não deveria ser reportado como unique violation")
	}
	if isUniqueViolation(errors.New("erro qualquer")) {
		t.Error("erro genérico não deveria ser reportado como unique violation")
	}
}

// ── GetOrCreate / UpdateOrCreate — corrida ──────────────────────────────────

// TestGetOrCreateFiltroDesalinhadoNaoEngoleErro garante que, quando o filtro
// do Where NÃO cobre a coluna que colidiu (ou seja, o retry de First() não
// vai mesmo encontrar a linha), GetOrCreate propaga o erro original em vez
// de mascará-lo como um "get" bem-sucedido — a recuperação de corrida só
// deve se aplicar quando ela realmente resolve o filtro.
func TestGetOrCreateFiltroDesalinhadoNaoEngoleErro(t *testing.T) {
	db := openMemDB(t)
	if err := EnsureSQLiteTable[ddlTestUser](db); err != nil {
		t.Fatal(err)
	}
	// "outro processo" já criou eve antes desta chamada.
	if err := Create(db, &ddlTestUser{Username: "eve", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("insert direto: %v", err)
	}

	// Filtro não bate com username — First() nunca vai encontrar "eve", nem
	// antes nem depois do retry. defaults colide em username (unique).
	q := FromDB[ddlTestUser](db).Where("id = ?", -1)
	_, _, err := q.GetOrCreate(&ddlTestUser{Username: "eve", CreatedAt: time.Now()})
	if err == nil {
		t.Fatal("esperava erro: o retry não deveria encontrar nada com um filtro desalinhado, e o erro de unicidade não deveria ser engolido")
	}
}

// TestGetOrCreateConcorrenteCriaApenasUmaLinha exercita GetOrCreate sob
// concorrência real (múltiplas goroutines, mesmo filtro/defaults, contra o
// mesmo arquivo SQLite) — antes da correção, uma chamada perdendo a corrida
// entre First() e Create() devolvia erro de violação de unicidade em vez de
// se recuperar. A asserção não depende de qual goroutine exercita o retry:
// só que, no fim, exista exatamente uma linha e nenhuma chamada tenha
// falhado.
func TestGetOrCreateConcorrenteCriaApenasUmaLinha(t *testing.T) {
	path := t.TempDir() + "/getorcreate.db"
	db := openFileDBForConcurrency(t, path)

	if err := EnsureSQLiteTable[ddlTestUser](db); err != nil {
		t.Fatal(err)
	}

	const n = 12
	var wg sync.WaitGroup
	errs := make([]error, n)
	ids := make([]int64, n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			obj, _, err := FromDB[ddlTestUser](db).Where("username = ?", "frank").
				GetOrCreate(&ddlTestUser{Username: "frank", CreatedAt: time.Now()})
			errs[i] = err
			if err == nil {
				ids[i] = obj.ID
			}
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("chamada %d: erro inesperado: %v", i, err)
		}
	}

	var count int64
	if err := db.QueryRow("SELECT COUNT(*) FROM ddl_test_users WHERE username = 'frank'").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("esperava exatamente 1 linha 'frank', encontrei %d", count)
	}

	firstID := ids[0]
	for i, id := range ids {
		if id != firstID {
			t.Errorf("chamada %d devolveu ID %d, esperava %d (todas deveriam apontar para a mesma linha)", i, id, firstID)
		}
	}
}

// openFileDBForConcurrency abre um SQLite em arquivo (não :memory:, que não é
// compartilhado entre conexões do pool) com WAL + busy_timeout, para que
// escritores concorrentes serializem em vez de falhar com "database locked".
func openFileDBForConcurrency(t *testing.T, path string) *database.DB {
	t.Helper()
	db, err := database.Open("sqlite", path)
	if err != nil {
		t.Fatalf("abrir sqlite em arquivo: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	return db
}
