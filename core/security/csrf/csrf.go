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
	"strings"
	"sync/atomic"
)

const (
	cookieName = "kyrux_csrf"
	fieldName  = "kyrux_csrf_token"
	headerName = "X-CSRF-Token"
	tokenLen   = 32
	rawKey     = "csrf_raw"   // token bruto (cookie) — colocado pelo middleware
	contextKey = "csrf_token" // token assinado — computado lazy e cacheado por request
)

var unsafeMethods = map[string]bool{
	"POST": true, "PUT": true, "PATCH": true, "DELETE": true,
}

var secureCookie bool
var atomicSecret atomic.Value
var exemptPrefixes []string

// Exempt isenta um prefixo de path da validação CSRF (ex: "/api/").
// Use para APIs autenticadas por Bearer token (RequireAuth), que não usam
// cookies — sem a isenção, clientes JSON puros recebem 403 em POST/PUT/DELETE.
// Chame no bootstrap ou no Register do app, antes de servir requisições.
func Exempt(prefix string) { exemptPrefixes = append(exemptPrefixes, prefix) }

func isExempt(path string) bool {
	for _, p := range exemptPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

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

// TokenFor devolve o token CSRF assinado da requisição atual — o mesmo valor
// que {{ csrf_token }} injeta no form. Útil para expor o token em respostas
// JSON. A assinatura é computada lazy e cacheada no ctx (uma vez por request).
func TokenFor(ctx *router.Context) string {
	if v, ok := ctx.Get(contextKey); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	v, _ := ctx.Get(rawKey)
	raw, _ := v.(string)
	if raw == "" {
		return ""
	}
	signed := sign(raw)
	ctx.Set(contextKey, signed)
	return signed
}

// RegisterFuncs registra {{ csrf_token }} no FuncMap global de templates.
// Deve ser chamado no bootstrap antes do primeiro render.
func RegisterFuncs() {
	render.AddFunc("csrf_token", func() template.HTML {
		ctx := render.GetCurrentCtx()
		if ctx == nil {
			return ""
		}
		signed := TokenFor(ctx)
		if signed == "" {
			return ""
		}
		return template.HTML(`<input type="hidden" name="` + fieldName + `" value="` + signed + `">`)
	})
}

func Middleware(next router.HandlerFunc) router.HandlerFunc {
	return func(ctx *router.Context) {
		raw, err := getOrCreate(ctx)
		if err != nil {
			http.Error(ctx.Writer, "500 Internal Server Error", http.StatusInternalServerError)
			return
		}
		// Assinatura lazy: o HMAC (~650ns + 10 allocs) só roda na validação
		// de métodos inseguros ou quando {{ csrf_token }}/TokenFor é usado.
		// GETs de JSON/estáticos não pagam a criptografia.
		ctx.Set(rawKey, raw)

		if unsafeMethods[ctx.Request.Method] && !isExempt(ctx.Request.URL.Path) {
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

// getOrCreate devolve o token bruto armazenado no cookie, criando-o se necessário.
func getOrCreate(ctx *router.Context) (raw string, err error) {
	if c, cerr := ctx.Request.Cookie(cookieName); cerr == nil && c.Value != "" {
		raw = c.Value
	} else {
		raw, err = generate()
		if err != nil {
			return
		}
		http.SetCookie(ctx.Writer, &http.Cookie{
			Name:  cookieName,
			Value: raw,
			// HttpOnly false é intencional (double-submit): o valor bruto pode
			// ser lido por JS, mas o token submetido é o HMAC assinado com a
			// SECRET_KEY — um XSS não consegue forjá-lo sem o segredo.
			HttpOnly: false,
			Secure:   secureCookie,
			SameSite: http.SameSiteStrictMode,
			Path:     "/",
		})
	}
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
