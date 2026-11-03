package errors

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pagina(code int, corpo string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		w.Write([]byte(corpo))
	}
}

func TestSetHandlerERemocao(t *testing.T) {
	Set(418, pagina(418, "chá"))
	if _, ok := Handler(418); !ok {
		t.Fatal("Handler(418) deveria existir depois de Set")
	}
	Set(418, nil)
	if _, ok := Handler(418); ok {
		t.Fatal("Set(418, nil) deveria remover o handler")
	}
	// Render sem handler cai na página padrão (production), sem panic.
	SetDebug(false)
	w := httptest.NewRecorder()
	Render(w, httptest.NewRequest("GET", "/", nil), http.StatusNotFound)
	if w.Code != http.StatusNotFound {
		t.Fatalf("Render 404 padrão: status %d", w.Code)
	}
}

func TestRenderPanicUsaPagina500DoProjeto(t *testing.T) {
	SetDebug(false)
	t.Cleanup(func() { Set(http.StatusInternalServerError, nil) })

	// sem página do projeto: página padrão do Kyrux
	w := httptest.NewRecorder()
	RenderPanic(w, httptest.NewRequest("GET", "/x", nil), "boom", nil)
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "página do projeto") {
		t.Fatalf("sem Set(500): status %d, corpo inesperado", w.Code)
	}

	// com página do projeto: ela é usada
	Set(http.StatusInternalServerError, pagina(http.StatusInternalServerError, "página do projeto"))
	w = httptest.NewRecorder()
	RenderPanic(w, httptest.NewRequest("GET", "/x", nil), "boom", nil)
	if w.Code != http.StatusInternalServerError || w.Body.String() != "página do projeto" {
		t.Fatalf("com Set(500): status %d, corpo %q", w.Code, w.Body.String())
	}

	// página do projeto que falha: cai na padrão, sem derrubar o processo
	Set(http.StatusInternalServerError, func(w http.ResponseWriter, r *http.Request) { panic("template quebrado") })
	w = httptest.NewRecorder()
	RenderPanic(w, httptest.NewRequest("GET", "/x", nil), "boom", nil)
	if w.Code != http.StatusInternalServerError || w.Body.Len() == 0 {
		t.Fatalf("página 500 com falha: status %d, corpo vazio=%v", w.Code, w.Body.Len() == 0)
	}
}
