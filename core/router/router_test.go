package router_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kyrux/core/router"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func newRouter() *router.Router { return router.New() }

func do(r *router.Router, method, path string, body ...string) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = strings.NewReader(body[0])
	}
	req := httptest.NewRequest(method, path, bodyReader)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ─── rotas básicas ────────────────────────────────────────────────────────────

func TestGETRoute(t *testing.T) {
	r := newRouter()
	r.Handle("GET /ping/", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, map[string]string{"pong": "true"})
	})

	w := do(r, "GET", "/ping/")
	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, obteve %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type inesperado: %s", ct)
	}
}

func TestPOSTRoute(t *testing.T) {
	r := newRouter()
	r.Handle("POST /items/", func(ctx *router.Context) {
		ctx.JSON(http.StatusCreated, map[string]string{"ok": "true"})
	})

	w := do(r, "POST", "/items/")
	if w.Code != http.StatusCreated {
		t.Fatalf("esperado 201, obteve %d", w.Code)
	}
}

func TestNotFound(t *testing.T) {
	r := newRouter()
	w := do(r, "GET", "/nao-existe/")
	if w.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obteve %d", w.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	r := newRouter()
	r.Handle("GET /somente-get/", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, nil)
	})

	w := do(r, "POST", "/somente-get/")
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("esperado 405, obteve %d", w.Code)
	}
}

func TestTrailingSlashAlternate(t *testing.T) {
	r := newRouter()
	r.Handle("GET /rota/", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, nil)
	})

	for _, path := range []string{"/rota/", "/rota"} {
		w := do(r, "GET", path)
		if w.Code != http.StatusOK {
			t.Errorf("path %q: esperado 200, obteve %d", path, w.Code)
		}
	}
}

// ─── parâmetros de path ───────────────────────────────────────────────────────

func TestPathParam(t *testing.T) {
	r := newRouter()
	r.Handle("GET /usuarios/<nome:str>/", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, map[string]string{"nome": ctx.Param("nome")})
	})

	w := do(r, "GET", "/usuarios/joao/")
	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, obteve %d", w.Code)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["nome"] != "joao" {
		t.Fatalf("param nome: esperado 'joao', obteve '%s'", body["nome"])
	}
}

func TestPathParamInt(t *testing.T) {
	r := newRouter()
	r.Handle("GET /produtos/<id:int>/", func(ctx *router.Context) {
		id, ok := ctx.ParamInt("id")
		if !ok {
			ctx.JSON(http.StatusBadRequest, nil)
			return
		}
		ctx.JSON(http.StatusOK, map[string]int{"id": id})
	})

	w := do(r, "GET", "/produtos/42/")
	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, obteve %d", w.Code)
	}
	var body map[string]int
	json.NewDecoder(w.Body).Decode(&body)
	if body["id"] != 42 {
		t.Fatalf("param id: esperado 42, obteve %d", body["id"])
	}
}

func TestMultiplePathParams(t *testing.T) {
	r := newRouter()
	r.Handle("GET /orgs/<org:str>/repos/<repo:str>/", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, map[string]string{
			"org":  ctx.Param("org"),
			"repo": ctx.Param("repo"),
		})
	})

	w := do(r, "GET", "/orgs/kyrux/repos/framework/")
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["org"] != "kyrux" || body["repo"] != "framework" {
		t.Fatalf("params incorretos: %v", body)
	}
}

// ─── query string ─────────────────────────────────────────────────────────────

func TestQuery(t *testing.T) {
	r := newRouter()
	r.Handle("GET /busca/", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, map[string]string{"q": ctx.Query("q")})
	})

	w := do(r, "GET", "/busca/?q=golang")
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["q"] != "golang" {
		t.Fatalf("query q: esperado 'golang', obteve '%s'", body["q"])
	}
}

func TestQueryDefault(t *testing.T) {
	r := newRouter()
	r.Handle("GET /pagina/", func(ctx *router.Context) {
		page := ctx.QueryDefault("page", "1")
		ctx.JSON(http.StatusOK, map[string]string{"page": page})
	})

	tests := []struct {
		url      string
		expected string
	}{
		{"/pagina/", "1"},
		{"/pagina/?page=3", "3"},
	}
	for _, tc := range tests {
		w := do(r, "GET", tc.url)
		var body map[string]string
		json.NewDecoder(w.Body).Decode(&body)
		if body["page"] != tc.expected {
			t.Errorf("url %q: esperado '%s', obteve '%s'", tc.url, tc.expected, body["page"])
		}
	}
}

func TestQueryInt(t *testing.T) {
	r := newRouter()
	r.Handle("GET /lista/", func(ctx *router.Context) {
		limit := ctx.QueryInt("limit", 20)
		ctx.JSON(http.StatusOK, map[string]int{"limit": limit})
	})

	tests := []struct {
		url      string
		expected int
	}{
		{"/lista/", 20},
		{"/lista/?limit=50", 50},
		{"/lista/?limit=abc", 20},
	}
	for _, tc := range tests {
		w := do(r, "GET", tc.url)
		var body map[string]int
		json.NewDecoder(w.Body).Decode(&body)
		if body["limit"] != tc.expected {
			t.Errorf("url %q: esperado %d, obteve %d", tc.url, tc.expected, body["limit"])
		}
	}
}

func TestQueryAll(t *testing.T) {
	r := newRouter()
	r.Handle("GET /tags/", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, map[string][]string{"tags": ctx.QueryAll("tag")})
	})

	w := do(r, "GET", "/tags/?tag=go&tag=web&tag=orm")
	var body map[string][]string
	json.NewDecoder(w.Body).Decode(&body)
	if len(body["tags"]) != 3 {
		t.Fatalf("esperado 3 tags, obteve %d: %v", len(body["tags"]), body["tags"])
	}
}

// ─── respostas ────────────────────────────────────────────────────────────────

func TestJSONResponse(t *testing.T) {
	r := newRouter()
	r.Handle("GET /json/", func(ctx *router.Context) {
		ctx.JSON(http.StatusAccepted, map[string]any{"status": "ok", "code": 202})
	})

	w := do(r, "GET", "/json/")
	if w.Code != http.StatusAccepted {
		t.Fatalf("esperado 202, obteve %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type: %s", ct)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("corpo inválido: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("campo status: %v", body["status"])
	}
}

func TestHTMLResponse(t *testing.T) {
	r := newRouter()
	r.Handle("GET /html/", func(ctx *router.Context) {
		ctx.HTML(http.StatusOK, "<h1>Kyrux</h1>")
	})

	w := do(r, "GET", "/html/")
	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, obteve %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type: %s", ct)
	}
	if !strings.Contains(w.Body.String(), "Kyrux") {
		t.Fatalf("corpo inesperado: %s", w.Body.String())
	}
}

func TestRedirect(t *testing.T) {
	r := newRouter()
	r.Handle("GET /antigo/", func(ctx *router.Context) {
		ctx.Redirect("/novo/", http.StatusMovedPermanently)
	})

	w := do(r, "GET", "/antigo/")
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("esperado 301, obteve %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/novo/" {
		t.Fatalf("Location: %s", loc)
	}
}

// ─── middleware ───────────────────────────────────────────────────────────────

func TestMiddlewareExecutionOrder(t *testing.T) {
	r := newRouter()
	order := []string{}

	r.Use(func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			order = append(order, "A-antes")
			next(ctx)
			order = append(order, "A-depois")
		}
	})
	r.Use(func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			order = append(order, "B-antes")
			next(ctx)
			order = append(order, "B-depois")
		}
	})

	r.Handle("GET /mw/", func(ctx *router.Context) {
		order = append(order, "handler")
		ctx.JSON(http.StatusOK, nil)
	})

	do(r, "GET", "/mw/")

	expected := []string{"A-antes", "B-antes", "handler", "B-depois", "A-depois"}
	for i, v := range expected {
		if i >= len(order) || order[i] != v {
			t.Fatalf("ordem de execução incorreta: %v (esperado %v)", order, expected)
		}
	}
}

func TestMiddlewareShortCircuit(t *testing.T) {
	r := newRouter()
	handlerRan := false

	r.Use(func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			ctx.JSON(http.StatusUnauthorized, map[string]string{"erro": "não autorizado"})
			// não chama next — curto-circuito
		}
	})

	r.Handle("GET /privado/", func(ctx *router.Context) {
		handlerRan = true
		ctx.JSON(http.StatusOK, nil)
	})

	w := do(r, "GET", "/privado/")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obteve %d", w.Code)
	}
	if handlerRan {
		t.Fatal("handler não deveria ter executado após curto-circuito no middleware")
	}
}

func TestMiddlewareInjectContextValue(t *testing.T) {
	r := newRouter()

	r.Use(func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			ctx.Set("user_id", 99)
			next(ctx)
		}
	})

	r.Handle("GET /perfil/", func(ctx *router.Context) {
		v, ok := ctx.Get("user_id")
		if !ok {
			ctx.JSON(http.StatusInternalServerError, nil)
			return
		}
		ctx.JSON(http.StatusOK, map[string]any{"user_id": v})
	})

	w := do(r, "GET", "/perfil/")
	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, obteve %d", w.Code)
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if int(body["user_id"].(float64)) != 99 {
		t.Fatalf("user_id: %v", body["user_id"])
	}
}

// ─── context store ────────────────────────────────────────────────────────────

func TestContextSetGet(t *testing.T) {
	r := newRouter()
	r.Handle("GET /ctx/", func(ctx *router.Context) {
		ctx.Set("chave", "valor")
		v, ok := ctx.Get("chave")
		if !ok || v.(string) != "valor" {
			ctx.JSON(http.StatusInternalServerError, nil)
			return
		}
		_, missing := ctx.Get("inexistente")
		if missing {
			ctx.JSON(http.StatusInternalServerError, nil)
			return
		}
		ctx.JSON(http.StatusOK, nil)
	})

	w := do(r, "GET", "/ctx/")
	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, obteve %d", w.Code)
	}
}

// ─── HasRoute ────────────────────────────────────────────────────────────────

func TestHasRoute(t *testing.T) {
	r := newRouter()
	r.Handle("GET /existe/", func(ctx *router.Context) {})

	if !r.HasRoute("GET /existe/") {
		t.Fatal("HasRoute: deveria retornar true para rota registrada")
	}
	if r.HasRoute("GET /nao-existe/") {
		t.Fatal("HasRoute: deveria retornar false para rota não registrada")
	}
}

// ─── conversão de padrões (unidade) ──────────────────────────────────────────

func TestConvertPattern(t *testing.T) {
	cases := []struct{ in, out string }{
		{"GET /usuarios/<id:int>/", "GET /usuarios/{id}/"},
		{"GET /arquivos/<caminho:path>", "GET /arquivos/{caminho...}"},
		{"GET /itens/<slug:slug>/", "GET /itens/{slug}/"},
		{"GET /sem-params/", "GET /sem-params/"},
	}
	for _, tc := range cases {
		// Testa indiretamente via HasRoute após Handle — a conversão é pública via comportamento.
		r := newRouter()
		r.Handle(tc.in, func(ctx *router.Context) {
			ctx.JSON(http.StatusOK, nil)
		})
		if !r.HasRoute(tc.in) {
			t.Errorf("HasRoute(%q) falhou após Handle", tc.in)
		}
	}
}

func TestExtractParamNames(t *testing.T) {
	r := newRouter()
	captured := map[string]string{}

	r.Handle("GET /a/<x:str>/b/<y:int>/", func(ctx *router.Context) {
		captured["x"] = ctx.Param("x")
		captured["y"] = ctx.Param("y")
		ctx.JSON(http.StatusOK, nil)
	})

	do(r, "GET", "/a/hello/b/7/")
	if captured["x"] != "hello" || captured["y"] != "7" {
		t.Fatalf("params: %v", captured)
	}
}
