package core_test

// Prova de ponta a ponta do core.Mail.SMTP: envia um e-mail de verdade via
// SMTP contra um servidor real (Mailpit, container Docker) e confirma o
// recebimento consultando a API HTTP do Mailpit — não é só "não retornou
// erro", é "o servidor realmente recebeu a mensagem certa".

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"kyrux/core"
	"kyrux/core/mail"
)

type mailpitMessage struct {
	Subject string `json:"Subject"`
	From    struct {
		Address string `json:"Address"`
	} `json:"From"`
}

type mailpitList struct {
	Total    int              `json:"total"`
	Messages []mailpitMessage `json:"messages"`
}

func TestCoreMailSMTPEnviaEConfirmaRecebimento(t *testing.T) {
	host := envOr("KYRUX_TEST_SMTP_HOST", "127.0.0.1")
	port := envOr("KYRUX_TEST_SMTP_PORT", "11025")
	apiBase := envOr("KYRUX_TEST_MAILPIT_API", "http://127.0.0.1:18025")

	c := core.New()
	client, err := c.Mail.SMTP("teste", host, port, "", "", false)
	if err != nil {
		t.Skipf("smtp indisponível em %s:%s: %v", host, port, err)
	}

	subject := "assunto de teste do kyrux " + time.Now().Format(time.RFC3339Nano)
	msg := mail.Message{
		From:    "remetente@kyrux.teste",
		To:      []string{"destinatario@kyrux.teste"},
		Subject: subject,
		Text:    "corpo em texto puro",
		HTML:    "<p>corpo em <b>HTML</b></p>",
	}
	if err := client.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// A API do Mailpit reflete o recebimento quase instantaneamente, mas
	// tenta com retries curtos em vez de assumir isso.
	var found bool
	for i := 0; i < 20 && !found; i++ {
		resp, err := http.Get(apiBase + "/api/v1/messages")
		if err == nil {
			var list mailpitList
			if json.NewDecoder(resp.Body).Decode(&list) == nil {
				for _, m := range list.Messages {
					if m.Subject == subject {
						found = true
						if m.From.Address != "remetente@kyrux.teste" {
							t.Errorf("esperava From=remetente@kyrux.teste, recebeu %q", m.From.Address)
						}
					}
				}
			}
			resp.Body.Close()
		}
		if !found {
			time.Sleep(200 * time.Millisecond)
		}
	}
	if !found {
		t.Fatal("mensagem enviada não apareceu na caixa do Mailpit")
	}
}

func TestCoreMailSMTPHostVazioFalhaEmInit(t *testing.T) {
	c := core.New()
	if _, err := c.Mail.SMTP("teste", "", "587", "", "", false); err == nil {
		t.Error("esperava erro de Init sem host")
	}
}

func TestCoreMailSMTPServidorInacessivelFalhaEmConfigure(t *testing.T) {
	c := core.New()
	if _, err := c.Mail.SMTP("teste", "127.0.0.1", "1", "", "", false); err == nil {
		t.Error("esperava erro de Configure com servidor inacessível")
	}
}
