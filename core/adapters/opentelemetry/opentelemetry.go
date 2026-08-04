// Package opentelemetry é o adapter que expõe tracing distribuído
// (go.opentelemetry.io/otel/sdk — o SDK oficial, exportando via OTLP/HTTP
// para qualquer collector compatível: Jaeger, Tempo, o próprio
// OpenTelemetry Collector) como um Module do Core (kyrux/core).
//
// Ao contrário de restapi/sqlpostgres, este pacote NÃO é importado por
// kyrux/core — o SDK completo (resource, batch processor, exporter OTLP)
// é uma dependência real que só quem quer tracing distribuído precisa.
// Importe este pacote você mesmo, na sua aplicação, só se for exportar
// traces — mesma filosofia dos clients NoSQL (core/nosql/*) e dos outros
// adapters.
//
// Configure registra o TracerProvider como o global do pacote otel
// (otel.SetTracerProvider) — é assim que a maioria das bibliotecas
// instrumentadas (incluindo o próprio go-redis, gRPC, etc.) encontram o
// tracer automaticamente, sem precisar recebê-lo explicitamente. Ativar
// mais de uma vez neste processo substitui o provider global anterior.
//
// Ativação: como este adapter recebe parâmetros de construção, ele não
// passa pelo registro por nome (core/registry) — construa com New e ative
// com core.UseModule:
//
//	client, err := core.UseModule[*opentelemetry.Client](c, opentelemetry.New("principal", "minha-app", "localhost:4318", true), "observability.otel.principal")
//	tracer := client.Tracer("meu-pacote")
//	ctx, span := tracer.Start(ctx, "minha-operacao")
//	defer span.End()
package opentelemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// Client agrega o TracerProvider configurado — Tracer devolve um
// trace.Tracer pronto para abrir spans; Shutdown drena o buffer de envio
// (batch processor) e fecha o exporter, garantindo que spans pendentes
// sejam exportados antes do processo encerrar.
type Client struct {
	provider *sdktrace.TracerProvider
}

// Tracer devolve um trace.Tracer nomeado (convenção: nome do pacote que o
// usa) — tracer.Start(ctx, "operacao") abre um span.
func (c *Client) Tracer(name string) trace.Tracer { return c.provider.Tracer(name) }

// Provider devolve o *sdktrace.TracerProvider nativo — escape hatch para
// configuração avançada (samplers customizados, múltiplos processors).
func (c *Client) Provider() *sdktrace.TracerProvider { return c.provider }

// Adapter implementa registry.Module para um TracerProvider nomeado.
type Adapter struct {
	name        string
	serviceName string
	endpoint    string
	insecure    bool
	client      *Client
}

// New cria (mas ainda não conecta — isso só acontece em Configure) um
// adapter OpenTelemetry. name identifica esta instância entre outras do
// mesmo tipo. serviceName é o nome do serviço reportado nos spans
// (resource attribute service.name). endpoint é o host:porta do collector
// OTLP/HTTP (ex: "localhost:4318"); insecure desliga TLS (padrão em
// desenvolvimento local).
func New(name, serviceName, endpoint string, insecure bool) *Adapter {
	return &Adapter{name: name, serviceName: serviceName, endpoint: endpoint, insecure: insecure}
}

func (a *Adapter) Name() string { return "observability.opentelemetry." + a.name }

func (a *Adapter) Init(ctx context.Context) error {
	if a.serviceName == "" {
		return fmt.Errorf("opentelemetry: serviceName vazio para %q", a.name)
	}
	if a.endpoint == "" {
		return fmt.Errorf("opentelemetry: endpoint vazio para %q", a.name)
	}
	return nil
}

// Configure cria o exporter OTLP/HTTP e o TracerProvider, e o registra
// como provider global (otel.SetTracerProvider) — não há handshake de rede
// aqui (o exporter só conecta de fato quando o primeiro batch de spans é
// enviado), então um collector inacessível não falha em Configure, e sim
// silenciosamente (com log de erro) no primeiro envio — comportamento
// padrão do exporter OTLP oficial.
func (a *Adapter) Configure(ctx context.Context) error {
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(a.endpoint)}
	if a.insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return fmt.Errorf("opentelemetry: criar exporter (%s): %w", a.name, err)
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(a.serviceName),
	))
	if err != nil {
		return fmt.Errorf("opentelemetry: montar resource (%s): %w", a.name, err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	a.client = &Client{provider: tp}
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

// Shutdown drena o batch processor (envia spans pendentes) e fecha o
// exporter — sem isso, spans dos últimos segundos antes do encerramento
// podem se perder.
func (a *Adapter) Shutdown(ctx context.Context) error {
	if a.client != nil {
		return a.client.provider.Shutdown(ctx)
	}
	return nil
}

// Value devolve o *Client já pronto.
func (a *Adapter) Value() *Client { return a.client }
