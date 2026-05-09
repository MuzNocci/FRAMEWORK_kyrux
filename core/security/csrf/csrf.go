package csrf

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"html/template"
	"kyrux/core/environment"
	"kyrux/core/render"
	"kyrux/core/router"
	"net/http"
	"sync/atomic"
)

const (
	cookieName = "kyrux_csrf"
	fieldName  = "kyrux_csrf_token"
	headerName = "X-CSRF-Token"
	tokenLen   = 32
	contextKey = "csrf_token"
)

var unsafeMethods = map[string]bool{
	"POST": true, "PUT": true, "PATCH": true, "DELETE": true,
}

var secureCookie bool
var atomicSecret atomic.Value

// SetSecure ativa a flag Secure no cookie CSRF. Deve ser chamado no bootstrap.
func SetSecure(v bool) { secureCookie = v }

// SetSecret configura a chave HMAC usada para assinar os tokens CSRF.
// Deve ser chamado no bootstrap antes de servir requisições.
func SetSecret(key string) { atomicSecret.Store(key) }

func secretKey() []byte {
	if v, _ := atomicSecret.Load().(string); v != "" {
		return []byte(v)
	}
	if k := environment.GetOr("SECRET_KEY", ""); k != "" {
		return []byte(k)
	}
	panic("csrf: SECRET_KEY não configurado — defina no .env ou chame csrf.SetSecret() no bootstrap")
}

// RegisterFuncs registra {{ csrf_token }} no FuncMap global de templates.
// Deve ser chamado no bootstrap antes do primeiro render.
func RegisterFuncs() {
	render.AddFunc("csrf_token", func() template.HTML {
		ctx := render.GetCurrentCtx()
		if ctx == nil {
			return ""
		}
		v, _ := ctx.Get(contextKey)
		signed, _ := v.(string)
		return template.HTML(`<input type="hidden" name="` + fieldName + `" value="` + signed + `">`)
	})
}

func Middleware(next router.HandlerFunc) router.HandlerFunc {
	return func(ctx *router.Context) {
		raw, signed, err := getOrCreate(ctx)
		if err != nil {
			http.Error(ctx.Writer, "500 Internal Server Error", http.StatusInternalServerError)
			return
		}
		ctx.Set(contextKey, signed)

		if unsafeMethods[ctx.Request.Method] {
			submitted := ctx.Request.FormValue(fieldName)
			if submitted == "" {
				submitted = ctx.Request.Header.Get(headerName)
			}
			if !valid(raw, submitted) {
				http.Error(ctx.Writer, "403 Forbidden — CSRF token inválido ou ausente", http.StatusForbidden)
				return
			}
		}

		next(ctx)
	}
}

// getOrCreate devolve o token bruto (armazenado no cookie), o token assinado (para o form) e um erro.
func getOrCreate(ctx *router.Context) (raw, signed string, err error) {
	if c, cerr := ctx.Request.Cookie(cookieName); cerr == nil && c.Value != "" {
		raw = c.Value
	} else {
		raw, err = generate()
		if err != nil {
			return
		}
		http.SetCookie(ctx.Writer, &http.Cookie{
			Name:     cookieName,
			Value:    raw,
			HttpOnly: false,
			Secure:   secureCookie,
			SameSite: http.SameSiteStrictMode,
			Path:     "/",
		})
	}
	signed = sign(raw)
	return
}

func generate() (string, error) {
	b := make([]byte, tokenLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("csrf: falha ao gerar token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// sign retorna HMAC-SHA256(secretKey, token) — resistente a bypass via XSS.
func sign(token string) string {
	mac := hmac.New(sha256.New, secretKey())
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

// valid verifica se o valor submetido é o HMAC correto do token do cookie.
func valid(rawCookie, submitted string) bool {
	if len(submitted) == 0 {
		return false
	}
	expected := sign(rawCookie)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(submitted)) == 1
}
