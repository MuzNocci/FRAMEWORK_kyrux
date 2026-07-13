package compress

import (
	"compress/gzip"
	"io"
	"kyrux/core/router"
	"net/http/httptest"
	"strings"
	"testing"
)

func run(t *testing.T, acceptGzip bool, handler router.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/", nil)
	if acceptGzip {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	rec := httptest.NewRecorder()
	Compress(handler)(&router.Context{Writer: rec, Request: req})
	return rec
}

func TestCompressHTML(t *testing.T) {
	body := strings.Repeat("kyrux ", 200)
	rec := run(t, true, func(ctx *router.Context) {
		ctx.HTML(200, body)
	})

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("esperava Content-Encoding: gzip, recebeu %q", rec.Header().Get("Content-Encoding"))
	}
	if rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("esperava Vary: Accept-Encoding, recebeu %q", rec.Header().Get("Vary"))
	}
	gr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("corpo não é gzip válido: %v", err)
	}
	out, _ := io.ReadAll(gr)
	if string(out) != body {
		t.Errorf("corpo descomprimido difere do original")
	}
}

func TestCompressSkipImage(t *testing.T) {
	rec := run(t, true, func(ctx *router.Context) {
		ctx.Writer.Header().Set("Content-Type", "image/png")
		ctx.Writer.WriteHeader(200)
		ctx.Writer.Write([]byte("png-bytes"))
	})

	if rec.Header().Get("Content-Encoding") != "" {
		t.Errorf("imagem não deveria ser comprimida (Content-Encoding=%q)", rec.Header().Get("Content-Encoding"))
	}
	if rec.Body.String() != "png-bytes" {
		t.Errorf("corpo deveria passar intacto, recebeu %q", rec.Body.String())
	}
	if rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("Vary deve estar presente mesmo sem comprimir")
	}
}

func TestCompressClientSemGzip(t *testing.T) {
	rec := run(t, false, func(ctx *router.Context) {
		ctx.HTML(200, "olá")
	})

	if rec.Header().Get("Content-Encoding") != "" {
		t.Errorf("cliente sem gzip não deve receber Content-Encoding")
	}
	if rec.Body.String() != "olá" {
		t.Errorf("corpo esperado 'olá', recebeu %q", rec.Body.String())
	}
	if rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("Vary deve estar presente para caches mesmo sem gzip")
	}
}
