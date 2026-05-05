package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"kyrux/core/environment"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// ── makemigrations ────────────────────────────────────────────────────────────

func runMakeMigrations() error {
	_ = environment.Load(".env")
	driver := environment.GetOr("DB_DRIVER", "postgres")

	appFiles, err := filepath.Glob(filepath.Join("apps", "*", "models", "*.go"))
	if err != nil {
		return fmt.Errorf("listar models de apps: %w", err)
	}
	coreFiles, err := filepath.Glob(filepath.Join("core", "security", "auth", "*.go"))
	if err != nil {
		return fmt.Errorf("listar models de core/security/auth: %w", err)
	}
	files := append(appFiles, coreFiles...)

	existing, err := migTablesInDir("database/migrations")
	if err != nil {
		return fmt.Errorf("ler migrações existentes: %w", err)
	}

	var pending []migModel
	for _, f := range files {
		models, err := migParseFile(f)
		if err != nil {
			fmt.Printf("  aviso: ignorando %s: %v\n", f, err)
			continue
		}
		for _, m := range models {
			if !existing[m.Table] {
				pending = append(pending, m)
			}
		}
	}

	if len(pending) == 0 {
		fmt.Println("Nenhum model novo encontrado.")
		return nil
	}

	num, err := migNextNum("database/migrations")
	if err != nil {
		return err
	}

	outPath := filepath.Join("database", "migrations", num+"_auto.sql")
	sql := migGenerateSQL(pending, driver)

	if err := os.WriteFile(outPath, []byte(sql), 0644); err != nil {
		return fmt.Errorf("escrever %s: %w", outPath, err)
	}

	fmt.Printf("Criada: %s\n", outPath)
	for _, m := range pending {
		fmt.Printf("  + %s → tabela %s\n", m.Name, m.Table)
	}
	fmt.Println("\nRevisione o arquivo antes de executar 'go run main.go migrate'.")
	return nil
}

// ── tipos internos ────────────────────────────────────────────────────────────

type migField struct {
	Column  string
	GoType  string
	Size    int
	IsPK    bool
	NotNull bool
	Unique  bool
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
		case strings.HasPrefix(part, "column:"):
			fd.Column = strings.TrimPrefix(part, "column:")
		case strings.HasPrefix(part, "size:"):
			fd.Size, _ = strconv.Atoi(strings.TrimPrefix(part, "size:"))
		}
	}
	return fd
}

// ── geração de SQL ────────────────────────────────────────────────────────────

func migGenerateSQL(models []migModel, driver string) string {
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

		// índices únicos (exceto PK)
		for _, f := range m.Fields {
			if f.Unique && !f.IsPK {
				fmt.Fprintf(&sb, "\nCREATE UNIQUE INDEX IF NOT EXISTS %s_%s_idx ON %s (%s);\n",
					m.Table, f.Column, m.Table, f.Column)
			}
		}
	}
	return sb.String()
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

func migTablesInDir(dir string) (map[string]bool, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.sql"))
	tables := make(map[string]bool)
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(content), "\n") {
			upper := strings.TrimSpace(strings.ToUpper(line))
			for _, prefix := range []string{"CREATE TABLE IF NOT EXISTS ", "CREATE TABLE "} {
				if strings.HasPrefix(upper, prefix) {
					rest := strings.TrimPrefix(upper, prefix)
					fields := strings.FieldsFunc(rest, func(r rune) bool {
						return r == ' ' || r == '\t' || r == '(' || r == '"' || r == '\''
					})
					if len(fields) > 0 {
						tables[strings.ToLower(fields[0])] = true
					}
					break
				}
			}
		}
	}
	return tables, nil
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

func migToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
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
