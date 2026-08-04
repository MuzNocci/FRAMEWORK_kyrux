package kafka

// Teste de integração real contra um Kafka de verdade (container Docker,
// modo KRaft — sem Zookeeper). Pulado (t.Skip) se o broker não estiver
// acessível.

import (
	"context"
	"os"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	broker := envOr("KYRUX_TEST_KAFKA_BROKER", "127.0.0.1:19092")
	a := New(broker)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := a.Configure(ctx); err != nil {
		t.Skipf("kafka indisponível em %s: %v", broker, err)
	}
	return a
}

func TestKafkaProduzEConsome(t *testing.T) {
	broker := envOr("KYRUX_TEST_KAFKA_BROKER", "127.0.0.1:19092")
	a := openTestAdapter(t)
	defer a.Shutdown(context.Background())

	client := a.Value()
	topic := "kyrux-teste-produz-consome"

	// Cria o tópico explicitamente — mais confiável em teste do que confiar
	// só no AllowAutoTopicCreation do Writer (a primeira escrita pode
	// chegar antes do tópico existir de fato no broker).
	conn, err := kafkago.Dial("tcp", broker)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := conn.CreateTopics(kafkago.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}); err != nil {
		t.Fatalf("CreateTopics: %v", err)
	}
	conn.Close()

	w := client.Producer(topic)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// A criação do tópico é assíncrona no cluster — tenta escrever com
	// retries curtos em vez de assumir que já está pronto.
	for i := 0; i < 20; i++ {
		err = w.WriteMessages(ctx, kafkago.Message{
			Key:   []byte("chave-1"),
			Value: []byte("mensagem de teste do kyrux"),
		})
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("WriteMessages: %v", err)
	}

	r := client.Consumer(topic, "kyrux-teste-grupo")
	readCtx, readCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer readCancel()
	msg, err := r.ReadMessage(readCtx)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(msg.Value) != "mensagem de teste do kyrux" {
		t.Errorf("esperava %q, recebeu %q", "mensagem de teste do kyrux", string(msg.Value))
	}
	if string(msg.Key) != "chave-1" {
		t.Errorf("esperava chave %q, recebeu %q", "chave-1", string(msg.Key))
	}
}

func TestKafkaSemBrokersFalhaEmInit(t *testing.T) {
	a := New()
	if err := a.Init(context.Background()); err == nil {
		t.Error("esperava erro de Init sem brokers")
	}
}

func TestKafkaBrokerInvalidoFalhaEmConfigure(t *testing.T) {
	a := New("127.0.0.1:1")
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Configure(ctx); err == nil {
		t.Error("esperava erro de Configure com broker inacessível")
	}
}
