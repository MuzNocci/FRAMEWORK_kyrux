package benchmark_test

// Layer 1 — Microbenchmark
//
// Mede o custo de registro de rotas via API pública.
// Internamente exercita convertPattern, extractParamNames, normalize e slashAlternate.

import (
	"net/http"
	"testing"

	"kyrux/core/router"
)

func BenchmarkRouterNew(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		router.New()
	}
}

func BenchmarkHandleStatic(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := router.New()
		r.Handle("GET /usuarios/perfil/", func(ctx *router.Context) {
			ctx.JSON(http.StatusOK, nil)
		})
	}
}

func BenchmarkHandleOneParam(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := router.New()
		r.Handle("GET /usuarios/<id:int>/", func(ctx *router.Context) {
			ctx.JSON(http.StatusOK, nil)
		})
	}
}

func BenchmarkHandleTwoParams(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := router.New()
		r.Handle("GET /orgs/<org:str>/repos/<repo:str>/", func(ctx *router.Context) {
			ctx.JSON(http.StatusOK, nil)
		})
	}
}

func BenchmarkHandle10Routes(b *testing.B) {
	routes := []string{
		"GET /a/", "GET /b/", "GET /c/", "GET /d/", "GET /e/",
		"GET /a/<id:int>/", "GET /b/<id:int>/", "GET /c/<id:int>/",
		"POST /a/", "POST /b/",
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := router.New()
		for _, pattern := range routes {
			r.Handle(pattern, func(ctx *router.Context) {})
		}
	}
}

func BenchmarkHandle50Routes(b *testing.B) {
	bases := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := router.New()
		for _, base := range bases {
			for _, method := range methods {
				r.Handle(method+" /"+base+"/", func(ctx *router.Context) {})
			}
		}
	}
}
