// Package sqlpostgres é o adapter que expõe uma conexão Postgres
// (core/database, driver "postgres") como um Module do Core (kyrux/core).
//
// Ao contrário de cachememory, este adapter recebe parâmetros de construção
// (nome da conexão e DSN) — por isso não se autorregistra via init()/hot-plug
// no core/registry; ele é construído diretamente por core.Database.SQL.Postgres,
// que já importa este pacote (custo zero adicional: core/database já é
// dependência obrigatória do framework, usada por bootstrap.Init
// independente do Core).
//
// Requer que a própria aplicação tenha importado (_) um driver Postgres real
// (ex: github.com/lib/pq) — como qualquer uso de database/sql no Kyrux, este
// adapter nunca importa o driver.
package sqlpostgres

import (
	"context"
	"fmt"

	"kyrux/core/database"
)

// Adapter implementa registry.Module para uma conexão Postgres nomeada.
type Adapter struct {
	name string
	dsn  string
	db   *database.DB
}

// New cria (mas ainda não conecta — isso só acontece em Configure) um
// adapter Postgres. name identifica esta conexão entre outras do mesmo tipo
// (permite múltiplas conexões Postgres simultâneas, cada uma sob seu nome).
func New(name, dsn string) *Adapter {
	return &Adapter{name: name, dsn: dsn}
}

func (a *Adapter) Name() string { return "database.sql.postgres." + a.name }

func (a *Adapter) Init(ctx context.Context) error {
	if a.dsn == "" {
		return fmt.Errorf("sqlpostgres: dsn vazio para a conexão %q", a.name)
	}
	return nil
}

// Configure abre a conexão de verdade — é aqui que um DSN ou servidor
// inválido se manifesta como erro.
func (a *Adapter) Configure(ctx context.Context) error {
	db, err := database.Open("postgres", a.dsn)
	if err != nil {
		return fmt.Errorf("sqlpostgres: %w", err)
	}
	a.db = db
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

func (a *Adapter) Shutdown(ctx context.Context) error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

// Value devolve o *database.DB já pronto.
func (a *Adapter) Value() *database.DB { return a.db }
