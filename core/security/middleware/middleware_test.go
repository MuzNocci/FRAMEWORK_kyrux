package middleware

import (
	"kyrux/core/router"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimit(t *testing.T) {
	limited := RateLimit(2, time.Minute)(func(ctx *router.Context) {
		ctx.Writer.WriteHeader(http.StatusOK)
	})

	do := func(remoteAddr string) int {
		req := httptest.NewRequest("POST", "/login/", nil)
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		limited(&router.Context{Writer: rec, Request: req})
		return rec.Code
	}

	// Duas primeiras passam, terceira bloqueia.
	for i := 1; i <= 2; i++ {
		if code := do("10.0.0.1:5000"); code != http.StatusOK {
			t.Fatalf("req %d: esperava 200, recebeu %d", i, code)
		}
	}
	if code := do("10.0.0.1:5000"); code != http.StatusTooManyRequests {
		t.Fatalf("req 3: esperava 429, recebeu %d", code)
	}

	// IP diferente tem janela própria — inclusive IPv6.
	if code := do("[::1]:5000"); code != http.StatusOK {
		t.Fatalf("outro IP: esperava 200, recebeu %d", code)
	}
}

func TestAllowedHostsIPv6(t *testing.T) {
	mw := AllowedHosts([]string{"::1", "meusite.com.br"}, false)
	handler := mw(func(ctx *router.Context) {
		ctx.Writer.WriteHeader(http.StatusOK)
	})

	do := func(host string) int {
		req := httptest.NewRequest("GET", "/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		handler(&router.Context{Writer: rec, Request: req})
		return rec.Code
	}

	cases := []struct {
		host string
		want int
	}{
		{"[::1]:8000", http.StatusOK},
		{"[::1]", http.StatusOK},
		{"meusite.com.br", http.StatusOK},
		{"meusite.com.br:443", http.StatusOK},
		{"evil.com", http.StatusBadRequest},
	}
	for _, c := range cases {
		if got := do(c.host); got != c.want {
			t.Errorf("host %q: esperava %d, recebeu %d", c.host, c.want, got)
		}
	}
}

func TestSecureHeadersUsaDefaultCSPSemSetCSP(t *testing.T) {
	t.Cleanup(func() { SetCSP(DefaultCSP) })
	SetCSP(DefaultCSP) // garante estado limpo mesmo se outro teste rodou antes

	handler := SecureHeaders(func(ctx *router.Context) {
		ctx.Writer.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler(&router.Context{Writer: rec, Request: req})

	if got := rec.Header().Get("Content-Security-Policy"); got != DefaultCSP {
		t.Fatalf("CSP = %q, esperava o default %q", got, DefaultCSP)
	}
	// Confirma que os outros cabeçalhos de segurança continuam saindo —
	// a mudança pra CSP configurável não pode ter derrubado os fixos.
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("X-Frame-Options ausente depois da refatoração de SecureHeaders")
	}
}

func TestSetCSPTrocaAPolicyGlobal(t *testing.T) {
	t.Cleanup(func() { SetCSP(DefaultCSP) })

	custom := "default-src 'self'; script-src 'self' https://exemplo.com"
	SetCSP(custom)

	if got := CSP(); got != custom {
		t.Fatalf("CSP() = %q, esperava %q", got, custom)
	}

	handler := SecureHeaders(func(ctx *router.Context) {
		ctx.Writer.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler(&router.Context{Writer: rec, Request: req})

	if got := rec.Header().Get("Content-Security-Policy"); got != custom {
		t.Fatalf("Content-Security-Policy = %q, esperava %q", got, custom)
	}
}

func TestSetCSPComPolicyVaziaNaoAlteraOValorAtual(t *testing.T) {
	t.Cleanup(func() { SetCSP(DefaultCSP) })

	custom := "default-src 'none'"
	SetCSP(custom)
	SetCSP("") // não deve apagar o que já estava configurado

	if got := CSP(); got != custom {
		t.Fatalf("CSP() = %q depois de SetCSP(\"\"), esperava manter %q", got, custom)
	}
}

func TestCSPOverrideSubstituiOHeaderDoSecureHeaders(t *testing.T) {
	t.Cleanup(func() { SetCSP(DefaultCSP) })
	SetCSP(DefaultCSP)

	routePolicy := "default-src 'self'; frame-src https://www.google.com/maps/"

	// Cadeia real: SecureHeaders (global) por fora, CSPOverride (por rota)
	// por dentro — mesma ordem de execução que router.Router.chain produz
	// (global primeiro, depois o handler já embrulhado pela rota).
	handler := SecureHeaders(CSPOverride(routePolicy)(func(ctx *router.Context) {
		ctx.Writer.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/contato/", nil)
	rec := httptest.NewRecorder()
	handler(&router.Context{Writer: rec, Request: req})

	if got := rec.Header().Get("Content-Security-Policy"); got != routePolicy {
		t.Fatalf("Content-Security-Policy = %q, esperava o override da rota %q", got, routePolicy)
	}
}

func TestCSPOverrideNaoAfetaOutrasRotas(t *testing.T) {
	t.Cleanup(func() { SetCSP(DefaultCSP) })
	SetCSP(DefaultCSP)

	routePolicy := "default-src 'self'; frame-src https://www.google.com/maps/"
	overridden := SecureHeaders(CSPOverride(routePolicy)(func(ctx *router.Context) {
		ctx.Writer.WriteHeader(http.StatusOK)
	}))
	plain := SecureHeaders(func(ctx *router.Context) {
		ctx.Writer.WriteHeader(http.StatusOK)
	})

	recOverridden := httptest.NewRecorder()
	overridden(&router.Context{Writer: recOverridden, Request: httptest.NewRequest("GET", "/contato/", nil)})

	recPlain := httptest.NewRecorder()
	plain(&router.Context{Writer: recPlain, Request: httptest.NewRequest("GET", "/", nil)})

	if got := recOverridden.Header().Get("Content-Security-Policy"); got != routePolicy {
		t.Fatalf("rota com override: CSP = %q, esperava %q", got, routePolicy)
	}
	if got := recPlain.Header().Get("Content-Security-Policy"); got != DefaultCSP {
		t.Fatalf("rota sem override: CSP = %q, esperava o default %q — override vazou pra outra rota", got, DefaultCSP)
	}
}
