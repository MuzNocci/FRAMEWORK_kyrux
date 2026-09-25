package csrf

import (
	"bytes"
	"mime/multipart"
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
		req.Header.Set("Cookie", cookieName()+"="+cookie)
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
		if c.Name == cookieName() {
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

// TestCookieNamePrefix garante que o nome do cookie leva __Host- só quando
// Secure está ligado (produção) — sem isso o navegador rejeitaria o cookie
// inteiro em HTTP puro (dev), derrubando CSRF em todo POST/PUT/PATCH/DELETE.
func TestCookieNamePrefix(t *testing.T) {
	t.Cleanup(func() { SetSecure(false) })

	SetSecure(false)
	if got := cookieName(); got != baseCookieName {
		t.Errorf("dev (Secure=false): esperava %q, recebeu %q", baseCookieName, got)
	}

	SetSecure(true)
	want := "__Host-" + baseCookieName
	if got := cookieName(); got != want {
		t.Errorf("produção (Secure=true): esperava %q, recebeu %q", want, got)
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

// multipartRequest monta um POST multipart/form-data com o token CSRF e um
// anexo de attachmentSize bytes — simula o formulário de briefing.
func multipartRequest(t *testing.T, cookie, token string, attachmentSize int) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if token != "" {
		if err := w.WriteField(fieldName, token); err != nil {
			t.Fatal(err)
		}
	}
	if attachmentSize > 0 {
		fw, err := w.CreateFormFile("attachments", "anexo.bin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(make([]byte, attachmentSize)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/start-project/", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if cookie != "" {
		req.Header.Set("Cookie", cookieName()+"="+cookie)
	}
	return req
}

// TestMultipartComAnexoValido garante que um POST multipart com anexo
// dentro do limite e token correto continua sendo aceito normalmente —
// era esse o caminho do formulário de "Iniciar Projeto" (com upload de
// arquivo), diferente do form simples de /contact/.
func TestMultipartComAnexoValido(t *testing.T) {
	SetSecret(testSecret)
	raw, _ := generate()
	req := multipartRequest(t, raw, sign(raw), 1024)
	rec := httptest.NewRecorder()
	Middleware(func(ctx *router.Context) {
		ctx.Writer.WriteHeader(http.StatusOK)
	})(&router.Context{Writer: rec, Request: req})
	if rec.Code != http.StatusOK {
		t.Errorf("multipart com anexo e token válido: esperava 200, recebeu %d — corpo: %s", rec.Code, rec.Body.String())
	}
}

// TestMultipartCorpoGrandeDemaisNaoViraCSRFInvalido é a regressão do bug
// real: antes, um corpo maior que o limite fazia ParseMultipartForm falhar
// e descartar o form inteiro (inclusive o token CSRF já lido corretamente),
// e o middleware reportava "403 CSRF inválido" — escondendo que o problema
// era o tamanho do corpo. Agora deve ser um 400 claro, não um 403.
func TestMultipartCorpoGrandeDemaisNaoViraCSRFInvalido(t *testing.T) {
	SetSecret(testSecret)
	raw, _ := generate()
	req := multipartRequest(t, raw, sign(raw), maxRequestBody+1024)
	rec := httptest.NewRecorder()
	Middleware(func(ctx *router.Context) {
		ctx.Writer.WriteHeader(http.StatusOK)
	})(&router.Context{Writer: rec, Request: req})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("multipart acima do limite: esperava 400, recebeu %d — corpo: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "CSRF") {
		t.Errorf("erro de corpo grande demais não deveria mencionar CSRF (mensagem enganosa): %s", rec.Body.String())
	}
}
