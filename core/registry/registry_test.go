package registry

import (
	"context"
	"testing"
)

type fakeModule struct{ name string }

func (f *fakeModule) Name() string                        { return f.name }
func (f *fakeModule) Init(ctx context.Context) error      { return nil }
func (f *fakeModule) Configure(ctx context.Context) error { return nil }
func (f *fakeModule) Start(ctx context.Context) error     { return nil }
func (f *fakeModule) Shutdown(ctx context.Context) error  { return nil }

func TestRegisterENewDevolveInstanciasDistintas(t *testing.T) {
	name := "registry.teste.instancias"
	Register(name, func() Module { return &fakeModule{name: name} })

	m1, err := New(name)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m2, err := New(name)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m1 == m2 {
		t.Error("New deveria devolver uma instância nova a cada chamada, não reaproveitar")
	}
	if m1.Name() != name || m2.Name() != name {
		t.Error("Name() da instância criada não bate com o nome registrado")
	}
}

func TestNewModuloNaoRegistradoDevolveErro(t *testing.T) {
	if _, err := New("registry.teste.nao-existe"); err == nil {
		t.Error("esperava erro para módulo nunca registrado (sem blank import do adapter)")
	}
}

func TestRegisterDuplicadoEntraEmPanic(t *testing.T) {
	name := "registry.teste.duplicado"
	Register(name, func() Module { return &fakeModule{name: name} })

	defer func() {
		if r := recover(); r == nil {
			t.Error("esperava panic ao registrar o mesmo nome duas vezes")
		}
	}()
	Register(name, func() Module { return &fakeModule{name: name} })
}

func TestNamesListaRegistrados(t *testing.T) {
	name := "registry.teste.names"
	Register(name, func() Module { return &fakeModule{name: name} })

	found := false
	for _, n := range Names() {
		if n == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("esperava %q na lista de Names(), recebeu %v", name, Names())
	}
}
