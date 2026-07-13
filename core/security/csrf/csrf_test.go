package csrf

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"kyrux/core/router"
)

const testSecret = "uma-chave-de-teste-com-32-caracteres!!"

func doRequest(t *testing.T, method, path, cookie, token string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if method == "POST" && token != "" {
		form := url.Values{fieldName: {token}}
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookieName+"="+cookie)
	}
	rec := httptest.NewRecorder()
	Middleware(func(ctx *router.Context) {
		ctx.Writer.WriteHeader(http.StatusOK)
	})(&router.Context{Writer: rec, Request: req})
	return rec
}

// TestFluxoCompleto simula o ciclo real: GET cria o cookie, o form envia o
// token assinado (o que {{ csrf_token }}/TokenFor gera) e o POST é aceito.
func TestFluxoCompleto(t *testing.T) {
	SetSecret(testSecret)

	// GET inicial: cookie criado, sem exigência de token.
	rec := doRequest(t, "GET", "/", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: esperava 200, recebeu %d", rec.Code)
	}
	var raw string
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName {
			raw = c.Value
		}
	}
	if raw == "" {
		t.Fatal("GET deveria ter criado o cookie CSRF")
	}

	// POST com o token assinado correto: aceito.
	if rec := doRequest(t, "POST", "/", raw, sign(raw)); rec.Code != http.StatusOK {
		t.Errorf("POST com token válido: esperava 200, recebeu %d", rec.Code)
	}

	// POST sem token: 403.
	if rec := doRequest(t, "POST", "/", raw, ""); rec.Code != http.StatusForbidden {
		t.Errorf("POST sem token: esperava 403, recebeu %d", rec.Code)
	}

	// POST com o cookie BRUTO como token (não assinado): 403.
	if rec := doRequest(t, "POST", "/", raw, raw); rec.Code != http.StatusForbidden {
		t.Errorf("POST com cookie bruto: esperava 403, recebeu %d", rec.Code)
	}
}

// TestTokenForLazy garante que TokenFor computa o token assinado a partir do
// raw colocado pelo middleware e o cacheia no ctx.
func TestTokenForLazy(t *testing.T) {
	SetSecret(testSecret)
	raw, _ := generate()

	ctx := &router.Context{}
	ctx.Set(rawKey, raw)

	tok := TokenFor(ctx)
	if tok != sign(raw) {
		t.Fatalf("TokenFor deveria devolver o HMAC do raw")
	}
	// Segunda chamada usa o cache do ctx (mesmo valor).
	if TokenFor(ctx) != tok {
		t.Error("TokenFor deveria ser estável na mesma request")
	}
}

// TestExempt garante que prefixos isentos pulam a validação em POST.
func TestExempt(t *testing.T) {
	SetSecret(testSecret)
	Exempt("/api-teste/")
	raw, _ := generate()

	if rec := doRequest(t, "POST", "/api-teste/coisa/", raw, ""); rec.Code != http.StatusOK {
		t.Errorf("POST em rota isenta: esperava 200, recebeu %d", rec.Code)
	}
	if rec := doRequest(t, "POST", "/nao-isento/", raw, ""); rec.Code != http.StatusForbidden {
		t.Errorf("POST fora da isenção: esperava 403, recebeu %d", rec.Code)
	}
}
