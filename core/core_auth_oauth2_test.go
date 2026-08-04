package core_test

// Prova de ponta a ponta do core.Auth.OAuth2: sobe um servidor de
// autorização OAuth2 mínimo (mas real — protocolo HTTP genuíno, não um
// stub que finge a resposta em memória) e executa o fluxo Authorization
// Code completo: AuthCodeURL, troca do código pelo token (Exchange) e uso
// do token para autenticar uma chamada.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"kyrux/core"
)

// newMockOAuth2Server sobe um provedor OAuth2 mínimo: /token troca um code
// válido por um access_token; qualquer outro code é rejeitado (400).
func newMockOAuth2Server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.FormValue("code") != "codigo-valido" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		if r.FormValue("client_id") != "meu-client-id" && r.FormValue("client_secret") != "meu-client-secret" {
			// alguns clients OAuth2 mandam client_id/secret via Basic Auth
			user, pass, ok := r.BasicAuth()
			if !ok || user != "meu-client-id" || pass != "meu-client-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token-de-teste-kyrux",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	})
	return httptest.NewServer(mux)
}

func TestCoreAuthOAuth2FluxoCompleto(t *testing.T) {
	srv := newMockOAuth2Server(t)
	defer srv.Close()

	c := core.New()
	cfg, err := c.Auth.OAuth2(
		"provedor-teste",
		"meu-client-id", "meu-client-secret",
		srv.URL+"/authorize", srv.URL+"/token",
		"https://minhaapp.exemplo.com/callback",
		"email", "profile",
	)
	if err != nil {
		t.Fatalf("Auth.OAuth2: %v", err)
	}

	authURL := cfg.AuthCodeURL("state-de-teste")
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("AuthCodeURL devolveu uma URL inválida: %v", err)
	}
	if !strings.HasPrefix(authURL, srv.URL+"/authorize") {
		t.Errorf("esperava AuthCodeURL apontando pro /authorize do provedor, recebeu %s", authURL)
	}
	q := parsed.Query()
	if q.Get("client_id") != "meu-client-id" {
		t.Errorf("esperava client_id=meu-client-id na URL, recebeu %q", q.Get("client_id"))
	}
	if q.Get("state") != "state-de-teste" {
		t.Errorf("esperava state=state-de-teste na URL, recebeu %q", q.Get("state"))
	}
	if q.Get("scope") != "email profile" {
		t.Errorf("esperava scope=%q, recebeu %q", "email profile", q.Get("scope"))
	}

	// Troca real do código de autorização pelo token — round-trip HTTP de
	// verdade contra o mock server acima.
	token, err := cfg.Exchange(t.Context(), "codigo-valido")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if token.AccessToken != "token-de-teste-kyrux" {
		t.Errorf("esperava access_token=%q, recebeu %q", "token-de-teste-kyrux", token.AccessToken)
	}
	if !token.Valid() {
		t.Error("token devolvido deveria ser válido")
	}

	if _, err := cfg.Exchange(t.Context(), "codigo-invalido"); err == nil {
		t.Error("esperava erro ao trocar um código inválido")
	}
}

func TestCoreAuthOAuth2CredenciaisVaziasFalha(t *testing.T) {
	c := core.New()
	if _, err := c.Auth.OAuth2("teste", "", "", "https://x/authorize", "https://x/token", "https://x/callback"); err == nil {
		t.Error("esperava erro ao ativar OAuth2 sem client id/secret")
	}
}
