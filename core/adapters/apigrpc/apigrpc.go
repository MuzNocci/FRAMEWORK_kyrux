// Package apigrpc é o adapter que expõe um servidor gRPC
// (google.golang.org/grpc) como um Module do Core (kyrux/core).
//
// Ao contrário de restapi/sqlpostgres, este pacote NÃO é importado por
// kyrux/core — google.golang.org/grpc + protobuf são dependências reais
// (HTTP/2 próprio, runtime de protobuf) que a maioria dos projetos Kyrux
// nunca vai usar. Importe este pacote você mesmo, na sua aplicação, só se
// for expor uma API gRPC — mesma filosofia dos clients NoSQL (core/nosql/*)
// e dos drivers SQL: o framework nunca importa por você.
//
// Como qualquer uso de gRPC em Go, você precisa gerar o código dos seus
// serviços a partir de um .proto (protoc + protoc-gen-go + protoc-gen-go-grpc)
// — este adapter não define serviços, só hospeda o *grpc.Server onde você
// registra os seus (pb.RegisterSeuServiceServer(srv, impl)).
//
// Ativação: como este adapter recebe parâmetros de construção (endereço, e
// opcionalmente grpc.ServerOption), ele não passa pelo registro por nome
// (core/registry) — construa com New e ative com core.UseModule:
//
//	srv, _ := core.UseModule[*grpc.Server](c, apigrpc.New(addr), "api.grpc.principal")
//	pb.RegisterSeuServiceServer(srv, &suaImplementacao{})
//	c.Run() // só aqui o servidor começa a escutar de fato
package apigrpc

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
)

// Adapter implementa registry.Module para um servidor gRPC nomeado (por
// endereço).
type Adapter struct {
	addr string
	opts []grpc.ServerOption
	srv  *grpc.Server
	ln   net.Listener
}

// New cria (mas ainda não escuta — isso só acontece em Start) um adapter
// gRPC. addr é o endereço de bind (ex: "127.0.0.1:9090"); opts são passadas
// direto para grpc.NewServer (interceptors, TLS, etc.).
func New(addr string, opts ...grpc.ServerOption) *Adapter {
	return &Adapter{addr: addr, opts: opts}
}

func (a *Adapter) Name() string { return "api.grpc." + a.addr }

func (a *Adapter) Init(ctx context.Context) error {
	if a.addr == "" {
		return fmt.Errorf("apigrpc: endereço vazio")
	}
	return nil
}

// Configure cria o *grpc.Server (sem escutar ainda) — é nele que o
// chamador registra seus serviços gerados antes de Core.Run().
func (a *Adapter) Configure(ctx context.Context) error {
	a.srv = grpc.NewServer(a.opts...)
	return nil
}

// Start abre a porta (falha aqui, de forma síncrona, se já estiver em uso)
// e começa a servir numa goroutine própria.
func (a *Adapter) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.addr)
	if err != nil {
		return fmt.Errorf("apigrpc: escutar em %s: %w", a.addr, err)
	}
	a.ln = ln
	go a.srv.Serve(ln)
	return nil
}

// Shutdown encerra o servidor de forma organizada (GracefulStop — espera
// RPCs em andamento terminarem, sem prazo — o próprio grpc.Server não tem
// timeout embutido aqui; adicione um watchdog na sua aplicação se precisar
// de um teto).
func (a *Adapter) Shutdown(ctx context.Context) error {
	if a.srv != nil {
		a.srv.GracefulStop()
	}
	return nil
}

// Value devolve o *grpc.Server já criado (mas só passa a aceitar conexões
// depois de Core.Run()) — registre seus serviços nele antes de chamar Run.
func (a *Adapter) Value() *grpc.Server { return a.srv }
