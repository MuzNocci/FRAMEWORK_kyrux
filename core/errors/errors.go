package errors

import (
	"bytes"
	_ "embed"
	"fmt"
	"html"
	"html/template"
	"kyrux/core/environment"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

//go:embed error.html
var errorHTML string

//go:embed debug.html
var debugHTML string

var (
	tpl      = template.Must(template.New("error").Parse(errorHTML))
	debugTpl = template.Must(template.New("debug").Funcs(template.FuncMap{
		"lbrace": func() string { return "{{" },
		"rbrace": func() string { return "}}" },
	}).Parse(debugHTML))
)

var debugMode atomic.Bool

func SetDebug(v bool) { debugMode.Store(v) }

var atomicAppName, atomicVersion atomic.Value

func SetApp(name, version string) {
	atomicAppName.Store(name)
	atomicVersion.Store(version)
}

func getAppName() string {
	if v, _ := atomicAppName.Load().(string); v != "" {
		return v
	}
	return environment.GetOr("APP_NAME", "kyrux")
}

func getVersion() string {
	if v, _ := atomicVersion.Load().(string); v != "" {
		return v
	}
	return environment.GetOr("APP_VERSION", "0.1.0")
}

// RouteEntry representa uma rota registrada, exibida na debug page de 404.
type RouteEntry struct {
	Method string
	Path   string
}

// routeListFunc é preenchida pelo bootstrap para evitar import cycle com o router.
var routeListFunc func() []RouteEntry

func SetRouteListFunc(f func() []RouteEntry) { routeListFunc = f }

type pageData struct {
	Code    int
	Title   string
	Message string
	AppName string
	Version string
}

type debugData struct {
	AppName  string
	Version  string
	Code     int
	Title    string
	Method   string
	Path     string
	Error    string
	Stack    template.HTML
	Routes   []RouteEntry
	TplError *TemplateError
}

// TemplateError descreve um erro de template com localização precisa.
// Preenchido pelo pacote render e renderizado pela debug page.
type TemplateError struct {
	Kind    string // "missing_dot" | "missing_key" | "syntax"
	File    string // nome do arquivo de template
	Line    int    // linha onde o erro ocorreu (1-based)
	VarName string // nome da variável envolvida
	Raw     string // mensagem original do Go template engine
	Snippet []SnippetLine
}

// SnippetLine representa uma linha do template exibida no contexto do erro.
type SnippetLine struct {
	Number  int
	Content string
	IsError bool
}

var catalog = map[int][2]string{
	400: {"Bad Request", "A requisição enviada é inválida ou malformada."},
	401: {"Não autorizado", "Autenticação é necessária para acessar este recurso."},
	403: {"Acesso negado", "Você não tem permissão para acessar este recurso."},
	404: {"Página não encontrada", "O recurso solicitado não existe ou foi removido."},
	405: {"Método não permitido", "O método HTTP utilizado não é suportado para esta rota."},
	408: {"Tempo esgotado", "A requisição demorou demais para ser concluída."},
	409: {"Conflito", "A requisição conflita com o estado atual do recurso."},
	422: {"Entidade inválida", "Os dados enviados não puderam ser processados."},
	429: {"Muitas requisições", "Você enviou requisições demais. Tente novamente em instantes."},
	500: {"Erro interno", "Ocorreu um erro inesperado no servidor."},
	502: {"Gateway inválido", "O servidor recebeu uma resposta inválida do serviço de origem."},
	503: {"Serviço indisponível", "O servidor está temporariamente fora do ar."},
	504: {"Timeout de gateway", "O servidor de origem demorou demais para responder."},
}

var (
	mu       sync.RWMutex
	handlers = map[int]http.HandlerFunc{}
)

// Set registra um handler personalizado para o código HTTP informado.
func Set(code int, h http.HandlerFunc) {
	mu.Lock()
	handlers[code] = h
	mu.Unlock()
}

// Render serve a página de erro. Em development mostra a debug page; em production a página estilizada.
func Render(w http.ResponseWriter, r *http.Request, code int) {
	mu.RLock()
	h, ok := handlers[code]
	mu.RUnlock()
	if ok {
		h(w, r)
		return
	}
	if debugMode.Load() {
		renderDebugHTTP(w, r, code)
		return
	}
	renderDefault(w, code)
}

// RenderPanic serve a debug page (development) ou erro 500 (production) a partir de um panic.
func RenderPanic(w http.ResponseWriter, r *http.Request, recovered any, stack []byte) {
	if debugMode.Load() {
		renderDebugPanic(w, r, recovered, stack)
		return
	}
	renderDefault(w, http.StatusInternalServerError)
}

func renderDefault(w http.ResponseWriter, code int) {
	defs, ok := catalog[code]
	if !ok {
		defs = [2]string{http.StatusText(code), "Ocorreu um erro inesperado."}
	}
	d := pageData{
		Code:    code,
		Title:   defs[0],
		Message: defs[1],
		AppName: getAppName(),
		Version: getVersion(),
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, d); err != nil {
		http.Error(w, http.StatusText(code), code)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	w.Write(buf.Bytes())
}

func renderDebugHTTP(w http.ResponseWriter, r *http.Request, code int) {
	defs, ok := catalog[code]
	if !ok {
		defs = [2]string{http.StatusText(code), ""}
	}
	d := debugData{
		AppName: getAppName(),
		Version: getVersion(),
		Code:    code,
		Title:   defs[0],
		Method:  r.Method,
		Path:    r.URL.Path,
	}
	if code == http.StatusNotFound && routeListFunc != nil {
		d.Routes = routeListFunc()
	}
	writeDebug(w, code, d)
}

func renderDebugPanic(w http.ResponseWriter, r *http.Request, recovered any, stack []byte) {
	d := debugData{
		AppName: getAppName(),
		Version: getVersion(),
		Method:  r.Method,
		Path:    r.URL.Path,
		Stack:   formatStack(stack),
	}
	if te, ok := recovered.(*TemplateError); ok {
		d.TplError = te
		d.Error = te.Raw
	} else {
		d.Error = fmt.Sprintf("%v", recovered)
	}
	writeDebug(w, http.StatusInternalServerError, d)
}

func writeDebug(w http.ResponseWriter, code int, d debugData) {
	var buf bytes.Buffer
	if err := debugTpl.Execute(&buf, d); err != nil {
		http.Error(w, fmt.Sprintf("erro: %v", d.Error), code)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	w.Write(buf.Bytes())
}

func formatStack(stack []byte) template.HTML {
	if len(stack) == 0 {
		return ""
	}
	lines := strings.Split(string(stack), "\n")
	var sb strings.Builder
	for _, line := range lines {
		escaped := html.EscapeString(line)
		switch {
		case strings.Contains(line, ".go:"):
			if strings.Contains(line, "/apps/") || strings.Contains(line, "main.go") {
				sb.WriteString(`<span class="user">` + escaped + "</span>\n")
			} else {
				sb.WriteString(`<span class="file">` + escaped + "</span>\n")
			}
		case strings.Contains(line, "(") && !strings.HasPrefix(line, "\t"):
			sb.WriteString(`<span class="fn">` + escaped + "</span>\n")
		default:
			sb.WriteString(escaped + "\n")
		}
	}
	return template.HTML(sb.String())
}
