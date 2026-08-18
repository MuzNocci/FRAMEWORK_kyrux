package admin

// Cobre a separação da navegação lateral entre models do framework (ex:
// auth.User) e models de apps do usuário — ver isFrameworkModel e o campo
// Framework de registeredModel.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kyrux/core/router"
	"kyrux/core/security/auth"
	"kyrux/core/security/session"
)

func TestIsFrameworkModel(t *testing.T) {
	cases := map[string]bool{
		"kyrux/core/security/auth": true,
		"kyrux/core/admin":         true,
		"kyrux/apps/blog/models":   false,
		"kyrux":                    false,
		"":                         false,
	}
	for pkg, want := range cases {
		if got := isFrameworkModel(pkg); got != want {
			t.Errorf("isFrameworkModel(%q) = %v, want %v", pkg, got, want)
		}
	}
}

// testProduto (admin_test.go) está definido no próprio pacote admin
// (kyrux/core/admin) — o que, pela heurística de PkgPath, conta como
// "framework". O que este teste prova é a ligação entre
// structType.PkgPath() e rm.Framework dentro de Register, não a
// classificação em si (já coberta por TestIsFrameworkModel).
func TestRegisterLigaPkgPathAoCampoFramework(t *testing.T) {
	resetRegistry()
	Register[testProduto]("produtos", "Produtos")
	rm, _ := modelBySlug("produtos")
	if !rm.Framework {
		t.Error("model definido em kyrux/core/admin deveria ter Framework=true")
	}
}

func TestBaseSeparaFrameworkEAppModels(t *testing.T) {
	resetRegistry()
	registryMu.Lock()
	registry["usuarios"] = &registeredModel{Slug: "usuarios", Label: "Usuários", Framework: true}
	registry["produtos"] = &registeredModel{Slug: "produtos", Label: "Produtos", Framework: false}
	order = []string{"usuarios", "produtos"}
	registryMu.Unlock()
	t.Cleanup(resetRegistry)

	s := &site{basePath: "/admin/", appName: "Teste", version: "0.0.0", store: session.NewStore(time.Hour)}
	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	ctx := &router.Context{Writer: httptest.NewRecorder(), Request: req}
	ctx.Set(userCtxKey, &auth.User{ID: 1, Username: "admin", IsAdmin: true})

	b := s.base(ctx, "", "Painel")

	if len(b.Models) != 2 {
		t.Fatalf("esperava 2 models em Models (lista completa), recebeu %d: %v", len(b.Models), b.Models)
	}
	if len(b.FrameworkModels) != 1 || b.FrameworkModels[0].Slug != "usuarios" {
		t.Errorf("FrameworkModels incorreto: %v", b.FrameworkModels)
	}
	if len(b.AppModels) != 1 || b.AppModels[0].Slug != "produtos" {
		t.Errorf("AppModels incorreto: %v", b.AppModels)
	}
}

// TestBaseUserPrefereNomeCompleto cobre o texto ao lado do botão Sair no
// header: nome completo quando existir, username só como fallback.
func TestBaseUserPrefereNomeCompleto(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)
	s := &site{basePath: "/admin/", appName: "Teste", version: "0.0.0", store: session.NewStore(time.Hour)}
	newCtx := func(u *auth.User) *router.Context {
		req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
		ctx := &router.Context{Writer: httptest.NewRecorder(), Request: req}
		ctx.Set(userCtxKey, u)
		return ctx
	}

	if b := s.base(newCtx(&auth.User{Username: "admin", FirstName: "Ana", LastName: "Silva"}), "", ""); b.User != "Ana Silva" {
		t.Errorf("com nome completo, esperava %q, recebeu %q", "Ana Silva", b.User)
	}
	if b := s.base(newCtx(&auth.User{Username: "admin", FirstName: "Ana"}), "", ""); b.User != "Ana" {
		t.Errorf("só com FirstName, esperava %q, recebeu %q", "Ana", b.User)
	}
	if b := s.base(newCtx(&auth.User{Username: "admin"}), "", ""); b.User != "admin" {
		t.Errorf("sem nome, esperava fallback para username %q, recebeu %q", "admin", b.User)
	}
}
