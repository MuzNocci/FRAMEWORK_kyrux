package router

import (
	"bufio"
	"fmt"
	kyerrors "kyrux/core/errors"
	"net"
	"net/http"
	"strings"
	"sync"
)

var ctxPool = sync.Pool{New: func() any { return &Context{} }}
var cwPool = sync.Pool{New: func() any { return &codeWriter{} }}
var paramsPool = sync.Pool{New: func() any { return make(map[string]string, 4) }}

type HandlerFunc func(ctx *Context)

type Router struct {
	mux         *http.ServeMux
	middlewares []MiddlewareFunc
	routes      map[string]bool
	internal    map[string]bool
	registered  map[string]bool // todos os padrões registrados no mux (evita duplicatas)
}

type MiddlewareFunc func(HandlerFunc) HandlerFunc

func New() *Router {
	return &Router{
		mux:        http.NewServeMux(),
		routes:     map[string]bool{},
		internal:   map[string]bool{},
		registered: map[string]bool{},
	}
}

func (r *Router) HasRoute(pattern string) bool {
	return r.routes[normalize(convertPattern(pattern))]
}

// Internal registra uma rota interna do framework (não aparece na debug page).
func (r *Router) Internal(pattern string, h HandlerFunc) {
	r.internal[normalize(convertPattern(pattern))] = true
	r.Handle(pattern, h)
}

// Routes retorna as rotas registradas pelo desenvolvedor, excluindo rotas internas do framework.
func (r *Router) Routes() []kyerrors.RouteEntry {
	entries := make([]kyerrors.RouteEntry, 0, len(r.routes))
	for pattern := range r.routes {
		if r.internal[pattern] {
			continue
		}
		method, path, _ := strings.Cut(pattern, " ")
		entries = append(entries, kyerrors.RouteEntry{Method: method, Path: displayPath(path)})
	}
	return entries
}

func (r *Router) Use(m MiddlewareFunc) {
	r.middlewares = append(r.middlewares, m)
}

// normalize converte padrões terminados em "/" para match exato usando {$},
// evitando que o ServeMux os trate como subtree (catch-all de prefixo).
func normalize(pattern string) string {
	if strings.HasSuffix(pattern, "/") {
		return pattern + "{$}"
	}
	return pattern
}

func (r *Router) Handle(pattern string, h HandlerFunc) {
	pattern = normalize(convertPattern(pattern))
	paramNames := extractParamNames(pattern)
	r.routes[pattern] = true

	chain := r.chain(h)
	muxHandler := func(w http.ResponseWriter, req *http.Request) {
		if cw, ok := w.(*codeWriter); ok {
			cw.handlerRan = true
		}
		ctx := ctxPool.Get().(*Context)
		ctx.Writer = w
		ctx.Request = req
		ctx.data = nil
		ctx.query = nil
		if len(paramNames) > 0 {
			params := paramsPool.Get().(map[string]string)
			clear(params)
			for _, name := range paramNames {
				params[name] = req.PathValue(name)
			}
			ctx.Params = params
		} else {
			ctx.Params = nil
		}
		chain(ctx)
		if ctx.data != nil {
			clear(ctx.data)
			dataPool.Put(ctx.data)
			ctx.data = nil
		}
		if ctx.Params != nil {
			clear(ctx.Params)
			paramsPool.Put(ctx.Params)
			ctx.Params = nil
		}
		ctxPool.Put(ctx)
	}

	r.registerMux(pattern, muxHandler)

	// Registra a variante complementar (com/sem barra) para aceitar ambas as formas.
	if alt := slashAlternate(pattern); alt != "" {
		r.registerMux(alt, muxHandler)
	}
}

// registerMux registra um padrão no ServeMux apenas se ainda não foi registrado.
func (r *Router) registerMux(pattern string, h http.HandlerFunc) {
	if r.registered[pattern] {
		return
	}
	r.registered[pattern] = true
	r.mux.HandleFunc(pattern, h)
}

func (r *Router) chain(h HandlerFunc) HandlerFunc {
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		h = r.middlewares[i](h)
	}
	return h
}

func (r *Router) HandlePrefix(prefix string, h http.Handler) {
	r.mux.Handle(prefix, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if cw, ok := w.(*codeWriter); ok {
			cw.handlerRan = true
		}
		h.ServeHTTP(w, req)
	}))
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	cw := cwPool.Get().(*codeWriter)
	cw.ResponseWriter = w
	cw.code = 0
	cw.handlerRan = false
	cw.intercepted = false
	r.mux.ServeHTTP(cw, req)
	if (cw.code == http.StatusNotFound || cw.code == http.StatusMethodNotAllowed) && !cw.handlerRan {
		kyerrors.Render(w, req, cw.code)
	}
	cwPool.Put(cw)
}

// codeWriter intercepta WriteHeader para detectar 404/405 automáticos do ServeMux.
// Quando um handler registrado é executado (handlerRan=true), tudo passa direto.
type codeWriter struct {
	http.ResponseWriter
	code        int
	handlerRan  bool
	intercepted bool
}

func (cw *codeWriter) WriteHeader(code int) {
	cw.code = code
	if cw.handlerRan || (code != http.StatusNotFound && code != http.StatusMethodNotAllowed) {
		cw.ResponseWriter.WriteHeader(code)
	} else {
		cw.intercepted = true
	}
}

func (cw *codeWriter) Write(b []byte) (int, error) {
	if cw.intercepted {
		return len(b), nil
	}
	return cw.ResponseWriter.Write(b)
}

func (cw *codeWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := cw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("router: ResponseWriter does not implement http.Hijacker")
	}
	return h.Hijack()
}

func (cw *codeWriter) Flush() {
	if f, ok := cw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
