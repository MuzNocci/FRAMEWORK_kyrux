package migrate

import (
	"fmt"
	"kyrux/core/database"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Run aplica todas as migrações pendentes do diretório dir.
// Cria a tabela kyrux_migrations se ainda não existir.
// Arquivos já registrados são pulados; os pendentes são executados em ordem alfabética.
// Se uma tabela já existe no banco mesmo que o arquivo de migration não exista,
// apenas registra a migration como aplicada.
func Run(db *database.DB, dir string) error {
	if err := ensureTable(db); err != nil {
		return fmt.Errorf("migrate: criar tabela de controle: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return fmt.Errorf("migrate: listar arquivos: %w", err)
	}
	sort.Strings(files)

	applied, err := appliedMigrations(db)
	if err != nil {
		return fmt.Errorf("migrate: ler migrações aplicadas: %w", err)
	}

	pending := 0
	for _, f := range files {
		name := filepath.Base(f)
		fmt.Printf("  [DEBUG] Processando: %s\n", name)

		if applied[name] {
			fmt.Printf("  ~ %s (já aplicada)\n", name)
			continue
		}

		content, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("migrate: ler %s: %w", name, err)
		}

		// Extrair nomes de tabelas do SQL
		tables := extractTableNames(string(content))
		fmt.Printf("  [DEBUG] Tabelas extraídas: %v\n", tables)

		// Se todas as tabelas já existem no banco, apenas registrar a migration
		allTablesExist := len(tables) > 0
		for _, table := range tables {
			if !tableExists(db, table) {
				allTablesExist = false
				break
			}
		}

		if allTablesExist && len(tables) > 0 {
			// Apenas registrar sem executar SQL
			fmt.Printf("  ⊙ %s (tabelas já existem, apenas registrando)\n", name)
		} else {
			// Executar SQL normalmente
			fmt.Printf("  [DEBUG] Executando SQL para %s\n", name)
			if err := execSQL(db, string(content)); err != nil {
				return fmt.Errorf("migrate: executar %s: %w", name, err)
			}
			fmt.Printf("  ✓ %s\n", name)
		}

		// Registrar a migration no banco de dados
		fmt.Printf("  [DEBUG] Registrando migration %s no banco\n", name)
		if err := record(db, name); err != nil {
			return fmt.Errorf("migrate: registrar %s: %w", name, err)
		}

		pending++
	}

	if pending == 0 && len(files) > 0 {
		fmt.Println("  Nenhuma migração pendente.")
	}
	if len(files) == 0 {
		fmt.Println("  Nenhum arquivo .sql encontrado em", dir)
	}
	return nil
}

// ensureTable cria a tabela kyrux_migrations com sintaxe adequada ao driver.
func ensureTable(db *database.DB) error {
	var ddl string
	switch db.Driver {
	case "postgres", "pgx":
		ddl = `CREATE TABLE IF NOT EXISTS kyrux_migrations (
			id         BIGSERIAL PRIMARY KEY,
			name       VARCHAR(255) NOT NULL UNIQUE,
			applied_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
		)`
	default:
		ddl = `CREATE TABLE IF NOT EXISTS kyrux_migrations (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       VARCHAR(255) NOT NULL UNIQUE,
			applied_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`
	}
	_, err := db.Exec(ddl)
	return err
}

// tableExists verifica se uma tabela existe no banco de dados.
func tableExists(db *database.DB, tableName string) bool {
	var exists bool
	var query string

	switch db.Driver {
	case "postgres", "pgx":
		query = `SELECT EXISTS(
			SELECT 1 FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = $1
		)`
		err := db.QueryRow(query, tableName).Scan(&exists)
		return err == nil && exists
	default:
		query = `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`
		var count int
		err := db.QueryRow(query, tableName).Scan(&count)
		return err == nil && count > 0
	}
}

// appliedMigrations retorna o conjunto de nomes de migrações já aplicadas.
func appliedMigrations(db *database.DB) (map[string]bool, error) {
	rows, err := db.Query("SELECT name FROM kyrux_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}

	if len(applied) > 0 {
		fmt.Printf("  [DEBUG] Migrations aplicadas no banco: %v\n", applied)
	}

	return applied, rows.Err()
}

// record registra uma migração como aplicada.
func record(db *database.DB, name string) error {
	query := "INSERT INTO kyrux_migrations (name) VALUES (?)"
	if db.Driver == "postgres" || db.Driver == "pgx" {
		query = "INSERT INTO kyrux_migrations (name) VALUES ($1)"
	}
	result, err := db.Exec(query, name)
	if err != nil {
		fmt.Printf("  [ERROR] record failed: %v\n", err)
		return err
	}
	rows, _ := result.RowsAffected()
	fmt.Printf("  [DEBUG] Migration '%s' registrada (rows affected: %d)\n", name, rows)
	return nil
}

// execSQL executa um arquivo SQL que pode conter múltiplos statements separados por ";".
// Linhas de comentário e statements vazios são ignorados.
func execSQL(db *database.DB, content string) error {
	for _, stmt := range strings.Split(content, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		// ignora blocos que sejam apenas comentários
		if isComment(stmt) {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func isComment(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		return false
	}
	return true
}

// extractTableNames extrai os nomes das tabelas criadas no SQL.
// Procura por padrões de CREATE TABLE.
func extractTableNames(sql string) []string {
	var tables []string
	upper := strings.ToUpper(sql)
	lower := sql

	// Procurar por CREATE TABLE
	pos := 0
	for {
		idx := strings.Index(upper[pos:], "CREATE TABLE")
		if idx == -1 {
			break
		}
		pos += idx + len("CREATE TABLE")

		// Pular espaços e IF NOT EXISTS
		rest := strings.TrimSpace(upper[pos:])
		restLower := strings.TrimSpace(lower[pos:])

		// Verificar se há IF NOT EXISTS
		if strings.HasPrefix(rest, "IF NOT EXISTS") {
			idx := strings.Index(rest, "IF NOT EXISTS") + len("IF NOT EXISTS")
			rest = strings.TrimSpace(rest[idx:])
			restLower = strings.TrimSpace(restLower[idx:])
		}

		// Extrair nome da tabela (até o próximo espaço ou parêntese)
		endIdx := strings.IndexAny(rest, " (\n")
		if endIdx > 0 {
			tableName := restLower[:endIdx]
			// Remover schema se houver (ex: public.users -> users)
			if dotIdx := strings.Index(tableName, "."); dotIdx >= 0 {
				tableName = tableName[dotIdx+1:]
			}
			fmt.Printf("    [DEBUG] Tabela extraída: '%s'\n", tableName)
			tables = append(tables, tableName)
		}
		pos++
	}
	return tables
}
