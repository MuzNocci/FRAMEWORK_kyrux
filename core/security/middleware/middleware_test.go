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
