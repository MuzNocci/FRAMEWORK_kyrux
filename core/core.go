// Package core é a fundação de um sistema modular e plugável para o Kyrux —
// uma camada ADICIONAL que convive com core/bootstrap (a API tradicional do
// framework, com Framework/fw.DB/fw.Cache/fw.Queue) sem substituí-la. Nada
// em bootstrap.Init ou nos subsistemas já existentes (ORM, Cache, Queue,
// Admin, os clients NoSQL) muda por causa deste pacote.
//
// O objetivo aqui é permitir registrar, ativar e orquestrar módulos (bancos,
// caches, filas, ou qualquer outra coisa que implemente registry.Module) de
// forma plugável — cada módulo entra no binário só se a aplicação
// explicitamente importar o pacote do seu adapter, exatamente como já
// acontece com os drivers SQL (lib/pq, go-sql-driver/mysql) e os clients
// NoSQL (core/nosql/*): "pague só pelo que você usa".
//
// Esta é a FUNDAÇÃO: Module, Registry, Container (DI) e Lifecycle, provados
// com 15 adapters reais (ver core/adapters/) — cache, Postgres, REST,
// GraphQL, gRPC, SOAP, Kafka, RabbitMQ, NATS, S3/MinIO, OAuth2,
// SMTP/SendGrid/SES, Prometheus e OpenTelemetry. Adapters cuja dependência
// já é obrigatória do framework (ou pura stdlib) ganham sugar direto aqui
// (core.Cache, core.Database, core.API.REST/.SOAPClient, core.Auth,
// core.Mail.SMTP); o resto ativa via UseModule, importando só quem
// realmente vai usar — ver USE.md seção 28 para o catálogo completo.
package core

import (
	"context"
	"fmt"

	"kyrux/core/container"
	"kyrux/core/events"
	"kyrux/core/lifecycle"
	"kyrux/core/registry"
)

// Core orquestra o ciclo de vida de módulos plugáveis da aplicação.
type Core struct {
	// Events é o barramento de eventos existente (core/events) — aqui
	// recebe, além do que a aplicação publicar, os eventos de fase do
	// lifecycle (lifecycle.BeforeInit, AfterInit, etc.), sempre de forma
	// assíncrona e best-effort (ver core/lifecycle para a razão).
	Events *events.Bus

	// Container guarda, por nome, os valores que os módulos ativos
	// produzem (ver Use) — disponíveis para qualquer parte da aplicação
	// via container.Resolve[T](core.Container, "nome").
	Container *container.Container

	lifecycle *lifecycle.Manager

	Database DatabaseNamespace
	Cache    CacheNamespace
	API      APINamespace
	Auth     AuthNamespace
	Mail     MailNamespace
}

// New cria um Core vazio, pronto para ativar módulos.
func New() *Core {
	bus := events.NewBus()
	c := &Core{
		Events:    bus,
		Container: container.New(),
		lifecycle: lifecycle.NewManager(bus),
	}
	c.Database = DatabaseNamespace{core: c, SQL: SQLNamespace{core: c}}
	c.Cache = CacheNamespace{core: c}
	c.API = APINamespace{core: c}
	c.Auth = AuthNamespace{core: c}
	c.Mail = MailNamespace{core: c}
	return c
}

// Activate ativa mod (Init, depois Configure) sem esperar que ele produza
// um valor via Value() — para módulos cujo propósito é o efeito colateral
// (abrir um servidor, por exemplo), não algo a ser resolvido depois no
// Container. Para módulos que produzem um valor útil, use UseModule.
func (c *Core) Activate(mod registry.Module) error {
	return c.lifecycle.Add(context.Background(), mod)
}

// valuer é implementado pelos adapters que produzem um valor utilizável
// (ex: *cache.Cache, *database.DB) através do método Value(). É o mecanismo
// genérico que permite a Use extrair esse valor sem que este pacote
// precise importar o adapter concreto.
type valuer[T any] interface {
	Value() T
}

// UseModule ativa mod (Init, depois Configure — nesta ordem, parando no
// primeiro erro), registra no Container sob containerKey o valor que mod
// produz (via Value() T) e o devolve. Use para adapters que recebem
// parâmetros de construção (ex: sqlpostgres.New(name, dsn)) e por isso não
// passam pelo registro por nome (registry.Register/New) — quem os constrói
// já sabe exatamente qual módulo quer ativar.
func UseModule[T any](c *Core, mod registry.Module, containerKey string) (T, error) {
	var zero T
	if err := c.lifecycle.Add(context.Background(), mod); err != nil {
		return zero, err
	}
	v, ok := mod.(valuer[T])
	if !ok {
		return zero, fmt.Errorf("core: módulo %q não produz um valor do tipo esperado", mod.Name())
	}
	val := v.Value()
	container.Provide(c.Container, containerKey, val)
	return val, nil
}

// Use ativa o módulo registrado sob moduleName (ver registry.Register) e
// devolve o valor que ele produz. É o caminho de hot-plug puro: só funciona
// se o pacote do adapter correspondente tiver sido importado (mesmo que só
// com _) em algum lugar da aplicação — sem isso, registry.New devolve erro.
// Indicado para módulos sem parâmetros de construção (config lida de
// variáveis de ambiente dentro do próprio adapter); para módulos
// parametrizados, construa o adapter você mesmo e use UseModule.
func Use[T any](c *Core, moduleName, containerKey string) (T, error) {
	var zero T
	mod, err := registry.New(moduleName)
	if err != nil {
		return zero, err
	}
	return UseModule[T](c, mod, containerKey)
}

// Run inicia (Start) todos os módulos ativados até aqui, na ordem em que
// foram ativados.
func (c *Core) Run() error {
	return c.lifecycle.StartAll(context.Background())
}

// Shutdown encerra todos os módulos ativados, em ordem reversa à ativação.
// Tenta encerrar todos mesmo se algum falhar — devolve a lista de erros
// ocorridos (vazia se tudo correu bem).
func (c *Core) Shutdown() []error {
	return c.lifecycle.ShutdownAll(context.Background())
}

// Modules lista os módulos atualmente ativos, na ordem de ativação —
// introspecção/health checks.
func (c *Core) Modules() []registry.Module {
	return c.lifecycle.Modules()
}
