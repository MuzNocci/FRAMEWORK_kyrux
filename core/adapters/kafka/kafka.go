// Package kafka é o adapter que expõe um client Kafka
// (github.com/segmentio/kafka-go — puro Go, sem cgo/librdkafka) como um
// Module do Core (kyrux/core).
//
// Ao contrário de restapi/sqlpostgres, este pacote NÃO é importado por
// kyrux/core — um client Kafka é uma dependência real que a maioria dos
// projetos Kyrux nunca vai usar. Importe este pacote você mesmo, na sua
// aplicação, só se for produzir/consumir de um Kafka — mesma filosofia dos
// clients NoSQL (core/nosql/*) e dos outros adapters de API (GraphQL, gRPC).
//
// Ativação: como este adapter recebe parâmetros de construção (lista de
// brokers), ele não passa pelo registro por nome (core/registry) — construa
// com New e ative com core.UseModule:
//
//	client, err := core.UseModule[*kafka.Client](c, kafka.New("localhost:9092"), "queue.kafka.principal")
//	w := client.Producer("pedidos")
//	r := client.Consumer("pedidos", "meu-grupo")
package kafka

import (
	"context"
	"fmt"
	"strings"
	"sync"

	kafkago "github.com/segmentio/kafka-go"
)

// Client agrega producers e consumers Kafka sobre o mesmo conjunto de
// brokers — Producer/Consumer abrem um *kafka.Writer/*kafka.Reader por
// tópico (o padrão recomendado pela própria lib), todos fechados juntos em
// Close.
type Client struct {
	brokers []string

	mu      sync.Mutex
	writers []*kafkago.Writer
	readers []*kafkago.Reader
}

// Producer abre um producer para topic.
func (c *Client) Producer(topic string) *kafkago.Writer {
	w := &kafkago.Writer{
		Addr:                   kafkago.TCP(c.brokers...),
		Topic:                  topic,
		Balancer:               &kafkago.LeastBytes{},
		AllowAutoTopicCreation: true,
	}
	c.mu.Lock()
	c.writers = append(c.writers, w)
	c.mu.Unlock()
	return w
}

// Consumer abre um consumer para topic, associado a groupID — offsets são
// gerenciados pelo Kafka por grupo; reiniciar com o mesmo groupID continua
// de onde parou.
func (c *Client) Consumer(topic, groupID string) *kafkago.Reader {
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: c.brokers,
		Topic:   topic,
		GroupID: groupID,
	})
	c.mu.Lock()
	c.readers = append(c.readers, r)
	c.mu.Unlock()
	return r
}

// Close fecha todos os producers e consumers abertos por este client.
// Tenta fechar todos mesmo se algum falhar — devolve o primeiro erro.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var firstErr error
	for _, w := range c.writers {
		if err := w.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, r := range c.readers {
		if err := r.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Adapter implementa registry.Module para um client Kafka nomeado (pela
// lista de brokers).
type Adapter struct {
	brokers []string
	client  *Client
}

// New cria (mas ainda não conecta — isso só acontece em Configure) um
// adapter Kafka para os brokers informados (ex: "localhost:9092").
func New(brokers ...string) *Adapter {
	return &Adapter{brokers: brokers}
}

func (a *Adapter) Name() string { return "queue.kafka." + strings.Join(a.brokers, ",") }

func (a *Adapter) Init(ctx context.Context) error {
	if len(a.brokers) == 0 {
		return fmt.Errorf("kafka: nenhum broker informado")
	}
	return nil
}

// Configure testa a conectividade (busca a lista de brokers do cluster)
// antes de devolver um client que pareceria pronto mas não é.
func (a *Adapter) Configure(ctx context.Context) error {
	conn, err := kafkago.DialContext(ctx, "tcp", a.brokers[0])
	if err != nil {
		return fmt.Errorf("kafka: conectar em %v: %w", a.brokers, err)
	}
	defer conn.Close()
	if _, err := conn.Brokers(); err != nil {
		return fmt.Errorf("kafka: listar brokers: %w", err)
	}
	a.client = &Client{brokers: a.brokers}
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

func (a *Adapter) Shutdown(ctx context.Context) error {
	if a.client != nil {
		return a.client.Close()
	}
	return nil
}

// Value devolve o *Client já pronto.
func (a *Adapter) Value() *Client { return a.client }
