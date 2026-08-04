package rabbitmq

// Teste de integração real contra um RabbitMQ de verdade (container
// Docker). Pulado (t.Skip) se o servidor não estiver acessível.

import (
	"context"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	url := envOr("KYRUX_TEST_RABBITMQ_URL", "amqp://guest:guest@127.0.0.1:15673/")
	a := New("teste", url)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := a.Configure(ctx); err != nil {
		t.Skipf("rabbitmq indisponível em %s: %v", url, err)
	}
	return a
}

func TestRabbitMQPublicaEConsome(t *testing.T) {
	a := openTestAdapter(t)
	defer a.Shutdown(context.Background())

	ch := a.Value()
	queueName := "kyrux-teste-publica-consome"

	// exclusive=true: RabbitMQ 4.x não permite mais filas transitórias
	// (durable=false) não-exclusivas — exclusive amarra a fila a este
	// Channel, o que é exatamente o que este teste precisa.
	q, err := ch.QueueDeclare(queueName, false, true, true, false, nil)
	if err != nil {
		t.Fatalf("QueueDeclare: %v", err)
	}

	msgs, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}

	if err := ch.Publish("", q.Name, false, false, amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte("mensagem de teste do kyrux"),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case msg := <-msgs:
		if string(msg.Body) != "mensagem de teste do kyrux" {
			t.Errorf("esperava %q, recebeu %q", "mensagem de teste do kyrux", string(msg.Body))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout esperando a mensagem publicada")
	}
}

func TestRabbitMQURLVaziaFalhaEmInit(t *testing.T) {
	a := New("teste", "")
	if err := a.Init(context.Background()); err == nil {
		t.Error("esperava erro de Init com url vazia")
	}
}

func TestRabbitMQURLInvalidaFalhaEmConfigure(t *testing.T) {
	a := New("teste", "amqp://guest:guest@127.0.0.1:1/")
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Configure(ctx); err == nil {
		t.Error("esperava erro de Configure com servidor inacessível")
	}
}
