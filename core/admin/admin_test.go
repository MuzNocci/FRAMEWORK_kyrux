package admin

import (
	"testing"
	"time"
)

type testProduto struct {
	ID        int64   `kyrux:"pk"`
	Nome      string  `kyrux:"size:100"`
	Email     *string `kyrux:"size:254"`
	Ativo     bool
	Senha     string `kyrux:"hash"`
	Preco     float64
	CriadoEm  time.Time
	UpdatedAt time.Time `kyrux:"column:updated_at,autonow"`
}

type testSemPK struct {
	Nome string
}

func resetRegistry() {
	registryMu.Lock()
	registry = map[string]*registeredModel{}
	order = nil
	registryMu.Unlock()
}

func TestRegisterEModelBySlug(t *testing.T) {
	resetRegistry()
	Register[testProduto]("produtos", "Produtos")

	if Count() != 1 {
		t.Fatalf("esperava 1 model registrado, recebeu %d", Count())
	}
	rm, ok := modelBySlug("produtos")
	if !ok {
		t.Fatal("produtos deveria estar registrado")
	}
	if rm.Label != "Produtos" || rm.Conn != "default" || rm.pkColumn != "id" {
		t.Errorf("registro incorreto: %+v", rm)
	}
}

func TestRegisterSlugInvalidoPanica(t *testing.T) {
	resetRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para slug inválido")
		}
	}()
	Register[testProduto]("Produtos!", "Produtos")
}

func TestRegisterSlugReservadoPanica(t *testing.T) {
	resetRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para slug reservado")
		}
	}()
	Register[testProduto]("login", "Login")
}

func TestRegisterSlugDuplicadoPanica(t *testing.T) {
	resetRegistry()
	Register[testProduto]("produtos", "Produtos")
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para slug duplicado")
		}
	}()
	Register[testProduto]("produtos", "Produtos 2")
}

func TestRegisterSemPKPanica(t *testing.T) {
	resetRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para model sem PK")
		}
	}()
	Register[testSemPK]("sem-pk", "Sem PK")
}

func TestListFieldsEDefaultExcluiHash(t *testing.T) {
	resetRegistry()
	Register[testProduto]("produtos", "Produtos")
	rm, _ := modelBySlug("produtos")

	for _, c := range rm.listCols {
		if c == "senha" {
			t.Error("listCols default não deveria incluir o campo hash")
		}
	}

	resetRegistry()
	Register[testProduto]("produtos", "Produtos", ListFields("Nome", "Ativo"))
	rm, _ = modelBySlug("produtos")
	if len(rm.listCols) != 2 || rm.listCols[0] != "nome" || rm.listCols[1] != "ativo" {
		t.Errorf("ListFields não respeitado: %v", rm.listCols)
	}
}

func TestListFieldsCampoInexistentePanica(t *testing.T) {
	resetRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para campo inexistente em ListFields")
		}
	}()
	Register[testProduto]("produtos", "Produtos", ListFields("NaoExiste"))
}

func TestSearchFieldsPadraoVazio(t *testing.T) {
	resetRegistry()
	Register[testProduto]("produtos", "Produtos")
	rm, _ := modelBySlug("produtos")
	if len(rm.searchCols) != 0 {
		t.Errorf("sem SearchFields, searchCols deveria ser vazio, recebeu %v", rm.searchCols)
	}

	resetRegistry()
	Register[testProduto]("produtos", "Produtos", SearchFields("Nome"))
	rm, _ = modelBySlug("produtos")
	if len(rm.searchCols) != 1 || rm.searchCols[0] != "nome" {
		t.Errorf("SearchFields não respeitado: %v", rm.searchCols)
	}
}

func TestConnOption(t *testing.T) {
	resetRegistry()
	Register[testProduto]("produtos", "Produtos", Conn("analytics"))
	rm, _ := modelBySlug("produtos")
	if rm.Conn != "analytics" {
		t.Errorf("Conn não aplicado: %q", rm.Conn)
	}
}

func TestDetectWidget(t *testing.T) {
	rm := struct{}{}
	_ = rm
	resetRegistry()
	Register[testProduto]("produtos", "Produtos")
	reg, _ := modelBySlug("produtos")
	byName := make(map[string]adminField, len(reg.Fields))
	for _, f := range reg.Fields {
		byName[f.GoName] = f
	}

	cases := map[string]struct {
		widget   string
		optional bool
	}{
		"Nome":      {"text", false},
		"Email":     {"text", true}, // ponteiro *string → text, optional
		"Ativo":     {"checkbox", false},
		"Senha":     {"password", false},
		"Preco":     {"number-float", false},
		"CriadoEm":  {"datetime", false},
		"UpdatedAt": {"datetime", false},
	}
	for name, want := range cases {
		f, ok := byName[name]
		if !ok {
			t.Fatalf("campo %s não encontrado", name)
		}
		if f.Widget != want.widget || f.Optional != want.optional {
			t.Errorf("%s: esperava widget=%q optional=%v, recebeu widget=%q optional=%v",
				name, want.widget, want.optional, f.Widget, f.Optional)
		}
	}
}

func TestHumanize(t *testing.T) {
	cases := map[string]string{
		"nome":       "Nome",
		"criado_em":  "Criado Em",
		"user_group": "User Group",
		"id":         "Id",
	}
	for in, want := range cases {
		if got := humanize(in); got != want {
			t.Errorf("humanize(%q) = %q, want %q", in, got, want)
		}
	}
}
