package cli

import (
	"fmt"
	"kyrux/core/database"
	dbmigrate "kyrux/core/database/migrate"
	"kyrux/core/environment"
	"os"
	"path/filepath"
	"strings"
)

// ── removemigration ───────────────────────────────────────────────────────────

func runRemoveMigration(migNum string, removeAll bool) error {
	migNum = strings.TrimSpace(migNum)
	if migNum == "" {
		return fmt.Errorf("número da migration não pode estar vazio")
	}

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

	// Apenas remove do disco — sem tocar no banco
	if !removeAll {
		if err := os.Remove(migPath); err != nil {
			return fmt.Errorf("remover arquivo %s: %w", migName, err)
		}
		fmt.Printf("✓ Migration '%s' removida do disco\n", migName)
		fmt.Println("  (para desfazer o schema e remover do banco, use: removemigration <num> all)")
		return nil
	}

	// Remove do banco: executa o down, limpa o registro e apaga o arquivo
	content, err := os.ReadFile(migPath)
	if err != nil {
		return fmt.Errorf("ler arquivo %s: %w", migName, err)
	}

	_, down := dbmigrate.SplitMigration(string(content))
	if strings.TrimSpace(down) == "" {
		return fmt.Errorf(
			"migration '%s' não possui seção '-- down'\n"+
				"  Adicione ao final do arquivo:\n\n"+
				"  -- down\n"+
				"  DROP TABLE IF EXISTS <tabela>;\n",
			migName,
		)
	}

	_ = environment.Load(".env")

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

	if err := dbmigrate.ExecDown(db, string(content)); err != nil {
		return fmt.Errorf("executar down da migration '%s': %w", migName, err)
	}
	fmt.Printf("✓ Schema revertido pela seção down de '%s'\n", migName)

	if err := dbmigrate.Unrecord(db, migName); err != nil {
		return fmt.Errorf("remover registro de '%s' do banco: %w", migName, err)
	}
	fmt.Printf("✓ Migration '%s' removida de kyrux_migrations\n", migName)

	if err := os.Remove(migPath); err != nil {
		return fmt.Errorf("remover arquivo %s: %w", migName, err)
	}
	fmt.Printf("✓ Arquivo '%s' removido do disco\n", migName)

	return nil
}
