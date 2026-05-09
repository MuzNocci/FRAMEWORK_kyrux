package assets

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"kyrux/core/router"
	"net/http"
	"strconv"
)

//go:embed images/logotipo/kyrux_wt.png
var kyruxLogo []byte

func Register(r *router.Router) {
	serveStatic(r, "GET /kyrux/statics/kyrux_wt.png", kyruxLogo, "image/png")
}

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
