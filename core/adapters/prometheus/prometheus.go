// Package prometheus é o adapter que expõe métricas no formato Prometheus
// (github.com/prometheus/client_golang — o client oficial) como um Module
// do Core (kyrux/core).
//
// Ao contrário de restapi/sqlpostgres, este pacote NÃO é importado por
// kyrux/core — client_golang é uma dependência real que só quem quer
// métricas Prometheus precisa. Importe este pacote você mesmo, na sua
// aplicação, só se for expor /metrics — mesma filosofia dos clients NoSQL
// (core/nosql/*) e dos outros adapters.
//
// O Registry é isolado (prometheus.NewRegistry(), não o
// DefaultRegisterer global) — múltiplas instâncias deste adapter no mesmo
// processo (ex: em testes) não colidem registrando a mesma métrica duas
// vezes no registry global.
//
// Ativação: como este adapter recebe parâmetros de construção (nome
// lógico), ele não passa pelo registro por nome (core/registry) — construa
// com New e ative com core.UseModule:
//
//	metrics, err := core.UseModule[*prometheus.Client](c, prometheusadapter.New("principal"), "observability.prometheus.principal")
//	requestsTotal := prom.NewCounter(prom.CounterOpts{Name: "http_requests_total"})
//	metrics.MustRegister(requestsTotal)
//	api, _ := c.API.REST(addr)
//	api.HandlePrefix("/metrics", metrics.Handler())
package prometheus

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Client agrega um Registry Prometheus isolado — Register/MustRegister
// registram coletores nele; Handler devolve o http.Handler que serve
// /metrics no formato de texto padrão do Prometheus.
type Client struct {
	registry *prometheus.Registry
}

// Registry devolve o *prometheus.Registry nativo — escape hatch para
// qualquer uso avançado (Gatherer customizado, etc.) que este wrapper não
// cobre.
func (c *Client) Registry() *prometheus.Registry { return c.registry }

// Register registra collector no Registry deste Client. Erro se um
// coletor com o mesmo nome/labels já estiver registrado.
func (c *Client) Register(collector prometheus.Collector) error {
	return c.registry.Register(collector)
}

// MustRegister é como Register, mas entra em pânico em erro — um erro de
// registro na inicialização é sempre um bug de programação (nome de
// métrica duplicado), não uma condição de runtime esperada.
func (c *Client) MustRegister(collectors ...prometheus.Collector) {
	c.registry.MustRegister(collectors...)
}

// Handler devolve o http.Handler que serve as métricas no formato de texto
// do Prometheus — monte em qualquer rota (ex: api.HandlePrefix("/metrics", ...)
// no *router.Router de core.API.REST).
func (c *Client) Handler() http.Handler {
	return promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{})
}

// Adapter implementa registry.Module para um Registry Prometheus nomeado.
type Adapter struct {
	name   string
	client *Client
}

// New cria um adapter Prometheus. name identifica esta instância entre
// outras do mesmo tipo (só usado em Name()/logs).
func New(name string) *Adapter {
	return &Adapter{name: name}
}

func (a *Adapter) Name() string { return "observability.prometheus." + a.name }

func (a *Adapter) Init(ctx context.Context) error { return nil }

// Configure cria o Registry isolado e já registra os coletores padrão do
// processo Go (goroutines, GC, memória) — o mesmo conjunto que o
// DefaultRegisterer do client_golang expõe por padrão.
func (a *Adapter) Configure(ctx context.Context) error {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	a.client = &Client{registry: reg}
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

// Shutdown não faz nada — o Registry não precisa de encerramento explícito.
func (a *Adapter) Shutdown(ctx context.Context) error { return nil }

// Value devolve o *Client já pronto.
func (a *Adapter) Value() *Client { return a.client }
