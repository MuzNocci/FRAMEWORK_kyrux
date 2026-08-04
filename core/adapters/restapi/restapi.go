// Package restapi é o adapter que expõe uma API REST (core/router — o mesmo
// motor de rotas usado por core/bootstrap) como um Module do Core
// (kyrux/core). Diferente do router usado por bootstrap.Init (ligado ao
// ciclo de vida da aplicação SSR tradicional), este router é independente:
// tem seu próprio *http.Server, sua própria porta e seu próprio
// Start/Shutdown, orquestrado pelo Core — permite rodar uma API REST lado a
// lado com (ou no lugar de) o servidor SSR tradicional.
package restapi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"kyrux/core/router"
)

// Adapter implementa registry.Module para uma API REST nomeada (por
// endereço) — recebe parâmetro de construção (addr), por isso não se
// autorregistra via init()/hot-plug; é construído diretamente por
// core.API.REST, que já importa este pacote (custo zero adicional:
// core/router já é dependência obrigatória do framework).
type Adapter struct {
	addr   string
	router *router.Router
	srv    *http.Server
	ln     net.Listener
}

// New cria (mas ainda não escuta — isso só acontece em Start) um adapter de
// API REST. addr é o endereço de bind (ex: "127.0.0.1:8081").
func New(addr string) *Adapter {
	return &Adapter{addr: addr}
}

func (a *Adapter) Name() string { return "api.rest." + a.addr }

func (a *Adapter) Init(ctx context.Context) error {
	if a.addr == "" {
		return fmt.Errorf("restapi: endereço vazio")
	}
	return nil
}

// Configure cria o router — instantâneo, sem I/O, então não há nada que
// possa falhar aqui hoje; o retorno de erro existe só pra cumprir a
// interface Module.
func (a *Adapter) Configure(ctx context.Context) error {
	a.router = router.New()
	return nil
}

// Start abre a porta (falha aqui, de forma síncrona, se já estiver em uso)
// e começa a servir numa goroutine própria — não bloqueia o Start do
// Lifecycle, que precisa seguir para os módulos seguintes.
func (a *Adapter) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.addr)
	if err != nil {
		return fmt.Errorf("restapi: escutar em %s: %w", a.addr, err)
	}
	a.ln = ln
	a.srv = &http.Server{
		Handler:           a.router,
		ReadHeaderTimeout: 5 * time.Second,
	}
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

// Value devolve o *router.Router para o chamador registrar rotas nele
// (router.Handle(pattern, handler)) antes de Core.Run() efetivamente
// começar a servir.
func (a *Adapter) Value() *router.Router { return a.router }
