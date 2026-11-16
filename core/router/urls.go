package router

import "sync"

type URLPattern struct {
	Method string
	Path   string
	View   HandlerFunc
	Name   string
}

var (
	registryMu  sync.RWMutex
	urlRegistry = map[string]string{}
)

// Resolve retorna o path associado ao nome da rota, ou "#" se não encontrado.
func Resolve(name string) string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if path, ok := urlRegistry[name]; ok {
		return path
	}
	return "#"
}

func Path(method, path string, view HandlerFunc, name string) URLPattern {
	return URLPattern{Method: method, Path: path, View: view, Name: name}
}

func Include(r *Router, patterns []URLPattern) {
	registryMu.Lock()
	for _, p := range patterns {
		r.Handle(p.Method+" "+p.Path, p.View)
		if p.Name != "" {
			// Rota com domínio ("site.com/rota/"): o url devolve só o caminho.
			urlRegistry[p.Name] = displayPath(semHost(convertPattern(p.Path)))
		}
	}
	registryMu.Unlock()
}
