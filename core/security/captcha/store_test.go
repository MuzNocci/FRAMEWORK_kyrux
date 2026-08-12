package captcha

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kyrux/core/router"
	"kyrux/core/security/session"
)

// imageHandlerRequest chama ImageHandler numa requisição nova (sem cookie
// de sessão) e devolve a resposta gravada + o cookie de sessão criado, pra
// os testes conseguirem inspecionar o que foi guardado e simular a
// requisição seguinte (o POST do formulário) com o mesmo cookie.
func imageHandlerRequest(t *testing.T, store *Store) (*httptest.ResponseRecorder, *http.Cookie) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/captcha/image", nil)
	rec := httptest.NewRecorder()
	ctx := &router.Context{Writer: rec, Request: req}

	store.ImageHandler()(ctx)

	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == session.CookieName() {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("ImageHandler não gravou o cookie de sessão")
	}
	return rec, sessionCookie
}

func TestImageHandlerServePNGComHeadersCorretos(t *testing.T) {
	sessions := session.NewStore(time.Hour)
	store := NewStore(sessions)

	rec, _ := imageHandlerRequest(t, store)

	if rec.Code != 200 {
		t.Fatalf("status = %d, esperava 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %q, esperava image/png", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, esperava no-store", cc)
	}
	if _, err := png.Decode(bytes.NewReader(rec.Body.Bytes())); err != nil {
		t.Fatalf("corpo da resposta não é um PNG válido: %v", err)
	}
}

func TestImageHandlerGuardaCodigoDeCodeLengthDigitosNaSessao(t *testing.T) {
	sessions := session.NewStore(time.Hour)
	store := NewStore(sessions)

	_, cookie := imageHandlerRequest(t, store)

	sess, ok := sessions.Get(cookie.Value)
	if !ok {
		t.Fatal("sessão criada pelo ImageHandler não foi encontrada no Store")
	}
	codeVal, ok := sess.Get(sessionKey)
	if !ok {
		t.Fatal("sessão não tem o código do captcha guardado")
	}
	code, _ := codeVal.(string)
	if len(code) != CodeLength {
		t.Fatalf("código guardado = %q, esperava %d dígitos", code, CodeLength)
	}
}

// requestWithSessionCookie monta uma requisição carregando o cookie de
// sessão dado, simulando o navegador reenviando o cookie no POST seguinte.
func requestWithSessionCookie(cookie *http.Cookie) (*router.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPost, "/contato/", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	return &router.Context{Writer: rec, Request: req}, rec
}

func TestVerifyAceitaRespostaCorretaEConsomeOCodigo(t *testing.T) {
	sessions := session.NewStore(time.Hour)
	store := NewStore(sessions)

	_, cookie := imageHandlerRequest(t, store)
	sess, _ := sessions.Get(cookie.Value)
	codeVal, _ := sess.Get(sessionKey)
	code, _ := codeVal.(string)

	ctx, _ := requestWithSessionCookie(cookie)
	if !store.Verify(ctx, code) {
		t.Fatal("Verify recusou a resposta correta")
	}

	// Uso único: a mesma resposta não deveria funcionar de novo.
	ctx2, _ := requestWithSessionCookie(cookie)
	if store.Verify(ctx2, code) {
		t.Fatal("Verify aceitou a mesma resposta uma segunda vez — código deveria ter sido consumido")
	}
}

func TestVerifyRecusaRespostaErradaEConsomeOCodigo(t *testing.T) {
	sessions := session.NewStore(time.Hour)
	store := NewStore(sessions)

	_, cookie := imageHandlerRequest(t, store)

	ctx, _ := requestWithSessionCookie(cookie)
	if store.Verify(ctx, "00000000") {
		t.Fatal("Verify aceitou uma resposta claramente errada")
	}

	sess, _ := sessions.Get(cookie.Value)
	if _, ok := sess.Get(sessionKey); ok {
		t.Fatal("código deveria ter sido consumido mesmo numa tentativa errada")
	}
}

func TestVerifySemCookieDeSessaoRecusa(t *testing.T) {
	sessions := session.NewStore(time.Hour)
	store := NewStore(sessions)

	req := httptest.NewRequest(http.MethodPost, "/contato/", nil)
	ctx := &router.Context{Writer: httptest.NewRecorder(), Request: req}

	if store.Verify(ctx, "12345") {
		t.Fatal("Verify aceitou uma requisição sem sessão nenhuma")
	}
}
