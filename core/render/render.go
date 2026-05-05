package render

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	kyerrors "kyrux/core/errors"
	"kyrux/core/router"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var templateFuncs = template.FuncMap{
	"url":     router.Resolve,
	"statics": func(_ string, parts ...string) string {
		path := strings.Join(parts, "")
		if path != "" {
			return "/statics/" + path
		}
		return "/statics"
	},
}

const liveScript = `<script>
(function(){
  var p=location.protocol==="https:"?"wss":"ws";
  var ws=new WebSocket(p+"://"+location.host+"/kyrux/websocket/ws/");
  ws.onmessage=function(e){
    try{
      var m=JSON.parse(e.data);
      if(m.type!=="kyrux:dom")return;
      var el=document.querySelector('[kyrux-target="'+m.target+'"]');
      if(!el)return;
      var a=m.action||"replace";
      if(a==="append")el.insertAdjacentHTML("beforeend",m.html);
      else if(a==="prepend")el.insertAdjacentHTML("afterbegin",m.html);
      else if(a==="remove")el.remove();
      else el.innerHTML=m.html;
    }catch(_){}
  };
})();
</script>`

const reloadScript = `<script>
(function(){
  var es=new EventSource('/__kyrux_reload__');
  es.onmessage=function(e){if(e.data==='reload')location.reload();};
  es.onerror=function(){setTimeout(function(){location.reload();},1000);};
})();
</script>`

var (
	AppsDir           = "apps"
	defaultProcessors []ContextProcessor
	renderersMu       sync.RWMutex
	renderers         = map[string]*Renderer{}
	debugMode         atomic.Bool
)

var (
	// {{ variavel }} sem ponto — erro de parse: Go interpreta como chamada de função inexistente.
	reMissingFunc = regexp.MustCompile(`template: ([^:]+):(\d+): function "([^"]+)" not defined`)
	// {{ .variavel }} onde a chave não existe no map — erro de execução com missingkey=error.
	reMissingKey = regexp.MustCompile(`map has no entry for key "([^"]+)"`)
	// Posição genérica: extrai arquivo:linha de qualquer erro de template.
	reTemplatePos = regexp.MustCompile(`template: ([^:]+):(\d+)`)
)

func parseTemplateError(err error, eng *Engine) *kyerrors.TemplateError {
	msg := err.Error()
	te := &kyerrors.TemplateError{Raw: msg}

	switch {
	case reMissingFunc.MatchString(msg):
		m := reMissingFunc.FindStringSubmatch(msg)
		te.Kind = "missing_dot"
		te.File = m[1]
		te.Line, _ = strconv.Atoi(m[2])
		te.VarName = m[3]
	case reMissingKey.MatchString(msg):
		te.Kind = "missing_key"
		m := reMissingKey.FindStringSubmatch(msg)
		te.VarName = m[1]
		if p := reTemplatePos.FindStringSubmatch(msg); p != nil {
			te.File = p[1]
			te.Line, _ = strconv.Atoi(p[2])
		}
	default:
		te.Kind = "syntax"
		if p := reTemplatePos.FindStringSubmatch(msg); p != nil {
			te.File = p[1]
			te.Line, _ = strconv.Atoi(p[2])
		}
	}

	if te.File != "" && te.Line > 0 {
		if src, ok := eng.sourceOf(te.File); ok {
			te.Snippet = buildSnippet(src, te.Line, 3)
		}
	}
	return te
}

func buildSnippet(src string, errorLine, context int) []kyerrors.SnippetLine {
	lines := strings.Split(src, "\n")
	start := errorLine - context - 1
	if start < 0 {
		start = 0
	}
	end := errorLine + context
	if end > len(lines) {
		end = len(lines)
	}
	out := make([]kyerrors.SnippetLine, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, kyerrors.SnippetLine{
			Number:  i + 1,
			Content: lines[i],
			IsError: i+1 == errorLine,
		})
	}
	return out
}

func SetDebug(v bool)  { debugMode.Store(v) }
func isDebug() bool    { return debugMode.Load() }

func AddDefaultProcessor(p ContextProcessor) {
	defaultProcessors = append(defaultProcessors, p)
}

type ContextProcessor func(ctx *router.Context) map[string]any

type Renderer struct {
	engine     *Engine
	processors []ContextProcessor
}

// For retorna um Renderer cacheado para apps/<appName>/templates
// com os processadores padrão do framework já aplicados.
func For(appName string) *Renderer {
	renderersMu.RLock()
	if r, ok := renderers[appName]; ok {
		renderersMu.RUnlock()
		return r
	}
	renderersMu.RUnlock()

	renderersMu.Lock()
	defer renderersMu.Unlock()

	if r, ok := renderers[appName]; ok {
		return r
	}

	dir := filepath.Join(AppsDir, appName, "templates")
	r := &Renderer{
		engine:     MustNew(dir),
		processors: append([]ContextProcessor{}, defaultProcessors...),
	}
	renderers[appName] = r
	return r
}

func (r *Renderer) With(processors ...ContextProcessor) *Renderer {
	r.processors = append(r.processors, processors...)
	return r
}

func (r *Renderer) Render(ctx *router.Context, template string, data map[string]any) {
	setCurrentCtx(ctx)
	defer clearCurrentCtx()

	merged := mergedPool.Get().(map[string]any)
	for k := range merged {
		delete(merged, k)
	}
	for _, p := range r.processors {
		for k, v := range p(ctx) {
			merged[k] = v
		}
	}
	for k, v := range data {
		merged[k] = v
	}
	err := r.engine.Render(ctx.Writer, template, merged)
	mergedPool.Put(merged)
	if err != nil {
		log.Printf("render: %v", err)
		if isDebug() {
			kyerrors.RenderPanic(ctx.Writer, ctx.Request, parseTemplateError(err, r.engine), nil)
		} else {
			kyerrors.RenderPanic(ctx.Writer, ctx.Request, err, nil)
		}
	}
}

type Engine struct {
	sources  map[string]srcInfo
	compiled map[string]*compiledEntry
	dir      string
	mu       sync.RWMutex
}

func New(dir string) (*Engine, error) {
	e := &Engine{dir: dir}
	return e, e.loadSources()
}

func (e *Engine) reload() error {
	return e.loadSources()
}

var (
	bufPool    = sync.Pool{New: func() any { return new(bytes.Buffer) }}
	mergedPool = sync.Pool{New: func() any { return make(map[string]any, 8) }}
)

func (e *Engine) Render(w http.ResponseWriter, name string, data any) error {
	if isDebug() {
		e.mu.Lock()
		reloadErr := e.reload()
		e.mu.Unlock()
		if reloadErr != nil {
			return reloadErr
		}
	}

	var ce *compiledEntry
	if isDebug() {
		e.mu.RLock()
		ce = e.compiled[name]
		e.mu.RUnlock()
	} else {
		ce = e.compiled[name]
	}

	if ce == nil {
		return fmt.Errorf("template '%s' não encontrado", name)
	}

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()

	if err := ce.set.ExecuteTemplate(buf, ce.execName, data); err != nil {
		bufPool.Put(buf)
		return err
	}

	inject := liveScript
	if isDebug() {
		inject += reloadScript
	}

	raw := buf.Bytes()
	idx := bytes.Index(raw, []byte("</body>"))
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")

	var writeErr error
	if idx < 0 {
		h.Set("Content-Length", strconv.Itoa(len(raw)))
		_, writeErr = w.Write(raw)
	} else {
		h.Set("Content-Length", strconv.Itoa(len(raw)+len(inject)))
		w.Write(raw[:idx])
		io.WriteString(w, inject)
		_, writeErr = w.Write(raw[idx:])
	}
	bufPool.Put(buf)
	return writeErr
}

func (e *Engine) RenderToString(name string, data any) (string, error) {
	ce := e.compiled[name]
	if ce == nil {
		return "", fmt.Errorf("template '%s' não encontrado", name)
	}
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	err := ce.set.ExecuteTemplate(buf, ce.execName, data)
	s := buf.String()
	bufPool.Put(buf)
	return s, err
}

func (e *Engine) RenderFS(w http.ResponseWriter, fsys fs.FS, name string, data any) error {
	tmpl, err := template.New("").Option("missingkey=error").ParseFS(fsys, "*.html")
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(w, name, data)
}

func (e *Engine) RenderEmbed(w http.ResponseWriter, fsys fs.FS, name string, data any) error {
	var files []string
	fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(path) == ".html" {
			files = append(files, path)
		}
		return nil
	})
	tmpl, err := template.New("").Option("missingkey=error").ParseFS(fsys, files...)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(w, name, data)
}

func StaticEmbedHandler(fsys fs.FS) http.Handler {
	return http.FileServer(http.FS(fsys))
}

func MustNew(dir string) *Engine {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return &Engine{dir: dir, sources: map[string]srcInfo{}, compiled: map[string]*compiledEntry{}}
	}
	e, err := New(dir)
	if err != nil {
		panic("render: " + err.Error())
	}
	return e
}
