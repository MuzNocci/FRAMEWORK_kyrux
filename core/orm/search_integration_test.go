package orm

// Testes de integração real do Query.Search() — conectam nos 3 bancos
// suportados (containers Docker locais) e verificam o pipeline completo:
// DDL de full-text (o mesmo gerado por makemigrations) + Search() + ranking.
// Pulados (t.Skip) quando o respectivo banco não está acessível, para não
// quebrar em máquinas sem essa infra local.

import (
	"os"
	"testing"

	"kyrux/core/database"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type artigoSearchTeste struct {
	ID       int64  `kyrux:"pk"`
	Titulo   string `kyrux:"size:200"`
	Conteudo string `kyrux:"fts"`
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ── Postgres ────────────────────────────────────────────────────────────────

func openTestPostgres(t *testing.T) *database.DB {
	t.Helper()
	dsn := envOr("KYRUX_TEST_POSTGRES_DSN", "postgres://postgres:postgres@127.0.0.1:5432/kyrux?sslmode=disable")
	db, err := database.Open("postgres", dsn)
	if err != nil || db.Ping() != nil {
		t.Skipf("postgres indisponível em %s: %v", dsn, err)
	}
	return db
}

func TestSearchIntegrationPostgres(t *testing.T) {
	db := openTestPostgres(t)
	defer db.Close()

	db.Exec("DROP TABLE IF EXISTS artigo_search_testes")
	if _, err := db.Exec(`CREATE TABLE artigo_search_testes (
		id BIGSERIAL PRIMARY KEY,
		titulo VARCHAR(200) NOT NULL DEFAULT '',
		conteudo TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	defer db.Exec("DROP TABLE IF EXISTS artigo_search_testes")

	if _, err := db.Exec(`CREATE INDEX artigo_search_testes_conteudo_fts_idx
		ON artigo_search_testes USING GIN (to_tsvector('portuguese', conteudo))`); err != nil {
		t.Fatalf("create index: %v", err)
	}

	seedSearchArtigos(t, db)

	got, err := FromDB[artigoSearchTeste](db).Search("conteudo", "golang").All()
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	assertSearchResults(t, got, "golang")

	if _, err := FromDB[artigoSearchTeste](db).Search("titulo", "golang").All(); err == nil {
		t.Error("esperava erro ao buscar em coluna sem kyrux:\"fts\"")
	}
}

// ── MySQL ───────────────────────────────────────────────────────────────────

func openTestMySQL(t *testing.T) *database.DB {
	t.Helper()
	dsn := envOr("KYRUX_TEST_MYSQL_DSN", "root:testpass@tcp(127.0.0.1:3307)/kyrux_test?parseTime=true")
	db, err := database.Open("mysql", dsn)
	if err != nil || db.Ping() != nil {
		t.Skipf("mysql indisponível em %s: %v", dsn, err)
	}
	return db
}

func TestSearchIntegrationMySQL(t *testing.T) {
	db := openTestMySQL(t)
	defer db.Close()

	db.Exec("DROP TABLE IF EXISTS artigo_search_testes")
	if _, err := db.Exec(`CREATE TABLE artigo_search_testes (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		titulo VARCHAR(200) NOT NULL DEFAULT '',
		conteudo TEXT NOT NULL,
		FULLTEXT INDEX artigo_search_testes_conteudo_fts_idx (conteudo)
	) ENGINE=InnoDB`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	defer db.Exec("DROP TABLE IF EXISTS artigo_search_testes")

	seedSearchArtigos(t, db)

	got, err := FromDB[artigoSearchTeste](db).Search("conteudo", "golang").All()
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	assertSearchResults(t, got, "golang")
}

// ── SQLite ──────────────────────────────────────────────────────────────────

func TestSearchIntegrationSQLite(t *testing.T) {
	db, err := database.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE artigo_search_testes (
		id INTEGER PRIMARY KEY,
		titulo TEXT NOT NULL DEFAULT '',
		conteudo TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	if _, err := db.Exec(`CREATE VIRTUAL TABLE artigo_search_testes_conteudo_fts
		USING fts5(conteudo, content='artigo_search_testes', content_rowid='id')`); err != nil {
		t.Fatalf("create virtual table: %v", err)
	}
	triggers := []string{
		`CREATE TRIGGER artigo_search_testes_conteudo_fts_ai AFTER INSERT ON artigo_search_testes BEGIN
			INSERT INTO artigo_search_testes_conteudo_fts(rowid, conteudo) VALUES (new.id, new.conteudo);
		END`,
		`CREATE TRIGGER artigo_search_testes_conteudo_fts_ad AFTER DELETE ON artigo_search_testes BEGIN
			INSERT INTO artigo_search_testes_conteudo_fts(artigo_search_testes_conteudo_fts, rowid, conteudo) VALUES('delete', old.id, old.conteudo);
		END`,
		`CREATE TRIGGER artigo_search_testes_conteudo_fts_au AFTER UPDATE ON artigo_search_testes BEGIN
			INSERT INTO artigo_search_testes_conteudo_fts(artigo_search_testes_conteudo_fts, rowid, conteudo) VALUES('delete', old.id, old.conteudo);
			INSERT INTO artigo_search_testes_conteudo_fts(rowid, conteudo) VALUES (new.id, new.conteudo);
		END`,
	}
	for _, trig := range triggers {
		if _, err := db.Exec(trig); err != nil {
			t.Fatalf("create trigger: %v", err)
		}
	}

	seedSearchArtigos(t, db)

	got, err := FromDB[artigoSearchTeste](db).Search("conteudo", "golang").All()
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	assertSearchResults(t, got, "golang")

	// Update precisa continuar sincronizando a tabela-sombra via trigger.
	if err := FromDB[artigoSearchTeste](db).Where("titulo = ?", "Sobre gatos").
		Update(map[string]any{"conteudo": "agora fala de golang também"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, err := FromDB[artigoSearchTeste](db).Search("conteudo", "golang").All()
	if err != nil {
		t.Fatalf("search após update: %v", err)
	}
	if len(got2) != len(got)+1 {
		t.Errorf("esperava %d resultados após o update sincronizar via trigger, recebeu %d", len(got)+1, len(got2))
	}

	// Delete também precisa refletir na tabela-sombra.
	if err := FromDB[artigoSearchTeste](db).Where("titulo = ?", "Sobre gatos").Delete(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got3, err := FromDB[artigoSearchTeste](db).Search("conteudo", "golang").All()
	if err != nil {
		t.Fatalf("search após delete: %v", err)
	}
	if len(got3) != len(got) {
		t.Errorf("esperava %d resultados após remover a linha atualizada, recebeu %d", len(got), len(got3))
	}
}

// ── helpers compartilhados ───────────────────────────────────────────────────

// seedSearchArtigos insere via orm.Create — reaproveita a reescrita de
// placeholders por driver, em vez de SQL cru com "?" (que não funciona em
// Postgres, onde o lib/pq exige $1/$2 nativos).
func seedSearchArtigos(t *testing.T, db *database.DB) {
	t.Helper()
	rows := []*artigoSearchTeste{
		{Titulo: "Introdução ao Go", Conteudo: "Aprenda golang do zero, com exemplos práticos de concorrência."},
		{Titulo: "ORM em Go", Conteudo: "Como o kyrux usa golang e reflection para mapear structs em tabelas."},
		{Titulo: "Sobre gatos", Conteudo: "Um texto qualquer que não tem nada a ver com programação."},
	}
	for _, r := range rows {
		if err := Create(db, r); err != nil {
			t.Fatalf("seed insert: %v", err)
		}
	}
}

// assertSearchResults confere que a busca por term encontrou exatamente os
// artigos relevantes (e nenhum irrelevante), sem exigir uma ordem específica
// entre eles (o ranking exato varia por driver/dicionário).
func assertSearchResults(t *testing.T, got []artigoSearchTeste, term string) {
	t.Helper()
	if len(got) != 2 {
		t.Fatalf("esperava 2 artigos contendo %q, recebeu %d: %+v", term, len(got), got)
	}
	titulos := map[string]bool{}
	for _, a := range got {
		titulos[a.Titulo] = true
	}
	for _, want := range []string{"Introdução ao Go", "ORM em Go"} {
		if !titulos[want] {
			t.Errorf("esperava encontrar artigo %q nos resultados: %+v", want, got)
		}
	}
	if titulos["Sobre gatos"] {
		t.Error("artigo irrelevante 'Sobre gatos' não deveria aparecer na busca")
	}
}
