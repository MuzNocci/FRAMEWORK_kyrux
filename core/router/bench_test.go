package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"kyrux/core/router"
)

// ─── cenários ────────────────────────────────────────────────────────────────

func BenchmarkRouteStatic(b *testing.B) {
	r := newRouter()
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
}

func BenchmarkRoutePathParam(b *testing.B) {
	r := newRouter()
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
}

func BenchmarkRouteQueryString(b *testing.B) {
	r := newRouter()
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
}

func BenchmarkMiddleware1(b *testing.B) {
	r := newRouter()
	r.Use(func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			ctx.Set("req_id", "abc123")
			next(ctx)
		}
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
}

func BenchmarkMiddleware3(b *testing.B) {
	r := newRouter()
	addMW := func(key, val string) {
		r.Use(func(next router.HandlerFunc) router.HandlerFunc {
			return func(ctx *router.Context) { ctx.Set(key, val); next(ctx) }
		})
	}
	addMW("a", "1")
	addMW("b", "2")
	addMW("c", "3")
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
}

func BenchmarkRoute404(b *testing.B) {
	r := newRouter()
	r.Handle("GET /existe/", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, nil)
	})
	req := httptest.NewRequest("GET", "/nao-existe/", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

func BenchmarkParallelStatic(b *testing.B) {
	r := newRouter()
	r.Handle("GET /ping/", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, map[string]string{"pong": "true"})
	})
	req := httptest.NewRequest("GET", "/ping/", nil)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})
}

func BenchmarkParallelPathParam(b *testing.B) {
	r := newRouter()
	r.Handle("GET /usuarios/<id:int>/", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, map[string]string{"id": ctx.Param("id")})
	})
	req := httptest.NewRequest("GET", "/usuarios/42/", nil)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})
}
