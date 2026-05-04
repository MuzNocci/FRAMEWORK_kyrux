package router_test

import (
	"net"
	"net/http"
	"testing"

	"kyrux/core/router"
)

// startBenchServer sobe um servidor real em porta aleatória e retorna a URL base.
// Usado para benchmarks externos (ab, wrk). Não é executado em go test normalmente.
func startBenchServer(b *testing.B) (addr string, stop func()) {
	b.Helper()
	r := router.New()
	r.Handle("GET /ping/", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, map[string]string{"pong": "true"})
	})
	r.Handle("GET /usuarios/<id:int>/", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, map[string]string{"id": ctx.Param("id")})
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: r}
	go srv.Serve(ln)
	return ln.Addr().String(), func() { srv.Close() }
}
