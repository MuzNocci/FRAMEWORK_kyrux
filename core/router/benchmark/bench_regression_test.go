package benchmark_test

// Linha de regressão automática — Layer 2 (framework benchmark).
//
// Baselines medidos em: Intel Core i3-12100F · Go 1.26.2 · Linux · GOMAXPROCS=8
// pacote kyrux/core/router/benchmark (external test package).
// Atualizar as constantes após otimizações intencionais ou troca de hardware.
//
// Uso:
//   go test ./core/router/benchmark/ -run TestRegressionCheck -v

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"kyrux/core/router"
)

const (
	baselineRouteStatic    int64 = 1200
	baselineRoutePathParam int64 = 1350
	baselineRouteQuery     int64 = 1800
	baselineMiddleware1    int64 = 1000
	baselineMiddleware3    int64 = 1000

	// Tolerância: regressão se ns/op > baseline * (1 + tolerance).
	tolerance = 0.10
)

func TestRegressionCheck(t *testing.T) {
	cases := []struct {
		name     string
		baseline int64
		fn       func(b *testing.B)
	}{
		{
			"RouteStatic", baselineRouteStatic,
			func(b *testing.B) {
				r := router.New()
				r.Handle("GET /ping/", func(ctx *router.Context) {
					ctx.JSON(http.StatusOK, map[string]string{"pong": "true"})
				})
				req := httptest.NewRequest("GET", "/ping/", nil)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
				}
			},
		},
		{
			"RoutePathParam", baselineRoutePathParam,
			func(b *testing.B) {
				r := router.New()
				r.Handle("GET /usuarios/<id:int>/", func(ctx *router.Context) {
					ctx.JSON(http.StatusOK, map[string]string{"id": ctx.Param("id")})
				})
				req := httptest.NewRequest("GET", "/usuarios/42/", nil)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
				}
			},
		},
		{
			"RouteQueryString", baselineRouteQuery,
			func(b *testing.B) {
				r := router.New()
				r.Handle("GET /busca/", func(ctx *router.Context) {
					q := ctx.Query("q")
					page := ctx.QueryInt("page", 1)
					ctx.JSON(http.StatusOK, map[string]any{"q": q, "page": page})
				})
				req := httptest.NewRequest("GET", "/busca/?q=kyrux&page=3", nil)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
				}
			},
		},
		{
			"Middleware1", baselineMiddleware1,
			func(b *testing.B) {
				r := router.New()
				r.Use(func(next router.HandlerFunc) router.HandlerFunc {
					return func(ctx *router.Context) { ctx.Set("req_id", "abc"); next(ctx) }
				})
				r.Handle("GET /ping/", func(ctx *router.Context) {
					ctx.JSON(http.StatusOK, nil)
				})
				req := httptest.NewRequest("GET", "/ping/", nil)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
				}
			},
		},
		{
			"Middleware3", baselineMiddleware3,
			func(b *testing.B) {
				r := router.New()
				for _, kv := range [][2]string{{"a", "1"}, {"b", "2"}, {"c", "3"}} {
					k, v := kv[0], kv[1]
					r.Use(func(next router.HandlerFunc) router.HandlerFunc {
						return func(ctx *router.Context) { ctx.Set(k, v); next(ctx) }
					})
				}
				r.Handle("GET /ping/", func(ctx *router.Context) {
					ctx.JSON(http.StatusOK, nil)
				})
				req := httptest.NewRequest("GET", "/ping/", nil)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
				}
			},
		},
	}

	fmt.Printf("\n%-20s  %9s  %9s  %9s  %8s  %s\n",
		"Cenário", "ns/op", "baseline", "limiar", "req/s", "Status")
	fmt.Printf("%s\n", "────────────────────────────────────────────────────────────────────")

	failed := 0
	for _, tc := range cases {
		result := testing.Benchmark(tc.fn)
		got := result.NsPerOp()
		threshold := int64(float64(tc.baseline) * (1 + tolerance))
		rps := 1e9 / float64(got)
		delta := float64(got-tc.baseline) / float64(tc.baseline) * 100

		status := fmt.Sprintf("OK  (%.1f%%)", delta)
		if got > threshold {
			status = fmt.Sprintf("REGRESSÃO  (+%.1f%%)", delta)
			failed++
		}

		fmt.Printf("%-20s  %9d  %9d  %9d  %8.0f  %s\n",
			tc.name, got, tc.baseline, threshold, rps, status)

		t.Logf("[%s] %d ns/op | baseline %d | limiar %d | %.0f req/s | %s",
			tc.name, got, tc.baseline, threshold, rps, status)
	}

	fmt.Println()
	if failed > 0 {
		t.Errorf("%d cenário(s) com regressão de performance (tolerância: %.0f%%)",
			failed, tolerance*100)
	}
}
