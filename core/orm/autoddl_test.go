package orm

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"kyrux/core/database"

	_ "modernc.org/sqlite"
)

type ddlTestUser struct {
	ID        int64   `kyrux:"pk"`
	Username  string  `kyrux:"size:150,unique"`
	Email     *string `kyrux:"size:254,unique"`
	IsActive  bool    `kyrux:"default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time `kyrux:"autonow"`
}

type ddlTestPost struct {
	ID     int64  `kyrux:"pk"`
	Titulo string `kyrux:"size:200"`
	UserID int64  `kyrux:"fk:ddl_test_users"`
}

func openMemDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("abrir sqlite em memória: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestEnsureSQLiteTableCriaEIdempotente(t *testing.T) {
	db := openMemDB(t)

	if err := EnsureSQLiteTable[ddlTestUser](db); err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	// Chamar de novo não deve falhar (IF NOT EXISTS) nem apagar dados.
	// created_at/updated_at são NOT NULL sem default (mesma tag de auth.User)
	// — igual ao orm.Create, o INSERT sempre fornece um valor explícito.
	if _, err := db.Exec("INSERT INTO ddl_test_users (username, is_active, created_at) VALUES ('alice', 1, CURRENT_TIMESTAMP)"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := EnsureSQLiteTable[ddlTestUser](db); err != nil {
		t.Fatalf("segunda chamada (idempotência): %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM ddl_test_users").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("segunda chamada não deveria apagar dados existentes, count=%d", count)
	}
}

func TestEnsureSQLiteTableUniqueBloqueiaDuplicata(t *testing.T) {
	db := openMemDB(t)
	if err := EnsureSQLiteTable[ddlTestUser](db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO ddl_test_users (username, created_at) VALUES ('bob', CURRENT_TIMESTAMP)"); err != nil {
		t.Fatalf("primeiro insert: %v", err)
	}
	if _, err := db.Exec("INSERT INTO ddl_test_users (username, created_at) VALUES ('bob', CURRENT_TIMESTAMP)"); err == nil {
		t.Error("índice único deveria rejeitar username duplicado")
	}
}

func TestEnsureSQLiteTablePermiteNullEmCampoPonteiro(t *testing.T) {
	db := openMemDB(t)
	if err := EnsureSQLiteTable[ddlTestUser](db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO ddl_test_users (username, email, created_at) VALUES ('carol', NULL, CURRENT_TIMESTAMP)"); err != nil {
		t.Errorf("campo ponteiro (Email) deveria aceitar NULL: %v", err)
	}
}

func TestEnsureSQLiteTableRejeitaDriverNaoSQLite(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para driver não-sqlite")
		}
	}()
	fakeDB := &database.DB{Driver: "postgres"}
	_ = EnsureSQLiteTable[ddlTestUser](fakeDB)
}

func TestBuildSQLiteCreateTableTiposEConstraints(t *testing.T) {
	meta := MetaOf[ddlTestUser]()
	structType := reflect.TypeOf(ddlTestUser{})
	createStmt, indexStmts := buildSQLiteCreateTable(meta, structType)

	if !strings.Contains(createStmt, "id INTEGER PRIMARY KEY") {
		t.Errorf("PK deveria ser INTEGER PRIMARY KEY:\n%s", createStmt)
	}
	if !strings.Contains(createStmt, "username VARCHAR(150) NOT NULL") {
		t.Errorf("username deveria ser VARCHAR(150) NOT NULL:\n%s", createStmt)
	}
	if strings.Contains(createStmt, "email VARCHAR(254) NOT NULL") {
		t.Errorf("email é ponteiro — não deveria ter NOT NULL:\n%s", createStmt)
	}
	if !strings.Contains(createStmt, "is_active INTEGER NOT NULL DEFAULT true") {
		t.Errorf("is_active deveria ser INTEGER NOT NULL DEFAULT true:\n%s", createStmt)
	}
	if !strings.Contains(createStmt, "created_at DATETIME NOT NULL") {
		t.Errorf("created_at deveria ser DATETIME NOT NULL:\n%s", createStmt)
	}
	if !strings.Contains(createStmt, "updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP") {
		t.Errorf("updated_at (autonow) deveria ter DEFAULT CURRENT_TIMESTAMP:\n%s", createStmt)
	}

	if len(indexStmts) != 2 {
		t.Fatalf("esperava 2 índices únicos (username, email), recebeu %d: %v", len(indexStmts), indexStmts)
	}
	joined := strings.Join(indexStmts, " | ")
	if !strings.Contains(joined, "CREATE UNIQUE INDEX IF NOT EXISTS ddl_test_users_username_idx") {
		t.Errorf("índice único de username ausente: %v", indexStmts)
	}
}

func TestBuildSQLiteCreateTableFK(t *testing.T) {
	meta := MetaOf[ddlTestPost]()
	structType := reflect.TypeOf(ddlTestPost{})
	createStmt, _ := buildSQLiteCreateTable(meta, structType)
	if !strings.Contains(createStmt, "user_id INTEGER NOT NULL REFERENCES ddl_test_users(id)") {
		t.Errorf("FK ausente ou incorreta:\n%s", createStmt)
	}
}

// TestEnsureSQLiteTableComOrmCreate garante que orm.Create funciona sobre a
// tabela gerada, inclusive deixando um campo NOT NULL sem default (CreatedAt)
// no zero value — exatamente o que o formulário "Novo" do admin faz quando
// o campo não é preenchido. orm.Create sempre passa um valor explícito por
// coluna (mesmo que seja o zero value do Go), então nunca colide com NOT NULL.
func TestEnsureSQLiteTableComOrmCreate(t *testing.T) {
	db := openMemDB(t)
	if err := EnsureSQLiteTable[ddlTestUser](db); err != nil {
		t.Fatal(err)
	}
	u := &ddlTestUser{Username: "dave", IsActive: true} // CreatedAt/UpdatedAt ficam zero
	if err := Create(db, u); err != nil {
		t.Fatalf("orm.Create: %v", err)
	}
	if u.ID == 0 {
		t.Error("PK deveria ser preenchida após Create")
	}

	got, err := FromDB[ddlTestUser](db).Where("username = ?", "dave").First()
	if err != nil {
		t.Fatalf("First: %v", err)
	}
	if got.Username != "dave" {
		t.Errorf("username incorreto: %q", got.Username)
	}
}

func TestSqliteDefaultTraduzNOW(t *testing.T) {
	if got := sqliteDefault("NOW()"); got != "CURRENT_TIMESTAMP" {
		t.Errorf("NOW() deveria virar CURRENT_TIMESTAMP, recebeu %q", got)
	}
	if got := sqliteDefault("now()"); got != "CURRENT_TIMESTAMP" {
		t.Errorf("now() (case-insensitive) deveria virar CURRENT_TIMESTAMP, recebeu %q", got)
	}
	if got := sqliteDefault("0"); got != "0" {
		t.Errorf("outros defaults não deveriam ser alterados, recebeu %q", got)
	}
}
