package compress

import (
	"compress/gzip"
	"kyrux/core/router"
	"net/http"
	"strings"
	"sync"
)

var gzipPool = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

// incompressiblePrefixes: conteúdos já comprimidos — gzip só desperdiça CPU
// e pode até aumentar o payload.
var incompressiblePrefixes = []string{
	"image/", "video/", "audio/", "font/",
	"application/zip", "application/gzip", "application/pdf",
	"application/octet-stream",
}

func compressible(contentType string) bool {
	if contentType == "" {
		return true // será text/html na maioria dos casos (render define depois)
	}
	if strings.HasPrefix(contentType, "image/svg") {
		return true // SVG é XML — comprime bem
	}
	for _, p := range incompressiblePrefixes {
		if strings.HasPrefix(contentType, p) {
			return false
		}
	}
	return true
}

// gzipWriter decide na primeira escrita (quando os headers do handler já
// estão definidos) se comprime ou passa direto.
type gzipWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer
	active  bool
	decided bool
}

func (gw *gzipWriter) decide() {
	gw.decided = true
	h := gw.Header()
	if h.Get("Content-Encoding") != "" || !compressible(h.Get("Content-Type")) {
		return // já codificado ou incompressível: passa direto
	}
	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length")
	gw.gz.Reset(gw.ResponseWriter)
	gw.active = true
}

func (gw *gzipWriter) WriteHeader(code int) {
	if !gw.decided {
		gw.decide()
	}
	gw.ResponseWriter.WriteHeader(code)
}

func (gw *gzipWriter) Write(b []byte) (int, error) {
	if !gw.decided {
		gw.decide()
	}
	if gw.active {
		return gw.gz.Write(b)
	}
	return gw.ResponseWriter.Write(b)
}

// Compress é um middleware que comprime respostas com gzip quando o cliente
// suporta. Envia Vary: Accept-Encoding em todas as respostas — sem isso,
// proxies/CDNs podem servir a versão gzip a clientes que não a aceitam.
func Compress(next router.HandlerFunc) router.HandlerFunc {
	return func(ctx *router.Context) {
		ctx.Writer.Header().Add("Vary", "Accept-Encoding")
		if !strings.Contains(ctx.Request.Header.Get("Accept-Encoding"), "gzip") {
			next(ctx)
			return
		}
		gz := gzipPool.Get().(*gzip.Writer)
		gw := &gzipWriter{ResponseWriter: ctx.Writer, gz: gz}
		defer func() {
			if gw.active {
				gz.Close()
			}
			gzipPool.Put(gz)
		}()
		ctx.Writer = gw
		next(ctx)
	}
}
