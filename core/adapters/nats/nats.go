// Package nats é o adapter que expõe um client NATS
// (github.com/nats-io/nats.go, o client oficial) como um Module do Core
// (kyrux/core).
//
// Ao contrário de restapi/sqlpostgres, este pacote NÃO é importado por
// kyrux/core — um client NATS é uma dependência real que a maioria dos
// projetos Kyrux nunca vai usar. Importe este pacote você mesmo, na sua
// aplicação, só se for publicar/consumir de um NATS — mesma filosofia dos
// clients NoSQL (core/nosql/*) e dos outros adapters de fila/API.
//
// Ativação: como este adapter recebe parâmetros de construção (nome lógico
// e URL), ele não passa pelo registro por nome (core/registry) — construa
// com New e ative com core.UseModule:
//
//	conn, err := core.UseModule[*nats.Conn](c, natsadapter.New("principal", "nats://localhost:4222"), "queue.nats.principal")
//	conn.Publish("pedidos", []byte("..."))
//	conn.Subscribe("pedidos", func(m *nats.Msg) { ... })
package nats

import (
	"context"
	"fmt"

	natsgo "github.com/nats-io/nats.go"
)

// Adapter implementa registry.Module para uma conexão NATS nomeada.
type Adapter struct {
	name string
	url  string
	conn *natsgo.Conn
}

// New cria (mas ainda não conecta — isso só acontece em Configure) um
// adapter NATS. name identifica esta conexão entre outras do mesmo tipo
// (usado só em Name()/logs). url é a URL do servidor (ex:
// "nats://localhost:4222"); aceita credenciais embutidas como qualquer URL
// NATS — evite logar url por causa disso, prefira o Name().
func New(name, url string) *Adapter {
	return &Adapter{name: name, url: url}
}

func (a *Adapter) Name() string { return "queue.nats." + a.name }

func (a *Adapter) Init(ctx context.Context) error {
	if a.url == "" {
		return fmt.Errorf("nats: url vazia para a conexão %q", a.name)
	}
	return nil
}

// Configure conecta de verdade — nats.Connect já faz o handshake inicial,
// então um servidor inacessível falha aqui, não silenciosamente depois.
func (a *Adapter) Configure(ctx context.Context) error {
	conn, err := natsgo.Connect(a.url)
	if err != nil {
		return fmt.Errorf("nats: conectar (%s): %w", a.name, err)
	}
	a.conn = conn
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

// Shutdown drena as inscrições pendentes e fecha a conexão de forma
// organizada.
func (a *Adapter) Shutdown(ctx context.Context) error {
	if a.conn != nil {
		return a.conn.Drain()
	}
	return nil
}

// Value devolve o *nats.Conn já pronto — Publish/Subscribe/Request (e
// qualquer outra operação NATS, incluindo JetStream via conn.JetStream())
// ficam disponíveis diretamente nele.
func (a *Adapter) Value() *natsgo.Conn { return a.conn }
