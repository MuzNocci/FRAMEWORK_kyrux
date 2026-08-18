package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"kyrux/core/environment"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ── makemigrations ────────────────────────────────────────────────────────────

// excludeTestFiles remove *_test.go da lista — arquivo de teste nunca é
// origem de model real, mesmo que declare um struct kyrux:"pk" (fixture).
func excludeTestFiles(paths []string) []string {
	out := paths[:0]
	for _, p := range paths {
		if !strings.HasSuffix(p, "_test.go") {
			out = append(out, p)
		}
	}
	return out
}

func runMakeMigrations() error {
	_ = environment.Load(".env")
	driver := environment.GetOr("DB_DRIVER", "postgres")

	appFiles, err := filepath.Glob(filepath.Join("apps", "*", "models", "*.go"))
	if err != nil {
		return fmt.Errorf("listar models de apps: %w", err)
	}
	authFiles, err := filepath.Glob(filepath.Join("core", "security", "auth", "*.go"))
	if err != nil {
		return fmt.Errorf("listar models de core/security/auth: %w", err)
	}
	// core/admin também é escaneado — o histórico de alterações
	// (admin.HistoryLog) é um model do próprio framework, igual a auth.User:
	// precisa de tabela em qualquer banco real, sem o dev declarar nada.
	adminFiles, err := filepath.Glob(filepath.Join("core", "admin", "*.go"))
	if err != nil {
		return fmt.Errorf("listar models de core/admin: %w", err)
	}
	files := append(appFiles, authFiles...)
	files = append(files, adminFiles...)
	// *_test.go nunca é model de verdade — sem isso, structs kyrux:"pk" de
	// fixtures de teste (definidas direto em core/admin, core/security/auth)
	// virariam tabela na migration gerada.
	files = excludeTestFiles(files)

	schema, err := migSchemaInDir("database/migrations")
	if err != nil {
		return fmt.Errorf("ler migrações existentes: %w", err)
	}

	var pending []migModel
	var alters []migAlter
	for _, f := range files {
		models, err := migParseFile(f)
		if err != nil {
			fmt.Printf("  aviso: ignorando %s: %v\n", f, err)
			continue
		}
		for _, m := range models {
			cols, exists := schema[m.Table]
			if !exists {
				pending = append(pending, m)
				continue
			}
			if a := migDiffModel(m, cols); a != nil {
				alters = append(alters, *a)
			}
		}
	}

	hasAdds := false
	for _, a := range alters {
		if len(a.Add) > 0 {
			hasAdds = true
		}
		for _, c := range a.Removed {
			fmt.Printf("  aviso: coluna '%s.%s' existe nas migrations mas sumiu do model — remoção NÃO é gerada automaticamente\n", a.Table, c)
		}
	}

	if len(pending) == 0 && !hasAdds {
		fmt.Println("Nenhuma mudança detectada nos models.")
		return nil
	}

	num, err := migNextNum("database/migrations")
	if err != nil {
		return err
	}

	outPath := filepath.Join("database", "migrations", num+"_auto.sql")
	sql := migGenerateSQL(pending, alters, driver)

	if err := os.WriteFile(outPath, []byte(sql), 0644); err != nil {
		return fmt.Errorf("escrever %s: %w", outPath, err)
	}

	fmt.Printf("Criada: %s\n", outPath)
	for _, m := range pending {
		fmt.Printf("  + %s → tabela %s\n", m.Name, m.Table)
	}
	for _, a := range alters {
		for _, f := range a.Add {
			fmt.Printf("  ~ %s → ADD COLUMN %s\n", a.Table, f.Column)
		}
	}
	fmt.Println("\nRevisione o arquivo antes de executar 'go run main.go migrate'.")
	return nil
}

// migAlter descreve as diferenças entre um model e o schema das migrations.
type migAlter struct {
	Table    string
	PKColumn string     // coluna da PK da tabela — necessária pro DDL de FTS no SQLite
	Add      []migField // colunas novas no model → ALTER TABLE ADD COLUMN
	Removed  []string   // colunas nas migrations que sumiram do model → só aviso
}

// migDiffModel compara os campos do model com as colunas conhecidas da tabela.
// Retorna nil quando não há diferença.
func migDiffModel(m migModel, cols map[string]bool) *migAlter {
	a := migAlter{Table: m.Table, PKColumn: migPKColumn(m)}
	modelCols := make(map[string]bool, len(m.Fields))
	for _, f := range m.Fields {
		modelCols[f.Column] = true
		if !cols[f.Column] {
			if f.IsPK {
				fmt.Printf("  aviso: PK '%s.%s' não encontrada nas migrations — mudança de PK exige migration manual\n", m.Table, f.Column)
				continue
			}
			a.Add = append(a.Add, f)
		}
	}
	for c := range cols {
		if !modelCols[c] {
			a.Removed = append(a.Removed, c)
		}
	}
	sort.Strings(a.Removed)
	if len(a.Add) == 0 && len(a.Removed) == 0 {
		return nil
	}
	return &a
}

// ── tipos internos ────────────────────────────────────────────────────────────

type migField struct {
	Column  string
	GoType  string
	Size    int
	IsPK    bool
	NotNull bool
	Unique  bool
	FK      string // kyrux:"fk:tabela" → REFERENCES tabela(id) + índice
	FTS     bool   // kyrux:"fts" → índice/tabela de busca full-text
}

type migModel struct {
	Name   string
	Table  string
	Fields []migField
}

// ── parsing de arquivos Go via AST ────────────────────────────────────────────

func migParseFile(path string) ([]migModel, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}

	var models []migModel
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}

		m := migModel{
			Name:  ts.Name.Name,
			Table: migPluralSnake(ts.Name.Name),
		}

		hasPK := false
		for _, astField := range st.Fields.List {
			if len(astField.Names) == 0 {
				continue // campo embutido
			}

			goType := migExprType(astField.Type)
			isPtr := strings.HasPrefix(goType, "*")
			if isPtr {
				goType = goType[1:]
			}

			kyruxTag := ""
			if astField.Tag != nil {
				kyruxTag = migExtractTag(strings.Trim(astField.Tag.Value, "`"), "kyrux")
			}

			for _, ident := range astField.Names {
				fd := migBuildField(ident.Name, goType, kyruxTag, !isPtr)
				m.Fields = append(m.Fields, fd)
				if fd.IsPK {
					hasPK = true
				}
			}
		}

		if hasPK {
			models = append(models, m)
		}
		return true
	})
	return models, nil
}

func migBuildField(name, goType, kyruxTag string, notNull bool) migField {
	fd := migField{
		Column:  migToSnake(name),
		GoType:  goType,
		NotNull: notNull,
	}
	for _, part := range strings.Split(kyruxTag, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "pk":
			fd.IsPK = true
		case part == "unique" || part == "unique:true":
			fd.Unique = true
		case part == "fts":
			fd.FTS = true
		case strings.HasPrefix(part, "column:"):
			fd.Column = strings.TrimPrefix(part, "column:")
		case strings.HasPrefix(part, "size:"):
			fd.Size, _ = strconv.Atoi(strings.TrimPrefix(part, "size:"))
		case strings.HasPrefix(part, "fk:"):
			fd.FK = strings.TrimPrefix(part, "fk:")
		}
	}
	return fd
}

// ── geração de SQL ────────────────────────────────────────────────────────────

func migGenerateSQL(models []migModel, alters []migAlter, driver string) string {
	isPostgres := driver == "postgres" || driver == "pgx"

	var sb strings.Builder
	sb.WriteString("-- Migração gerada automaticamente pelo Kyrux Framework\n")
	sb.WriteString("-- Revisione antes de executar: go run main.go migrate\n\n")

	for i, m := range models {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "CREATE TABLE IF NOT EXISTS %s (\n", m.Table)

		for j, f := range m.Fields {
			sqlType := migSQLType(f, isPostgres)
			constraints := migConstraints(f, isPostgres)
			comma := ","
			if j == len(m.Fields)-1 {
				comma = ""
			}
			fmt.Fprintf(&sb, "    %-20s %s%s%s\n", f.Column, sqlType, constraints, comma)
		}
		sb.WriteString(");\n")

		for _, f := range m.Fields {
			if f.Unique && !f.IsPK {
				fmt.Fprintf(&sb, "\nCREATE UNIQUE INDEX IF NOT EXISTS %s_%s_idx ON %s (%s);\n",
					m.Table, f.Column, m.Table, f.Column)
			}
			// FK sempre ganha índice — JOINs e deletes em cascata dependem dele.
			if f.FK != "" && !f.Unique {
				fmt.Fprintf(&sb, "\nCREATE INDEX IF NOT EXISTS %s_%s_idx ON %s (%s);\n",
					m.Table, f.Column, m.Table, f.Column)
			}
			if f.FTS {
				sb.WriteString(migFTSUpSQL(m.Table, migPKColumn(m), f, driver))
			}
		}
	}

	// Colunas novas em tabelas existentes (autodetectadas).
	for _, a := range alters {
		if len(a.Add) > 0 {
			fmt.Fprintf(&sb, "\n-- Colunas novas em %s\n", a.Table)
			for _, f := range a.Add {
				fmt.Fprintf(&sb, "ALTER TABLE %s ADD COLUMN %s %s%s;\n",
					a.Table, f.Column, migSQLType(f, isPostgres), migAlterConstraints(f, isPostgres))
				if f.Unique {
					fmt.Fprintf(&sb, "CREATE UNIQUE INDEX IF NOT EXISTS %s_%s_idx ON %s (%s);\n",
						a.Table, f.Column, a.Table, f.Column)
				} else if f.FK != "" {
					fmt.Fprintf(&sb, "CREATE INDEX IF NOT EXISTS %s_%s_idx ON %s (%s);\n",
						a.Table, f.Column, a.Table, f.Column)
				}
				if f.FTS {
					sb.WriteString(migFTSUpSQL(a.Table, a.PKColumn, f, driver))
				}
			}
		}
		if len(a.Removed) > 0 {
			fmt.Fprintf(&sb, "\n-- AVISO: colunas presentes nas migrations mas ausentes do model '%s'.\n", a.Table)
			sb.WriteString("-- A remoção NÃO é gerada automaticamente (perda de dados) — descomente para aplicar:\n")
			for _, c := range a.Removed {
				fmt.Fprintf(&sb, "-- ALTER TABLE %s DROP COLUMN %s;\n", a.Table, c)
			}
		}
	}

	// Seção down: desfaz tudo na ordem inversa (alters antes das tabelas).
	sb.WriteString("\n-- down\n")
	for i := len(alters) - 1; i >= 0; i-- {
		a := alters[i]
		for j := len(a.Add) - 1; j >= 0; j-- {
			f := a.Add[j]
			if f.FTS {
				sb.WriteString(migFTSDownSQL(a.Table, f, driver))
			}
			if f.Unique || f.FK != "" {
				fmt.Fprintf(&sb, "DROP INDEX IF EXISTS %s_%s_idx;\n", a.Table, f.Column)
			}
			fmt.Fprintf(&sb, "ALTER TABLE %s DROP COLUMN %s;\n", a.Table, f.Column)
		}
	}
	for i := len(models) - 1; i >= 0; i-- {
		m := models[i]
		for _, f := range m.Fields {
			if f.FTS {
				sb.WriteString(migFTSDownSQL(m.Table, f, driver))
			}
			if (f.Unique || f.FK != "") && !f.IsPK {
				fmt.Fprintf(&sb, "DROP INDEX IF EXISTS %s_%s_idx;\n", m.Table, f.Column)
			}
		}
		fmt.Fprintf(&sb, "DROP TABLE IF EXISTS %s;\n", m.Table)
	}

	return sb.String()
}

// migPKColumn retorna a coluna da chave primária do model — "id" como
// fallback (não deveria acontecer: migParseFile só inclui models com PK).
func migPKColumn(m migModel) string {
	for _, f := range m.Fields {
		if f.IsPK {
			return f.Column
		}
	}
	return "id"
}

// migFTSShadowTable é o nome da tabela virtual FTS5 (SQLite) para table.coluna.
func migFTSShadowTable(table, col string) string {
	return table + "_" + col + "_fts"
}

// migFTSUpSQL gera o DDL que habilita busca full-text na coluna, de acordo
// com o driver — cada um com seu próprio mecanismo nativo (ver Query.Search
// em core/orm/query.go). Em driver não suportado, gera só um aviso
// comentado: não existe fallback silencioso para LIKE.
func migFTSUpSQL(table, pkCol string, f migField, driver string) string {
	var sb strings.Builder
	switch driver {
	case "postgres", "pgx":
		fmt.Fprintf(&sb, "\nCREATE INDEX IF NOT EXISTS %s_%s_fts_idx ON %s USING GIN (to_tsvector('portuguese', %s));\n",
			table, f.Column, table, f.Column)
	case "mysql":
		fmt.Fprintf(&sb, "\nCREATE FULLTEXT INDEX %s_%s_fts_idx ON %s (%s);\n",
			table, f.Column, table, f.Column)
	case "sqlite", "sqlite3":
		shadow := migFTSShadowTable(table, f.Column)
		fmt.Fprintf(&sb, "\nCREATE VIRTUAL TABLE IF NOT EXISTS %s USING fts5(%s, content='%s', content_rowid='%s');\n",
			shadow, f.Column, table, pkCol)
		fmt.Fprintf(&sb, "CREATE TRIGGER IF NOT EXISTS %s_ai AFTER INSERT ON %s BEGIN\n  INSERT INTO %s(rowid, %s) VALUES (new.%s, new.%s);\nEND;\n",
			shadow, table, shadow, f.Column, pkCol, f.Column)
		fmt.Fprintf(&sb, "CREATE TRIGGER IF NOT EXISTS %s_ad AFTER DELETE ON %s BEGIN\n  INSERT INTO %s(%s, rowid, %s) VALUES('delete', old.%s, old.%s);\nEND;\n",
			shadow, table, shadow, shadow, f.Column, pkCol, f.Column)
		fmt.Fprintf(&sb, "CREATE TRIGGER IF NOT EXISTS %s_au AFTER UPDATE ON %s BEGIN\n  INSERT INTO %s(%s, rowid, %s) VALUES('delete', old.%s, old.%s);\n  INSERT INTO %s(rowid, %s) VALUES (new.%s, new.%s);\nEND;\n",
			shadow, table, shadow, shadow, f.Column, pkCol, f.Column, shadow, f.Column, pkCol, f.Column)
	default:
		fmt.Fprintf(&sb, "\n-- AVISO: kyrux:\"fts\" em %s.%s ignorado — full-text search não suportado no driver %q (suportado: postgres, mysql, sqlite)\n",
			table, f.Column, driver)
	}
	return sb.String()
}

// migFTSDownSQL desfaz o DDL gerado por migFTSUpSQL.
func migFTSDownSQL(table string, f migField, driver string) string {
	var sb strings.Builder
	switch driver {
	case "postgres", "pgx":
		fmt.Fprintf(&sb, "DROP INDEX IF EXISTS %s_%s_fts_idx;\n", table, f.Column)
	case "mysql":
		fmt.Fprintf(&sb, "DROP INDEX %s_%s_fts_idx ON %s;\n", table, f.Column, table)
	case "sqlite", "sqlite3":
		shadow := migFTSShadowTable(table, f.Column)
		fmt.Fprintf(&sb, "DROP TRIGGER IF EXISTS %s_ai;\n", shadow)
		fmt.Fprintf(&sb, "DROP TRIGGER IF EXISTS %s_ad;\n", shadow)
		fmt.Fprintf(&sb, "DROP TRIGGER IF EXISTS %s_au;\n", shadow)
		fmt.Fprintf(&sb, "DROP TABLE IF EXISTS %s;\n", shadow)
	}
	return sb.String()
}

// migAlterConstraints monta as constraints de um ADD COLUMN.
// Diferente do CREATE TABLE, não emite UNIQUE inline — o SQLite não aceita
// UNIQUE em ADD COLUMN; o índice único é criado em statement separado.
func migAlterConstraints(f migField, isPostgres bool) string {
	var parts []string
	if f.NotNull {
		parts = append(parts, "NOT NULL")
		if def := migDefault(f.GoType, isPostgres); def != "" {
			parts = append(parts, "DEFAULT "+def)
		}
	}
	if f.FK != "" {
		parts = append(parts, fmt.Sprintf("REFERENCES %s(id)", f.FK))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func migSQLType(f migField, isPostgres bool) string {
	if f.IsPK {
		if isPostgres {
			return "BIGSERIAL"
		}
		return "INTEGER"
	}
	switch f.GoType {
	case "int", "int32":
		return "INTEGER"
	case "int64":
		if isPostgres {
			return "BIGINT"
		}
		return "INTEGER"
	case "float32", "float64":
		return "DECIMAL"
	case "bool":
		return "BOOLEAN"
	case "time.Time":
		if isPostgres {
			return "TIMESTAMPTZ"
		}
		return "DATETIME"
	default: // string e outros
		if f.Size > 0 {
			return fmt.Sprintf("VARCHAR(%d)", f.Size)
		}
		return "TEXT"
	}
}

func migConstraints(f migField, isPostgres bool) string {
	if f.IsPK {
		return " PRIMARY KEY"
	}
	var parts []string
	if f.NotNull {
		parts = append(parts, "NOT NULL")
		def := migDefault(f.GoType, isPostgres)
		if def != "" {
			parts = append(parts, "DEFAULT "+def)
		}
	}
	if f.Unique {
		parts = append(parts, "UNIQUE")
	}
	// FK: a tabela referenciada precisa existir antes — declare o model
	// referenciado primeiro (ou em migration anterior).
	if f.FK != "" {
		parts = append(parts, fmt.Sprintf("REFERENCES %s(id)", f.FK))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func migDefault(goType string, isPostgres bool) string {
	switch goType {
	case "bool":
		return "FALSE"
	case "int", "int32", "int64", "float32", "float64":
		return "0"
	case "time.Time":
		if isPostgres {
			return "NOW()"
		}
		return "CURRENT_TIMESTAMP"
	default: // string
		return "''"
	}
}

// ── helpers de migration directory ───────────────────────────────────────────

func migNextNum(dir string) (string, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.sql"))
	max := 0
	for _, f := range files {
		base := filepath.Base(f)
		parts := strings.SplitN(base, "_", 2)
		if n, err := strconv.Atoi(parts[0]); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("%04d", max+1), nil
}

var (
	reAlterAdd  = regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+"?([a-zA-Z_][\w.]*)"?\s+ADD\s+(?:COLUMN\s+)?"?([a-zA-Z_]\w*)"?`)
	reAlterDrop = regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+"?([a-zA-Z_][\w.]*)"?\s+DROP\s+(?:COLUMN\s+)?"?([a-zA-Z_]\w*)"?`)
)

// migConstraintKeywords são inícios de linha dentro de CREATE TABLE que não
// declaram coluna.
var migConstraintKeywords = map[string]bool{
	"PRIMARY": true, "FOREIGN": true, "UNIQUE": true,
	"CONSTRAINT": true, "CHECK": true, "KEY": true, "INDEX": true,
}

// migSchemaInDir reconstrói o schema conhecido a partir das migrations:
// tabela → conjunto de colunas. Considera CREATE TABLE e ALTER TABLE
// ADD/DROP COLUMN, e ignora a seção "-- down" de cada arquivo.
func migSchemaInDir(dir string) (map[string]map[string]bool, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.sql"))
	sort.Strings(files) // ordem de aplicação: ADD/DROP posteriores prevalecem
	schema := make(map[string]map[string]bool)

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		inTable := ""
		for _, line := range strings.Split(string(content), "\n") {
			trim := strings.TrimSpace(line)
			if trim == "" {
				continue
			}
			if strings.HasPrefix(trim, "--") {
				// A seção down desfaz o up — processá-la anularia o schema.
				if strings.EqualFold(trim, "-- down") {
					break
				}
				continue
			}

			if inTable != "" {
				if strings.HasPrefix(trim, ")") {
					inTable = ""
					continue
				}
				fields := strings.Fields(trim)
				if len(fields) == 0 {
					continue
				}
				col := strings.ToLower(strings.Trim(fields[0], `",`))
				if col == "" || migConstraintKeywords[strings.ToUpper(col)] {
					continue
				}
				schema[inTable][col] = true
				continue
			}

			upper := strings.ToUpper(trim)
			matched := false
			for _, prefix := range []string{"CREATE TABLE IF NOT EXISTS ", "CREATE TABLE "} {
				if strings.HasPrefix(upper, prefix) {
					rest := trim[len(prefix):]
					fields := strings.FieldsFunc(rest, func(r rune) bool {
						return r == ' ' || r == '\t' || r == '(' || r == '"' || r == '\''
					})
					if len(fields) > 0 {
						inTable = strings.ToLower(fields[0])
						if schema[inTable] == nil {
							schema[inTable] = make(map[string]bool)
						}
					}
					matched = true
					break
				}
			}
			if matched {
				continue
			}

			if m := reAlterAdd.FindStringSubmatch(trim); m != nil {
				t, c := strings.ToLower(m[1]), strings.ToLower(m[2])
				if schema[t] == nil {
					schema[t] = make(map[string]bool)
				}
				schema[t][c] = true
			} else if m := reAlterDrop.FindStringSubmatch(trim); m != nil {
				t, c := strings.ToLower(m[1]), strings.ToLower(m[2])
				if schema[t] != nil {
					delete(schema[t], c)
				}
			}
		}
	}
	return schema, nil
}

// ── helpers de AST ───────────────────────────────────────────────────────────

func migExprType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return migExprType(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + migExprType(t.X)
	case *ast.ArrayType:
		return "[]" + migExprType(t.Elt)
	default:
		return "string"
	}
}

func migExtractTag(raw, key string) string {
	search := key + `:"`
	idx := strings.Index(raw, search)
	if idx < 0 {
		return ""
	}
	rest := raw[idx+len(search):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// ── helpers de nomenclatura (espelho do ORM) ──────────────────────────────────

// migToSnake espelha orm.toSnake, com tratamento de acrônimos (UserID → user_id).
func migToSnake(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i, r := range rs {
		if unicode.IsUpper(r) {
			if i > 0 && (!unicode.IsUpper(rs[i-1]) || (i+1 < len(rs) && unicode.IsLower(rs[i+1]))) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func migPluralSnake(name string) string {
	s := migToSnake(name)
	switch {
	case strings.HasSuffix(s, "s") ||
		strings.HasSuffix(s, "x") ||
		strings.HasSuffix(s, "z") ||
		strings.HasSuffix(s, "sh") ||
		strings.HasSuffix(s, "ch"):
		return s + "es"
	case strings.HasSuffix(s, "y") && len(s) > 1 && !migIsVowel(rune(s[len(s)-2])):
		return s[:len(s)-1] + "ies"
	default:
		return s + "s"
	}
}

func migIsVowel(r rune) bool {
	return strings.ContainsRune("aeiou", r)
}
