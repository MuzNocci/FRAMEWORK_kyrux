package cli

import (
	"fmt"
	"kyrux/core/database"
	"kyrux/core/environment"
	"os"
	"path/filepath"
	"strings"
)

// ── removemigration ───────────────────────────────────────────────────────────

func runRemoveMigration(migNum string, removeAll bool) error {
	// Validar número
	migNum = strings.TrimSpace(migNum)
	if migNum == "" {
		return fmt.Errorf("número da migration não pode estar vazio")
	}

	// Encontrar arquivo da migration
	migDir := "database/migrations"
	files, err := filepath.Glob(filepath.Join(migDir, migNum+"_*.sql"))
	if err != nil {
		return fmt.Errorf("listar migrations: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("nenhuma migration encontrada com número %s", migNum)
	}

	if len(files) > 1 {
		return fmt.Errorf("múltiplas migrations encontradas com número %s: %v", migNum, files)
	}

	migPath := files[0]
	migName := filepath.Base(migPath)

	// Se for apenas remover do disco
	if !removeAll {
		if err := os.Remove(migPath); err != nil {
			return fmt.Errorf("remover arquivo %s: %w", migName, err)
		}
		fmt.Printf("✓ Migration '%s' removida do disco\n", migName)
		fmt.Println("  (para remover do banco, use: removemigration <num> all)")
		return nil
	}

	// Remover do disco e do banco de dados
	_ = environment.Load(".env")

	if environment.GetOr("DB_ENABLED", "false") != "true" {
		return fmt.Errorf("DB_ENABLED=false — banco de dados não configurado, não é possível remover do banco")
	}

	driver := environment.GetOr("DB_DRIVER", "postgres")
	dsn := environment.Get("DB_DSN")
	if dsn == "" {
		return fmt.Errorf("DB_DSN não configurado no .env")
	}

	db, err := database.Open(driver, dsn)
	if err != nil {
		return fmt.Errorf("conectar ao banco: %w", err)
	}
	defer db.Close()

	// Remover do banco de dados
	sqlQuery := "DELETE FROM kyrux_migrations WHERE name = ?"

	// Converter ? para $1 se for PostgreSQL
	if driver == "postgres" || driver == "pgx" {
		sqlQuery = "DELETE FROM kyrux_migrations WHERE name = $1"
	}

	result, err := db.Exec(sqlQuery, migName)
	if err != nil {
		return fmt.Errorf("remover do banco: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("ler rows afetadas: %w", err)
	}

	if rowsAffected == 0 {
		fmt.Printf("⚠ Migration '%s' não encontrada na tabela kyrux_migrations (já removida?)\n", migName)
	} else {
		fmt.Printf("✓ Migration '%s' removida do banco de dados\n", migName)
	}

	// Remover do disco
	if err := os.Remove(migPath); err != nil {
		return fmt.Errorf("remover arquivo %s: %w", migName, err)
	}

	fmt.Printf("✓ Migration '%s' removida do disco\n", migName)
	return nil
}
