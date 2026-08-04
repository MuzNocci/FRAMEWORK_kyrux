package container

import "testing"

func TestProvideEResolve(t *testing.T) {
	c := New()
	Provide(c, "saudacao", "olá")

	v, ok := Resolve[string](c, "saudacao")
	if !ok || v != "olá" {
		t.Fatalf("esperava (%q, true), recebeu (%q, %v)", "olá", v, ok)
	}
}

func TestResolveNomeInexistente(t *testing.T) {
	c := New()
	_, ok := Resolve[string](c, "nao-existe")
	if ok {
		t.Error("esperava ok=false para nome nunca fornecido")
	}
}

func TestResolveTipoErradoDevolveFalse(t *testing.T) {
	c := New()
	Provide(c, "numero", 42)

	_, ok := Resolve[string](c, "numero")
	if ok {
		t.Error("esperava ok=false ao resolver com um tipo diferente do fornecido")
	}
}

func TestProvideSobrescreveValorAnterior(t *testing.T) {
	c := New()
	Provide(c, "chave", "primeiro")
	Provide(c, "chave", "segundo")

	v, ok := Resolve[string](c, "chave")
	if !ok || v != "segundo" {
		t.Fatalf("esperava (%q, true), recebeu (%q, %v)", "segundo", v, ok)
	}
}

func TestMustResolveComSucesso(t *testing.T) {
	c := New()
	Provide(c, "chave", 10)

	if got := MustResolve[int](c, "chave"); got != 10 {
		t.Errorf("esperava 10, recebeu %d", got)
	}
}

func TestMustResolveEntraEmPanicSeAusente(t *testing.T) {
	c := New()
	defer func() {
		if r := recover(); r == nil {
			t.Error("esperava panic para MustResolve de um nome ausente")
		}
	}()
	MustResolve[int](c, "nao-existe")
}

func TestNamesListaFornecidos(t *testing.T) {
	c := New()
	Provide(c, "a", 1)
	Provide(c, "b", 2)

	names := c.Names()
	if len(names) != 2 {
		t.Fatalf("esperava 2 nomes, recebeu %v", names)
	}
}
