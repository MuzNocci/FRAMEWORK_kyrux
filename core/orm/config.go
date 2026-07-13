package orm

import (
	"fmt"
	"log"

	"kyrux/core/database"
	"kyrux/core/settings"
)

// LoadDatabases abre as conexões definidas nos blocos DB_NAME do .env
// (parseados por settings.Load), registra cada uma no registry global e
// retorna um *database.Manager com as mesmas conexões (para fw.DB no bootstrap).
//
// O primeiro banco habilitado também responde pelo nome "default" em
// orm.From/orm.DB, espelhando o comportamento de Manager.Use().
//
// Faz panic em configuração inválida — a aplicação não deve subir com um
// banco habilitado mas mal configurado. Sem blocos DB_NAME (ou com todos
// desabilitados), retorna um Manager vazio e o framework roda sem banco.
func LoadDatabases(configs []settings.DatabaseSettings) *database.Manager {
	mgr := database.NewManager()
	opened := 0

	for _, cfg := range configs {
		if !cfg.Enabled {
			log.Printf("orm: database '%s' desabilitada\n", cfg.Name)
			continue
		}
		if cfg.Name == "" {
			panic("orm: bloco de banco com DB_NAME vazio no .env")
		}
		if cfg.DSN == "" {
			panic(fmt.Sprintf("orm: database '%s': DB_DSN não definido no .env", cfg.Name))
		}

		db, err := database.Open(cfg.Driver, cfg.DSN)
		if err != nil {
			panic(fmt.Sprintf("orm: database '%s': %v", cfg.Name, err))
		}

		Register(cfg.Name, db)
		mgr.AddDB(cfg.Name, db)
		opened++
		log.Printf("orm: database '%s' conectada (%s)\n", cfg.Name, cfg.Driver)
	}

	if opened == 0 {
		log.Println("orm: nenhum banco habilitado — rodando sem banco de dados")
	}
	return mgr
}
