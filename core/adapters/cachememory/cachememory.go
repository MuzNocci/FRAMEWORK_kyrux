// Package cachememory é o adapter que expõe core/cache (modo memória) como
// um Module do Core (kyrux/core) — hot-plug puro: importar este pacote
// (mesmo que só com _) já registra "cache.memory" no core/registry; sem
// esse import, core.Cache.Memory() falha com "módulo não registrado".
package cachememory

import (
	"context"

	"kyrux/core/cache"
	"kyrux/core/registry"
)

func init() {
	registry.Register("cache.memory", func() registry.Module { return &Adapter{} })
}

// Adapter implementa registry.Module para um cache em memória.
type Adapter struct {
	cache *cache.Cache
}

func (a *Adapter) Name() string { return "cache.memory" }

func (a *Adapter) Init(ctx context.Context) error { return nil }

// Configure cria o cache em memória (cache.New) — não faz I/O externo, então
// não há nada que possa falhar aqui hoje; o retorno de erro existe só pra
// cumprir a interface Module.
func (a *Adapter) Configure(ctx context.Context) error {
	a.cache = cache.New()
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

func (a *Adapter) Shutdown(ctx context.Context) error {
	if a.cache != nil {
		a.cache.Close()
	}
	return nil
}

// Value devolve o *cache.Cache já pronto — usado por core.Use para extrair
// o produto deste módulo depois de Configure.
func (a *Adapter) Value() *cache.Cache { return a.cache }
