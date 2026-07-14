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
)

//go:embed layout.html login.html dashboard.html list.html form.html
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
)

func renderPage(w http.ResponseWriter, tpl *template.Template, data any) {
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "layout", data); err != nil {
		http.Error(w, "admin: erro ao renderizar: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
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
