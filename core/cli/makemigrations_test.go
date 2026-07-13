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
