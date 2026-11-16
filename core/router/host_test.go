package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Rotas com domínio: a raiz do domínio não pode gerar a variante sem barra
// ("GET site.com", inválida no ServeMux), e o url devolve só o caminho.
func TestRotasPorDominio(t *testing.T) {
	r := New()
	responde := func(texto string) HandlerFunc {
		return func(ctx *Context) { ctx.Writer.Write([]byte(texto)) }
	}
	Include(r, []URLPattern{
		Path("GET", "/", responde("site"), "teste_host_site"),
		Path("GET", "interno.exemplo.com/", responde("interno"), "teste_host_raiz"),
		Path("GET", "interno.exemplo.com/entrar/", responde("entrar"), "teste_host_entrar"),
	})

	casos := []struct{ host, caminho, esperado string }{
		{"exemplo.com", "/", "site"},
		{"interno.exemplo.com", "/", "interno"},
		{"interno.exemplo.com", "/entrar/", "entrar"},
		{"interno.exemplo.com", "/entrar", "entrar"},
	}
	for _, c := range casos {
		req := httptest.NewRequest(http.MethodGet, c.caminho, nil)
		req.Host = c.host
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Body.String() != c.esperado {
			t.Errorf("%s%s: veio %q, esperado %q", c.host, c.caminho, w.Body.String(), c.esperado)
		}
	}

	if got := Resolve("teste_host_raiz"); got != "/" {
		t.Errorf("url da raiz do domínio = %q, esperado /", got)
	}
	if got := Resolve("teste_host_entrar"); got != "/entrar/" {
		t.Errorf("url com domínio = %q, esperado /entrar/", got)
	}
}

func TestSlashAlternateComDominio(t *testing.T) {
	casos := map[string]string{
		"GET /{$}":              "",
		"GET site.com/{$}":      "",
		"GET site.com/rota/{$}": "GET site.com/rota",
		"GET site.com/rota":     "GET site.com/rota/{$}",
		"GET /rota/{$}":         "GET /rota",
		"GET site.com/":         "",
	}
	for padrao, esperado := range casos {
		if got := slashAlternate(padrao); got != esperado {
			t.Errorf("slashAlternate(%q) = %q, esperado %q", padrao, got, esperado)
		}
	}
}
