package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"kyrux/core/router"
)

func TestRealIPUsaUltimoDoProxy(t *testing.T) {
	SetTrustedProxyHeader("X-Forwarded-For")
	defer SetTrustedProxyHeader("")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.2:1234"
	// O cliente forja o primeiro valor; o proxy acrescenta o IP real no fim.
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.9")
	if got := RealIP(r); got != "203.0.113.9" {
		t.Fatalf("esperava o IP acrescentado pelo proxy, veio %q", got)
	}
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := RealIP(r); got != "198.51.100.7" {
		t.Fatalf("header com um IP só: veio %q", got)
	}
}

func TestSecureHeadersCompletos(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := &router.Context{Writer: w, Request: httptest.NewRequest("GET", "/", nil)}
	SecureHeaders(func(*router.Context) {})(ctx)
	h := w.Header()
	for _, k := range []string{"Strict-Transport-Security", "Cross-Origin-Opener-Policy", "X-Content-Type-Options", "Referrer-Policy", "Permissions-Policy"} {
		if h.Get(k) == "" {
			t.Errorf("faltou o cabeçalho %s", k)
		}
	}
	csp := h.Get("Content-Security-Policy")
	for _, d := range []string{"object-src 'none'", "base-uri 'self'", "form-action 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, d) {
			t.Errorf("CSP sem %q: %s", d, csp)
		}
	}
}
