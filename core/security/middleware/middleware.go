package middleware

import (
	kyerrors "kyrux/core/errors"
	"kyrux/core/router"
	"kyrux/core/security/auth"
	"kyrux/core/security/session"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
)

func AllowedHosts(hosts []string, debug bool) router.MiddlewareFunc {
	allowed := make(map[string]struct{}, len(hosts))
	wildcard := false
	for _, h := range hosts {
		if h == "*" {
			wildcard = true
			break
		}
		allowed[strings.ToLower(h)] = struct{}{}
	}
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			if debug || wildcard {
				next(ctx)
				return
			}
			host := strings.ToLower(ctx.Request.Host)
			if i := strings.LastIndex(host, ":"); i != -1 {
				host = host[:i]
			}
			if _, ok := allowed[host]; !ok {
				http.Error(ctx.Writer, "400 Bad Request — host não permitido", http.StatusBadRequest)
				return
			}
			next(ctx)
		}
	}
}

func RequireAuth(a *auth.Authenticator) router.MiddlewareFunc {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			header := ctx.Request.Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				ctx.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			claims, err := a.ValidateToken(token)
			if err != nil {
				ctx.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			ctx.Set("claims", claims)
			next(ctx)
		}
	}
}

func CORS(allowedOrigins []string) router.MiddlewareFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			origin := ctx.Request.Header.Get("Origin")
			if _, ok := allowed[origin]; ok {
				h := ctx.Writer.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Vary", "Origin")
			}
			if ctx.Request.Method == http.MethodOptions {
				h := ctx.Writer.Header()
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token")
				h.Set("Access-Control-Max-Age", "86400")
				ctx.Writer.WriteHeader(http.StatusNoContent)
				return
			}
			next(ctx)
		}
	}
}

// SecureHeaders adiciona cabeçalhos de segurança em produção.
// Deve ser registrado com r.Use() no bootstrap quando !debug.
func SecureHeaders(next router.HandlerFunc) router.HandlerFunc {
	return func(ctx *router.Context) {
		h := ctx.Writer.Header()
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		h.Set("X-Permitted-Cross-Domain-Policies", "none")
		next(ctx)
	}
}

// LocalhostOnly rejeita requisições que não venham de 127.0.0.1 ou ::1.
// Use para proteger endpoints internos registrados com r.Internal().
func LocalhostOnly(next router.HandlerFunc) router.HandlerFunc {
	return func(ctx *router.Context) {
		if !isLocalhost(ctx.Request.RemoteAddr) {
			http.Error(ctx.Writer, "403 Forbidden", http.StatusForbidden)
			return
		}
		next(ctx)
	}
}

// LocalhostOnlyHandler é a variante http.Handler de LocalhostOnly.
// Use para proteger handlers registrados com r.HandlePrefix().
func LocalhostOnlyHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalhost(r.RemoteAddr) {
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalhost(remoteAddr string) bool {
	ip := remoteAddr
	if i := strings.LastIndex(ip, ":"); i != -1 {
		ip = ip[:i]
	}
	ip = strings.Trim(ip, "[]")
	return ip == "127.0.0.1" || ip == "::1"
}

// MaxBodySize limita o tamanho do corpo da requisição.
// Use com r.Use() para aplicar globalmente ou por rota.
func MaxBodySize(maxBytes int64) router.MiddlewareFunc {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxBytes)
			next(ctx)
		}
	}
}

// RequireLogin protege rotas SSR que exigem sessão ativa.
// É um no-op completo quando DB_ENABLED=false — nenhuma leitura de sessão ocorre.
// Se não houver sessão válida, redireciona para loginURL com ?next=<caminho_atual>.
// Caso contrário, coloca a sessão em ctx com a chave "session".
func RequireLogin(store *session.Store, loginURL string) router.MiddlewareFunc {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			if !auth.IsDBEnabled() {
				next(ctx)
				return
			}
			sess, ok := session.FromRequest(ctx.Request, store)
			if !ok {
				dest, _ := url.Parse(loginURL)
				q := dest.Query()
				q.Set("next", ctx.Request.URL.RequestURI())
				dest.RawQuery = q.Encode()
				http.Redirect(ctx.Writer, ctx.Request, dest.String(), http.StatusFound)
				return
			}
			ctx.Set("session", sess)
			next(ctx)
		}
	}
}

func Recovery() router.MiddlewareFunc {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			defer func() {
				if rec := recover(); rec != nil {
					kyerrors.RenderPanic(ctx.Writer, ctx.Request, rec, debug.Stack())
				}
			}()
			next(ctx)
		}
	}
}
