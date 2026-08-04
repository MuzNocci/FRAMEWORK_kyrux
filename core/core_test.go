package core

import (
	"context"
	"testing"

	"kyrux/core/container"
	"kyrux/core/registry"
)

// fakeValueModule é um módulo mínimo que produz um valor via Value() —
// simula um adapter real (cachememory, sqlpostgres) sem depender de I/O.
type fakeValueModule struct {
	name  string
	value string
}

func (m *fakeValueModule) Name() string                        { return m.name }
func (m *fakeValueModule) Init(ctx context.Context) error      { return nil }
func (m *fakeValueModule) Configure(ctx context.Context) error { return nil }
func (m *fakeValueModule) Start(ctx context.Context) error     { return nil }
func (m *fakeValueModule) Shutdown(ctx context.Context) error  { return nil }
func (m *fakeValueModule) Value() string                       { return m.value }

func TestUseModuleAtivaEProveNoContainer(t *testing.T) {
	c := New()
	mod := &fakeValueModule{name: "teste.fake", value: "pronto"}

	got, err := UseModule[string](c, mod, "teste.chave")
	if err != nil {
		t.Fatalf("UseModule: %v", err)
	}
	if got != "pronto" {
		t.Errorf("esperava %q, recebeu %q", "pronto", got)
	}

	fromContainer, ok := container.Resolve[string](c.Container, "teste.chave")
	if !ok || fromContainer != "pronto" {
		t.Errorf("esperava o valor disponível no Container sob a chave usada")
	}

	found := false
	for _, m := range c.Modules() {
		if m.Name() == "teste.fake" {
			found = true
		}
	}
	if !found {
		t.Error("módulo ativado deveria aparecer em Modules()")
	}
}

func TestUseComModuloNaoRegistradoDevolveErro(t *testing.T) {
	c := New()
	if _, err := Use[string](c, "core.teste.nao-existe", "chave"); err == nil {
		t.Error("esperava erro ao usar um módulo nunca registrado (sem blank import do adapter)")
	}
}

func TestUseComModuloRegistradoAtivaViaRegistry(t *testing.T) {
	registry.Register("core.teste.registry-helper", func() registry.Module {
		return &fakeValueModule{name: "core.teste.registry-helper", value: "via-registry"}
	})

	c := New()
	got, err := Use[string](c, "core.teste.registry-helper", "chave-registry")
	if err != nil {
		t.Fatalf("Use: %v", err)
	}
	if got != "via-registry" {
		t.Errorf("esperava %q, recebeu %q", "via-registry", got)
	}
}

func TestUseModuleComTipoErradoDevolveErro(t *testing.T) {
	c := New()
	mod := &fakeValueModule{name: "teste.fake2", value: "x"}

	if _, err := UseModule[int](c, mod, "chave"); err == nil {
		t.Error("esperava erro ao pedir um tipo (int) diferente do que o módulo produz (string)")
	}
}

func TestRunEShutdownOrquestramModulosAtivos(t *testing.T) {
	c := New()
	mod := &fakeValueModule{name: "teste.fake3", value: "y"}
	if _, err := UseModule[string](c, mod, "chave3"); err != nil {
		t.Fatalf("UseModule: %v", err)
	}

	if err := c.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := c.Shutdown(); len(errs) != 0 {
		t.Errorf("esperava Shutdown sem erros, recebeu %v", errs)
	}
}
