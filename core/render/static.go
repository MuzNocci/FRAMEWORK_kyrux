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

// noDirListing rejeita diretórios: sem isso o http.FileServer devolve a
// listagem de arquivos (enumeração de todos os assets do projeto).
func noDirListing(f http.File) (http.File, error) {
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		f.Close()
		return nil, os.ErrNotExist
	}
	return f, nil
}

func (m *multiStatic) Open(name string) (http.File, error) {
	// Primeiro tenta em statics/ da raiz
	if f, err := http.Dir("statics").Open(name); err == nil {
		if f, err = noDirListing(f); err == nil {
			return f, nil
		}
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
	return noDirListing(f)
}

func StaticHandler(dir string) http.Handler {
	return http.FileServer(http.Dir(dir))
}

// dirListingSafe embrulha um http.FileSystem para recusar listagem de
// diretório (mesma proteção de noDirListing, reaproveitada aqui).
type dirListingSafe struct{ http.FileSystem }

func (d dirListingSafe) Open(name string) (http.File, error) {
	f, err := d.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	return noDirListing(f)
}

// MediaHandler serve os arquivos enviados via admin (campos kyrux:"image",
// salvos em medias/<app>/<tabela>/ por core/admin) a partir de dir (ex:
// "medias"). Cache longo: cada upload recebe nome único (nunca sobrescrito
// em disco), então tratar como imutável é seguro mesmo em produção.
func MediaHandler(dir string) http.Handler {
	fs := http.FileServer(dirListingSafe{http.Dir(dir)})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isDebug() {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-store")
		}
		fs.ServeHTTP(w, r)
	})
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
