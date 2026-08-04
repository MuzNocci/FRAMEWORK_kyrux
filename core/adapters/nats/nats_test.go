package nats

// Teste de integração real contra um NATS de verdade (container Docker).
// Pulado (t.Skip) se o servidor não estiver acessível.

import (
	"context"
	"os"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	url := envOr("KYRUX_TEST_NATS_URL", "nats://127.0.0.1:14222")
	a := New("teste", url)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := a.Configure(ctx); err != nil {
		t.Skipf("nats indisponível em %s: %v", url, err)
	}
	return a
}

func TestNATSPublicaEAssina(t *testing.T) {
	a := openTestAdapter(t)
	defer a.Shutdown(context.Background())

	conn := a.Value()
	received := make(chan string, 1)

	sub, err := conn.Subscribe("kyrux.teste", func(m *natsgo.Msg) {
		received <- string(m.Data)
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	if err := conn.Publish("kyrux.teste", []byte("mensagem de teste do kyrux")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	conn.Flush()

	select {
	case msg := <-received:
		if msg != "mensagem de teste do kyrux" {
			t.Errorf("esperava %q, recebeu %q", "mensagem de teste do kyrux", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout esperando a mensagem publicada")
	}
}

func TestNATSURLVaziaFalhaEmInit(t *testing.T) {
	a := New("teste", "")
	if err := a.Init(context.Background()); err == nil {
		t.Error("esperava erro de Init com url vazia")
	}
}

func TestNATSURLInvalidaFalhaEmConfigure(t *testing.T) {
	a := New("teste", "nats://127.0.0.1:1")
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Configure(ctx); err == nil {
		t.Error("esperava erro de Configure com servidor inacessível")
	}
}
