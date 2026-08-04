package welcome

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"html/template"
	"kyrux/core/router"
	"net/http"
	"runtime"
	"strconv"
)

//go:embed welcome.html
var welcomeHTML string

//go:embed welcome.css
var welcomeCSS []byte

type pageData struct {
	AppName   string
	Version   string
	Env       string
	Addr      string
	GoVersion string
}

var welcomeTpl = template.Must(template.New("welcome").Parse(welcomeHTML))

// RegisterIfNeeded registra a página de boas-vindas em "GET /" se o dev
// ainda não tiver definido uma rota própria para a raiz. appName, version,
// env e addr vêm já resolvidos do bootstrap — addr em particular já é o
// endereço EXIBÍVEL (127.0.0.1, não 0.0.0.0), evitado aqui em vez de reler
// SERVER_HOST/SERVER_PORT direto do ambiente (duplicaria essa tradução).
func RegisterIfNeeded(r *router.Router, appName, version, env, addr string) {
	cssEtag := fmt.Sprintf(`"%x"`, sha256.Sum256(welcomeCSS))
	r.Internal("GET /kyrux/statics/welcome.css", func(ctx *router.Context) {
		if ctx.Request.Header.Get("If-None-Match") == cssEtag {
			ctx.Writer.WriteHeader(http.StatusNotModified)
			return
		}
		h := ctx.Writer.Header()
		h.Set("Content-Type", "text/css; charset=utf-8")
		h.Set("Content-Length", strconv.Itoa(len(welcomeCSS)))
		h.Set("ETag", cssEtag)
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		ctx.Writer.Write(welcomeCSS)
	})

	if r.HasRoute("GET /") {
		return
	}

	d := pageData{
		AppName:   appName,
		Version:   version,
		Env:       env,
		Addr:      addr,
		GoVersion: runtime.Version(),
	}
	var buf bytes.Buffer
	if err := welcomeTpl.Execute(&buf, d); err != nil {
		panic("welcome: " + err.Error())
	}
	body := buf.Bytes()
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(body))
	contentLen := strconv.Itoa(len(body))

	r.Handle("GET /", func(ctx *router.Context) {
		if ctx.Request.Header.Get("If-None-Match") == etag {
			ctx.Writer.WriteHeader(http.StatusNotModified)
			return
		}
		h := ctx.Writer.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Content-Length", contentLen)
		h.Set("ETag", etag)
		h.Set("Cache-Control", "no-cache")
		ctx.Writer.Write(body)
	})
}
