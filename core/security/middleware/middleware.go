package middleware

import (
	kyerrors "kyrux/core/errors"
	"kyrux/core/router"
	"kyrux/core/security/auth"
	"kyrux/core/security/session"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
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
			// net.SplitHostPort trata IPv6 ([::1]:8000) corretamente;
			// sem porta, remove só os colchetes do literal IPv6.
			host := ctx.Request.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			host = strings.ToLower(strings.Trim(host, "[]"))
			if _, ok := allowed[host]; !ok {
				kyerrors.Render(ctx.Writer, ctx.Request, http.StatusBadRequest)
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

var (
	corsAllowMethodsHeader = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsAllowHeadersHeader = "Content-Type, Authorization, X-CSRF-Token"
	corsMaxAgeHeader       = "86400"
)

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
				h.Set("Access-Control-Allow-Methods", corsAllowMethodsHeader)
				h.Set("Access-Control-Allow-Headers", corsAllowHeadersHeader)
				h.Set("Access-Control-Max-Age", corsMaxAgeHeader)
				ctx.Writer.WriteHeader(http.StatusNoContent)
				return
			}
			next(ctx)
		}
	}
}

// SecureHeaders adiciona cabeçalhos de segurança em produção.
// Deve ser registrado com r.Use() no bootstrap quando !debug.
// Slice em vez de map: sem hash walk no hot path (roda em toda request).
var secureHeadersList = [...][2]string{
	{"Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload"},
	{"X-Content-Type-Options", "nosniff"},
	{"X-Frame-Options", "DENY"},
	{"Referrer-Policy", "strict-origin-when-cross-origin"},
	{"Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:"},
	{"Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()"},
	{"X-Permitted-Cross-Domain-Policies", "none"},
}

func SecureHeaders(next router.HandlerFunc) router.HandlerFunc {
	return func(ctx *router.Context) {
		h := ctx.Writer.Header()
		for _, kv := range secureHeadersList {
			h.Set(kv[0], kv[1])
		}
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
	ip := clientIP(remoteAddr)
	return ip == "127.0.0.1" || ip == "::1"
}

// rlEntry é a janela de contagem de um IP no RateLimit.
type rlEntry struct {
	count int
	reset time.Time
}

// RateLimit limita requisições por IP: no máximo max requisições por janela.
// Excedentes recebem 429 com Retry-After. Use nas rotas de autenticação para
// conter brute force — o custo do Argon2id torna o login um alvo de DoS:
//
//	router.Path("POST", "/login/", secmiddleware.RateLimit(10, time.Minute)(views.LoginPost(fw)), "login_post"),
//
// O estado é por processo (memória). Atrás de proxy reverso, configure o
// proxy para preservar o IP real ou aplique o limite no próprio proxy.
func RateLimit(max int, window time.Duration) router.MiddlewareFunc {
	var (
		mu        sync.Mutex
		entries   = make(map[string]*rlEntry)
		lastSweep = time.Now()
	)
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			ip := clientIP(ctx.Request.RemoteAddr)
			now := time.Now()

			mu.Lock()
			// Sweep periódico: remove janelas expiradas para o mapa não crescer sem teto.
			if now.Sub(lastSweep) > window {
				for k, e := range entries {
					if now.After(e.reset) {
						delete(entries, k)
					}
				}
				lastSweep = now
			}
			e, ok := entries[ip]
			if !ok || now.After(e.reset) {
				e = &rlEntry{reset: now.Add(window)}
				entries[ip] = e
			}
			e.count++
			blocked := e.count > max
			retryAfter := int(time.Until(e.reset).Seconds()) + 1
			mu.Unlock()

			if blocked {
				ctx.Writer.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				kyerrors.Render(ctx.Writer, ctx.Request, http.StatusTooManyRequests)
				return
			}
			next(ctx)
		}
	}
}

// clientIP extrai o IP (sem porta) de um RemoteAddr, com suporte a IPv6.
func clientIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return strings.Trim(remoteAddr, "[]")
}

// MaxBodySize limita o tamanho do corpo da requisição.
// Use com r.Use() para aplicar globalmente ou por rota.
func MaxBodySize(maxBytes int64) router.MiddlewareFunc {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			// Sem corpo (GET/HEAD típicos) não há o que limitar — evita
			// alocar o wrapper na maioria das requests SSR.
			if ctx.Request.Body != nil && ctx.Request.Body != http.NoBody {
				ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxBytes)
			}
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
