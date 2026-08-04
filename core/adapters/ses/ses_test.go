package ses

// Teste de integração real contra o SES emulado pelo LocalStack (container
// Docker). Pulado (t.Skip) se o serviço não estiver acessível.

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsses "github.com/aws/aws-sdk-go-v2/service/ses"

	kymail "kyrux/core/mail"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	endpoint := envOr("KYRUX_TEST_SES_ENDPOINT", "http://127.0.0.1:14566")
	a := New("teste", "us-east-1", endpoint, "test", "test")
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := a.Configure(ctx); err != nil {
		t.Skipf("ses/localstack indisponível em %s: %v", endpoint, err)
	}
	return a
}

func TestSESEnviaEmail(t *testing.T) {
	a := openTestAdapter(t)
	client := a.Value()
	ctx := context.Background()

	// Como o SES real, o LocalStack exige que o remetente seja uma
	// identidade verificada antes de aceitar o envio — diferente do SES
	// real, aqui a verificação é instantânea (não manda e-mail nenhum).
	if _, err := client.Raw().VerifyEmailIdentity(ctx, &awsses.VerifyEmailIdentityInput{
		EmailAddress: aws.String("remetente@kyrux.teste"),
	}); err != nil {
		t.Fatalf("VerifyEmailIdentity: %v", err)
	}

	err := client.Send(ctx, kymail.Message{
		From:    "remetente@kyrux.teste",
		To:      []string{"destinatario@kyrux.teste"},
		Subject: "assunto de teste do kyrux",
		Text:    "corpo em texto puro",
		HTML:    "<p>corpo em <b>HTML</b></p>",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestSESRegiaoVaziaFalhaEmInit(t *testing.T) {
	a := New("teste", "", "http://127.0.0.1:14566", "test", "test")
	if err := a.Init(context.Background()); err == nil {
		t.Error("esperava erro de Init sem região")
	}
}

func TestSESEndpointInvalidoFalhaEmConfigure(t *testing.T) {
	a := New("teste", "us-east-1", "http://127.0.0.1:1", "test", "test")
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Configure(ctx); err == nil {
		t.Error("esperava erro de Configure com endpoint inacessível")
	}
}

func TestSESRawDevolveClientNativo(t *testing.T) {
	a := openTestAdapter(t)
	if raw := a.Value().Raw(); raw == nil {
		t.Fatal("Raw() não deveria ser nil")
	} else if _, ok := any(raw).(*awsses.Client); !ok {
		t.Error("Raw() deveria devolver *ses.Client")
	}
}
