package core_test

// Prova de conceito de ponta a ponta: um mesmo Core ativando dois módulos
// de natureza bem diferente ao mesmo tempo — um cache em memória (via
// hot-plug puro, registry.Register/Use) e uma conexão Postgres real (via
// construção direta parametrizada, UseModule) — validando a exigência da
// especificação de que múltiplos bancos/serviços coexistam sem limitação.
// Pulado (t.Skip) se não houver um Postgres acessível.

import (
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"kyrux/core"
	_ "kyrux/core/adapters/cachememory"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestCoreCacheEPostgresCoexistindo(t *testing.T) {
	dsn := envOr("KYRUX_TEST_POSTGRES_DSN", "postgres://postgres:postgres@127.0.0.1:5432/kyrux?sslmode=disable")

	c := core.New()

	cch, err := c.Cache.Memory()
	if err != nil {
		t.Fatalf("Cache.Memory: %v", err)
	}

	db, err := c.Database.SQL.Postgres("principal", dsn)
	if err != nil {
		t.Skipf("postgres indisponível em %s: %v", dsn, err)
	}

	if err := c.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Os dois módulos precisam funcionar de verdade, simultaneamente.
	cch.Set("chave", "valor-cache", time.Minute)
	if v, ok := cch.Get("chave"); !ok || v != "valor-cache" {
		t.Errorf("cache.memory não funcionou junto com o postgres ativo: (%v, %v)", v, ok)
	}

	var um int
	if err := db.QueryRow("SELECT 1").Scan(&um); err != nil {
		t.Fatalf("query no postgres ativado via Core: %v", err)
	}
	if um != 1 {
		t.Errorf("esperava 1, recebeu %d", um)
	}

	// A conexão deve estar disponível via Database.Get, pelo nome usado na ativação.
	got, ok := c.Database.Get("principal")
	if !ok || got != db {
		t.Error("Database.Get(\"principal\") deveria devolver a mesma conexão ativada")
	}

	if len(c.Modules()) != 2 {
		t.Errorf("esperava 2 módulos ativos, recebeu %d: %v", len(c.Modules()), c.Modules())
	}

	if errs := c.Shutdown(); len(errs) != 0 {
		t.Errorf("esperava Shutdown sem erros, recebeu %v", errs)
	}
}

func TestCoreCacheMemorySemImportarAdapterFalha(t *testing.T) {
	// Este teste roda no mesmo binário que já importou cachememory acima
	// (efeito colateral do blank import do pacote) — serve só de
	// documentação executável do comportamento de erro, verificado de
	// forma isolada em core/core_test.go (TestUseComModuloNaoRegistradoDevolveErro)
	// para um nome que garantidamente nunca foi registrado.
	c := core.New()
	if _, err := c.Database.SQL.Postgres("outro", "postgres://usuario-invalido@127.0.0.1:1/db?sslmode=disable"); err == nil {
		t.Error("esperava erro ao ativar Postgres com DSN/servidor inválido")
	}
}
