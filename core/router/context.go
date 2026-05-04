package router

import (
	"bytes"
	"encoding/json"
	kyerrors "kyrux/core/errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
)

// jsonEnc agrupa buffer + encoder reutilizáveis por request.
// O encoder aponta permanentemente para o buffer — reset em buf basta para reuso.
type jsonEnc struct {
	buf *bytes.Buffer
	enc *json.Encoder
}

var jsonPool = sync.Pool{New: func() any {
	b := &bytes.Buffer{}
	return &jsonEnc{buf: b, enc: json.NewEncoder(b)}
}}

type Context struct {
	Writer  http.ResponseWriter
	Request *http.Request
	Params  map[string]string
	data    map[string]any
	query   url.Values // cache lazy do query string parseado
}

func (c *Context) SetParam(key, value string) {
	if c.Params == nil {
		c.Params = make(map[string]string)
	}
	c.Params[key] = value
}

func (c *Context) Set(key string, value any) {
	if c.data == nil {
		c.data = make(map[string]any)
	}
	c.data[key] = value
}

func (c *Context) Get(key string) (any, bool) {
	v, ok := c.data[key]
	return v, ok
}

// JSON serializa v como JSON, define Content-Type e Content-Length,
// e escreve a resposta em um único Write para minimizar syscalls.
func (c *Context) JSON(status int, v any) {
	je := jsonPool.Get().(*jsonEnc)
	je.buf.Reset()
	if err := je.enc.Encode(v); err != nil {
		jsonPool.Put(je)
		http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
		return
	}
	h := c.Writer.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(je.buf.Len()))
	c.Writer.WriteHeader(status)
	c.Writer.Write(je.buf.Bytes())
	jsonPool.Put(je)
}

func (c *Context) HTML(status int, body string) {
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Writer.WriteHeader(status)
	io.WriteString(c.Writer, body)
}

func (c *Context) Redirect(url string, status int) {
	http.Redirect(c.Writer, c.Request, url, status)
}

func (c *Context) Error(code int) {
	kyerrors.Render(c.Writer, c.Request, code)
}

// Param retorna o valor do parâmetro de path pelo nome.
// Exemplo: rota "/usuarios/<id:int>/" → ctx.Param("id")
func (c *Context) Param(key string) string {
	return c.Params[key]
}

// ParamInt retorna o parâmetro de path convertido para int.
// Retorna (0, false) se ausente ou não for um inteiro válido.
func (c *Context) ParamInt(key string) (int, bool) {
	v := c.Params[key]
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}

// queryVals retorna os valores de query string parseados, cacheando o resultado
// para evitar que múltiplas chamadas Query*/QueryAll* reparsem a mesma URL.
func (c *Context) queryVals() url.Values {
	if c.query == nil {
		c.query = c.Request.URL.Query()
	}
	return c.query
}

// Query retorna o primeiro valor do parâmetro de query string.
// Exemplo: /busca?q=golang → ctx.Query("q")
func (c *Context) Query(key string) string {
	return c.queryVals().Get(key)
}

// QueryDefault retorna o parâmetro de query string ou fallback se ausente/vazio.
func (c *Context) QueryDefault(key, fallback string) string {
	if v := c.queryVals().Get(key); v != "" {
		return v
	}
	return fallback
}

// QueryInt retorna o parâmetro de query string como int, ou fallback se inválido.
func (c *Context) QueryInt(key string, fallback int) int {
	if n, err := strconv.Atoi(c.queryVals().Get(key)); err == nil {
		return n
	}
	return fallback
}

// QueryAll retorna todos os valores de um parâmetro de query string.
// Exemplo: /busca?tag=go&tag=web → ctx.QueryAll("tag") → ["go", "web"]
func (c *Context) QueryAll(key string) []string {
	return c.queryVals()[key]
}
