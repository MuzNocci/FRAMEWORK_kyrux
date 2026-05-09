package render

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type multiStatic struct {
	appsDir string
}

func (m *multiStatic) Open(name string) (http.File, error) {
	// Primeiro tenta em statics/ da raiz
	if f, err := http.Dir("statics").Open(name); err == nil {
		return f, nil
	}

	// http.FileServer sempre passa paths com "/" no início — strip antes de manipular
	clean := strings.TrimPrefix(name, "/")
	parts := strings.SplitN(clean, "/", 2)
	if parts[0] == "" {
		return nil, os.ErrNotExist
	}

	appName := parts[0]
	var subPath string
	if len(parts) > 1 {
		subPath = parts[1]
	}

	path := filepath.Join(m.appsDir, appName, "assets", subPath)
	f, err := os.Open(path)
	if err != nil {
		return nil, os.ErrNotExist
	}
	return f, nil
}

func StaticHandler(dir string) http.Handler {
	return http.FileServer(http.Dir(dir))
}

func MultiStaticHandler(appsDir string) http.Handler {
	fs := http.FileServer(&multiStatic{appsDir: appsDir})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isDebug() {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-store")
		}
		fs.ServeHTTP(w, r)
	})
}
