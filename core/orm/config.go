package orm

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"kyrux/core/database"
)

// DBConfig descreve uma entrada de DATABASES.
type DBConfig struct {
	Enabled  bool   `json:"enabled"`
	Driver   string `json:"driver"`
	Servidor string `json:"servidor"`
}

// LoadDatabases lê DATABASES, abre as conexões, registra no registry global
// e retorna um *database.Manager com as mesmas conexões (para fw.DB no bootstrap).
//
// Faz panic em qualquer erro — a aplicação não deve subir com configuração inválida.
// Para apps sem banco de dados, use DATABASES={}.
func LoadDatabases() *database.Manager {
	raw := os.Getenv("DATABASES")
	if raw == "" {
		panic("orm: variável de ambiente DATABASES não definida")
	}

	var configs map[string]DBConfig
	if err := json.Unmarshal([]byte(raw), &configs); err != nil {
		panic(fmt.Sprintf("orm: DATABASES inválido: %v", err))
	}

	mgr := database.NewManager()

	for name, cfg := range configs {
		if !cfg.Enabled {
			log.Printf("orm: database '%s' desabilitada\n", name)
			continue
		}
		if cfg.Driver == "" {
			panic(fmt.Sprintf("orm: database '%s': campo driver não definido", name))
		}
		if cfg.Servidor == "" {
			panic(fmt.Sprintf("orm: database '%s': campo servidor não definido", name))
		}

		db, err := database.Open(cfg.Driver, cfg.Servidor)
		if err != nil {
			panic(fmt.Sprintf("orm: database '%s': %v", name, err))
		}

		Register(name, db)
		mgr.AddDB(name, db)
		log.Printf("orm: database '%s' conectada (%s)\n", name, cfg.Driver)
	}

	return mgr
}
