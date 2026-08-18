package admin

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"fmt"
	"html/template"
	"kyrux/core/router"
	"net/http"
	"strconv"
	"sync"
)

//go:embed layout.html login.html dashboard.html list.html form.html history.html
var tplFS embed.FS

//go:embed admin.css
var adminCSS []byte

//go:embed admin.js
var adminJS []byte

// mustPage clona o layout compartilhado e anexa o template de conteúdo da
// página — cada página define seu próprio {{define "content"}}, executado
// dentro do layout via ExecuteTemplate(w, "layout", data).
func mustPage(name string) *template.Template {
	base := template.Must(template.New("layout").ParseFS(tplFS, "layout.html"))
	return template.Must(template.Must(base.Clone()).ParseFS(tplFS, name))
}

var (
	loginTpl     = mustPage("login.html")
	dashboardTpl = mustPage("dashboard.html")
	listTpl      = mustPage("list.html")
	formTpl      = mustPage("form.html")
	historyTpl   = mustPage("history.html")
)

// bufPool evita, a cada render, começar de um bytes.Buffer com capacidade
// zero e pagar o custo de bytes.growSlice até o tamanho real da página —
// mesmo padrão já usado em core/render/render.go.
var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func renderPage(w http.ResponseWriter, tpl *template.Template, data any) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	if err := tpl.ExecuteTemplate(buf, "layout", data); err != nil {
		http.Error(w, "admin: erro ao renderizar: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	// no-store cobre a maioria dos navegadores; Pragma é reforço para caches
	// mais antigos. Ainda assim o bfcache pode restaurar sem requisição nova
	// (daí o listener de "pageshow" em admin.js, que força reload).
	h.Set("Cache-Control", "no-store")
	h.Set("Pragma", "no-cache")
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Write(buf.Bytes())
}

// serveStatic serve um asset embutido com ETag e cache imutável — mesmo
// padrão usado por core/bootstrap/assets e core/bootstrap/welcome.
func serveStatic(r *router.Router, pattern string, data []byte, contentType string) {
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
	r.Internal(pattern, func(ctx *router.Context) {
		if ctx.Request.Header.Get("If-None-Match") == etag {
			ctx.Writer.WriteHeader(http.StatusNotModified)
			return
		}
		h := ctx.Writer.Header()
		h.Set("Content-Type", contentType)
		h.Set("Content-Length", strconv.Itoa(len(data)))
		h.Set("ETag", etag)
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		ctx.Writer.Write(data)
	})
}
