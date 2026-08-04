// Package rabbitmq é o adapter que expõe um client RabbitMQ
// (github.com/rabbitmq/amqp091-go — sucessor oficial do streadway/amqp)
// como um Module do Core (kyrux/core).
//
// Ao contrário de restapi/sqlpostgres, este pacote NÃO é importado por
// kyrux/core — um client RabbitMQ é uma dependência real que a maioria dos
// projetos Kyrux nunca vai usar. Importe este pacote você mesmo, na sua
// aplicação, só se for publicar/consumir de um RabbitMQ — mesma filosofia
// dos clients NoSQL (core/nosql/*) e dos outros adapters de fila/API.
//
// Ativação: como este adapter recebe parâmetros de construção (nome lógico
// e URL AMQP), ele não passa pelo registro por nome (core/registry) —
// construa com New e ative com core.UseModule:
//
//	ch, err := core.UseModule[*amqp.Channel](c, rabbitmq.New("principal", "amqp://guest:guest@localhost:5672/"), "queue.rabbitmq.principal")
//	ch.QueueDeclare("pedidos", true, false, false, false, nil)
//	ch.Publish("", "pedidos", false, false, amqp.Publishing{Body: []byte("...")})
package rabbitmq

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Adapter implementa registry.Module para uma conexão RabbitMQ nomeada.
type Adapter struct {
	name string
	url  string
	conn *amqp.Connection
	ch   *amqp.Channel
}

// New cria (mas ainda não conecta — isso só acontece em Configure) um
// adapter RabbitMQ. name identifica esta conexão entre outras do mesmo tipo
// (usado só em Name()/logs — a URL, que carrega credenciais, nunca aparece
// ali). url é a URI AMQP completa (ex: "amqp://guest:guest@localhost:5672/").
func New(name, url string) *Adapter {
	return &Adapter{name: name, url: url}
}

func (a *Adapter) Name() string { return "queue.rabbitmq." + a.name }

func (a *Adapter) Init(ctx context.Context) error {
	if a.url == "" {
		return fmt.Errorf("rabbitmq: url vazia para a conexão %q", a.name)
	}
	return nil
}

// Configure abre a conexão e um Channel — é aqui que uma URL ou servidor
// inválido se manifesta como erro.
func (a *Adapter) Configure(ctx context.Context) error {
	conn, err := amqp.Dial(a.url)
	if err != nil {
		return fmt.Errorf("rabbitmq: conectar (%s): %w", a.name, err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("rabbitmq: abrir channel (%s): %w", a.name, err)
	}
	a.conn = conn
	a.ch = ch
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

// Shutdown fecha o Channel e a conexão, nessa ordem. Tenta fechar os dois
// mesmo se o primeiro falhar.
func (a *Adapter) Shutdown(ctx context.Context) error {
	var firstErr error
	if a.ch != nil {
		if err := a.ch.Close(); err != nil {
			firstErr = err
		}
	}
	if a.conn != nil {
		if err := a.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Value devolve o *amqp.Channel já pronto — QueueDeclare/Publish/Consume
// (e qualquer outra operação AMQP) ficam disponíveis diretamente nele.
func (a *Adapter) Value() *amqp.Channel { return a.ch }
