// Package apigraphql é o adapter que expõe um servidor GraphQL
// (github.com/graphql-go/graphql + github.com/graphql-go/handler) como um
// Module do Core (kyrux/core).
//
// Ao contrário de restapi/sqlpostgres, este pacote NÃO é importado por
// kyrux/core — graphql-go arrasta dependências reais (html/template,
// compress/gzip, o parser/lexer da linguagem GraphQL) que a maioria dos
// projetos Kyrux nunca vai usar. Importe este pacote você mesmo, na sua
// aplicação, só se for expor uma API GraphQL — mesma filosofia dos clients
// NoSQL (core/nosql/*) e dos drivers SQL: o framework nunca importa por você.
//
// Ativação: como este adapter recebe parâmetros de construção (endereço e
// schema), ele não passa pelo registro por nome (core/registry) — construa
// com New e ative com core.UseModule:
//
//	schema, _ := apigraphql.BuildSchema(...) // seu código, com graphql-go
//	_, err := core.UseModule[*graphql.Schema](c, apigraphql.New(addr, schema), "api.graphql.principal")
package apigraphql

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
)

// Adapter implementa registry.Module para um servidor GraphQL nomeado (por
// endereço).
type Adapter struct {
	addr   string
	schema graphql.Schema
	srv    *http.Server
	ln     net.Listener
}

// New cria (mas ainda não escuta — isso só acontece em Start) um adapter
// GraphQL. addr é o endereço de bind (ex: "127.0.0.1:8082"); schema é
// construído pelo chamador com a API de github.com/graphql-go/graphql —
// este adapter só serve o schema já pronto, não o define.
func New(addr string, schema graphql.Schema) *Adapter {
	return &Adapter{addr: addr, schema: schema}
}

func (a *Adapter) Name() string { return "api.graphql." + a.addr }

func (a *Adapter) Init(ctx context.Context) error {
	if a.addr == "" {
		return fmt.Errorf("apigraphql: endereço vazio")
	}
	return nil
}

// Configure não faz nada hoje — o schema já chega pronto de New. Existe só
// pra cumprir a interface Module.
func (a *Adapter) Configure(ctx context.Context) error { return nil }

// Start abre a porta (falha aqui, de forma síncrona, se já estiver em uso)
// e começa a servir numa goroutine própria. GraphiQL (a IDE embutida de
// exploração de schema) fica habilitada — desative na sua própria camada
// (proxy/middleware) se isso não for desejável em produção.
func (a *Adapter) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.addr)
	if err != nil {
		return fmt.Errorf("apigraphql: escutar em %s: %w", a.addr, err)
	}
	a.ln = ln

	h := handler.New(&handler.Config{
		Schema:   &a.schema,
		Pretty:   true,
		GraphiQL: true,
	})
	a.srv = &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go a.srv.Serve(ln)
	return nil
}

// Shutdown encerra o servidor HTTP de forma organizada (aguarda requisições
// em andamento, até 5s).
func (a *Adapter) Shutdown(ctx context.Context) error {
	if a.srv == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.srv.Shutdown(shutdownCtx)
}

// Value devolve o *graphql.Schema servido — o mesmo passado a New.
func (a *Adapter) Value() *graphql.Schema { return &a.schema }
