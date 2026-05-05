package welcome

import (
	_ "embed"
	"bytes"
	"crypto/sha256"
	"fmt"
	"html/template"
	"kyrux/core/environment"
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

func RegisterIfNeeded(r *router.Router) {
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
		AppName:   environment.GetOr("APP_NAME", "kyrux"),
		Version:   environment.GetOr("APP_VERSION", "0.1.0"),
		Env:       environment.GetOr("APP_ENV", "production"),
		Addr:      environment.GetOr("SERVER_HOST", "0.0.0.0") + ":" + environment.GetOr("SERVER_PORT", "8080"),
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
