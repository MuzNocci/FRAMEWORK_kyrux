package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMigration(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestMigSchemaInDir(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "0001_auto.sql", `-- Migração gerada
CREATE TABLE IF NOT EXISTS posts (
    id                   BIGSERIAL PRIMARY KEY,
    titulo               VARCHAR(200) NOT NULL DEFAULT '',
    user_id              BIGINT NOT NULL DEFAULT 0 REFERENCES users(id)
);

CREATE UNIQUE INDEX IF NOT EXISTS posts_titulo_idx ON posts (titulo);

-- down
DROP TABLE IF EXISTS posts;
`)
	writeMigration(t, dir, "0002_auto.sql", `ALTER TABLE posts ADD COLUMN resumo TEXT;
ALTER TABLE posts DROP COLUMN titulo;

-- down
ALTER TABLE posts DROP COLUMN resumo;
`)

	schema, err := migSchemaInDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	cols := schema["posts"]
	if cols == nil {
		t.Fatal("tabela posts não detectada")
	}
	for _, want := range []string{"id", "user_id", "resumo"} {
		if !cols[want] {
			t.Errorf("coluna %q deveria existir no schema: %v", want, cols)
		}
	}
	// DROP na seção up remove; a seção down NÃO pode ser processada.
	if cols["titulo"] {
		t.Error("coluna 'titulo' foi dropada no up e não deveria constar")
	}
}

func TestMigDiffEGeracaoDeAlter(t *testing.T) {
	m := migModel{
		Name:  "Post",
		Table: "posts",
		Fields: []migField{
			{Column: "id", GoType: "int64", IsPK: true},
			{Column: "titulo", GoType: "string", Size: 200, NotNull: true},
			{Column: "visitas", GoType: "int64", NotNull: true},
			{Column: "slug", GoType: "string", Size: 100, NotNull: true, Unique: true},
		},
	}
	existing := map[string]bool{"id": true, "titulo": true, "antiga": true}

	a := migDiffModel(m, existing)
	if a == nil {
		t.Fatal("diff deveria detectar mudanças")
	}
	if len(a.Add) != 2 || a.Add[0].Column != "visitas" || a.Add[1].Column != "slug" {
		t.Fatalf("Add errado: %+v", a.Add)
	}
	if len(a.Removed) != 1 || a.Removed[0] != "antiga" {
		t.Fatalf("Removed errado: %v", a.Removed)
	}

	sql := migGenerateSQL(nil, []migAlter{*a}, "postgres")

	for _, want := range []string{
		"ALTER TABLE posts ADD COLUMN visitas BIGINT NOT NULL DEFAULT 0;",
		"ALTER TABLE posts ADD COLUMN slug VARCHAR(100) NOT NULL DEFAULT '';",
		"CREATE UNIQUE INDEX IF NOT EXISTS posts_slug_idx ON posts (slug);",
		"-- ALTER TABLE posts DROP COLUMN antiga;", // sugestão comentada
		"ALTER TABLE posts DROP COLUMN visitas;",   // down
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL gerado deveria conter %q\n---\n%s", want, sql)
		}
	}
	// ADD COLUMN não pode ter UNIQUE inline (SQLite não aceita).
	if strings.Contains(sql, "slug VARCHAR(100) NOT NULL DEFAULT '' UNIQUE") {
		t.Error("UNIQUE inline em ADD COLUMN não é portável")
	}
}

func TestMigDiffSemMudanca(t *testing.T) {
	m := migModel{Table: "posts", Fields: []migField{
		{Column: "id", GoType: "int64", IsPK: true},
		{Column: "titulo", GoType: "string"},
	}}
	if a := migDiffModel(m, map[string]bool{"id": true, "titulo": true}); a != nil {
		t.Errorf("sem mudanças deveria retornar nil, recebeu %+v", a)
	}
}

// ── kyrux:"fts" — DDL de full-text por driver ────────────────────────────────

// TestMigBuildFieldParseiaFTS confere que a tag "fts" é reconhecida no
// mesmo parser (via AST) usado por makemigrations em apps/*/models/*.go —
// não só na versão de teste que monta migField manualmente.
func TestMigBuildFieldParseiaFTS(t *testing.T) {
	f := migBuildField("Conteudo", "string", "fts", true)
	if !f.FTS {
		t.Error("esperava FTS=true ao parsear a tag kyrux:\"fts\"")
	}
	if f.Column != "conteudo" {
		t.Errorf("esperava coluna 'conteudo', recebeu %q", f.Column)
	}

	f2 := migBuildField("Titulo", "string", "size:200", true)
	if f2.FTS {
		t.Error("campo sem a tag fts não deveria ter FTS=true")
	}

	f3 := migBuildField("Resumo", "string", "size:500,fts", true)
	if !f3.FTS || f3.Size != 500 {
		t.Errorf("esperava FTS=true e Size=500 combinando tags, recebeu %+v", f3)
	}
}

func migFTSTestModel() migModel {
	return migModel{
		Name:  "Post",
		Table: "posts",
		Fields: []migField{
			{Column: "id", GoType: "int64", IsPK: true},
			{Column: "conteudo", GoType: "string", NotNull: true, FTS: true},
		},
	}
}

func TestMigFTSPostgres(t *testing.T) {
	sql := migGenerateSQL([]migModel{migFTSTestModel()}, nil, "postgres")
	for _, want := range []string{
		"CREATE INDEX IF NOT EXISTS posts_conteudo_fts_idx ON posts USING GIN (to_tsvector('portuguese', conteudo));",
		"DROP INDEX IF EXISTS posts_conteudo_fts_idx;",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL deveria conter %q\n---\n%s", want, sql)
		}
	}
}

func TestMigFTSMySQL(t *testing.T) {
	sql := migGenerateSQL([]migModel{migFTSTestModel()}, nil, "mysql")
	for _, want := range []string{
		"CREATE FULLTEXT INDEX posts_conteudo_fts_idx ON posts (conteudo);",
		"DROP INDEX posts_conteudo_fts_idx ON posts;",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL deveria conter %q\n---\n%s", want, sql)
		}
	}
}

func TestMigFTSSQLite(t *testing.T) {
	sql := migGenerateSQL([]migModel{migFTSTestModel()}, nil, "sqlite")
	for _, want := range []string{
		"CREATE VIRTUAL TABLE IF NOT EXISTS posts_conteudo_fts USING fts5(conteudo, content='posts', content_rowid='id');",
		"CREATE TRIGGER IF NOT EXISTS posts_conteudo_fts_ai AFTER INSERT ON posts",
		"CREATE TRIGGER IF NOT EXISTS posts_conteudo_fts_ad AFTER DELETE ON posts",
		"CREATE TRIGGER IF NOT EXISTS posts_conteudo_fts_au AFTER UPDATE ON posts",
		"DROP TRIGGER IF EXISTS posts_conteudo_fts_ai;",
		"DROP TRIGGER IF EXISTS posts_conteudo_fts_ad;",
		"DROP TRIGGER IF EXISTS posts_conteudo_fts_au;",
		"DROP TABLE IF EXISTS posts_conteudo_fts;",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL deveria conter %q\n---\n%s", want, sql)
		}
	}
}

func TestMigFTSDriverNaoSuportadoGeraAviso(t *testing.T) {
	sql := migGenerateSQL([]migModel{migFTSTestModel()}, nil, "sqlserver")
	want := `kyrux:"fts" em posts.conteudo ignorado`
	if !strings.Contains(sql, want) {
		t.Errorf("esperava aviso comentado sobre driver não suportado, recebeu:\n%s", sql)
	}
	if strings.Contains(sql, "CREATE INDEX") || strings.Contains(sql, "CREATE VIRTUAL") {
		t.Error("driver não suportado não deveria gerar nenhum DDL de FTS")
	}
}

// TestMigFTSAlterExistente cobre o caminho de ALTER TABLE (campo fts novo
// num model já migrado) — precisa da PKColumn vinda do migAlter.
func TestMigFTSAlterExistente(t *testing.T) {
	a := migAlter{
		Table:    "posts",
		PKColumn: "id",
		Add:      []migField{{Column: "conteudo", GoType: "string", NotNull: true, FTS: true}},
	}
	sql := migGenerateSQL(nil, []migAlter{a}, "sqlite")
	if !strings.Contains(sql, "CREATE VIRTUAL TABLE IF NOT EXISTS posts_conteudo_fts USING fts5(conteudo, content='posts', content_rowid='id');") {
		t.Errorf("ALTER com fts deveria gerar a tabela virtual usando a PKColumn do alter:\n%s", sql)
	}
}
