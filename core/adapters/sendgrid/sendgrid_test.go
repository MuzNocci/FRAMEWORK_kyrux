package sendgrid

// Teste de integração real do protocolo HTTP contra um mock da API do
// SendGrid (não há como testar contra a API real sem uma conta/chave) —
// verifica que a requisição de verdade (autenticação, JSON do corpo)
// corresponde ao que a API do SendGrid espera, e que o client trata a
// resposta corretamente.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	kymail "kyrux/core/mail"
)

func TestSendGridEnviaRequisicaoCorreta(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/mail/send" || r.Method != http.MethodPost {
			t.Errorf("esperava POST /v3/mail/send, recebeu %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	a := newWithHost("teste", "minha-api-key", srv.URL)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := a.Configure(ctx); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	err := a.Value().Send(ctx, kymail.Message{
		From:    "remetente@kyrux.teste",
		To:      []string{"destinatario@kyrux.teste"},
		Subject: "assunto de teste",
		Text:    "corpo em texto",
		HTML:    "<p>corpo em html</p>",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotAuth != "Bearer minha-api-key" {
		t.Errorf("esperava Authorization=%q, recebeu %q", "Bearer minha-api-key", gotAuth)
	}
	if gotBody["subject"] != "assunto de teste" {
		t.Errorf("esperava subject correto no corpo JSON, recebeu %+v", gotBody)
	}
	from, _ := gotBody["from"].(map[string]any)
	if from["email"] != "remetente@kyrux.teste" {
		t.Errorf("esperava from.email correto, recebeu %+v", from)
	}
}

func TestSendGridRespostaDeErroFalha(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errors":[{"message":"api key inválida"}]}`))
	}))
	defer srv.Close()

	a := newWithHost("teste", "chave-errada", srv.URL)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Configure(ctx); err != nil {
		t.Fatal(err)
	}

	err := a.Value().Send(ctx, kymail.Message{From: "a@b.com", To: []string{"c@d.com"}, Subject: "x", Text: "y"})
	if err == nil {
		t.Error("esperava erro para uma resposta 401 do SendGrid")
	}
}

func TestSendGridAPIKeyVaziaFalhaEmInit(t *testing.T) {
	a := New("teste", "")
	if err := a.Init(context.Background()); err == nil {
		t.Error("esperava erro de Init sem api key")
	}
}
