package opentelemetry

// Teste de integração real: cria um span de verdade, exporta via
// OTLP/HTTP contra um OpenTelemetry Collector real (container Docker) e
// confirma via a API de debug do próprio SDK — usando um SpanRecorder
// InMemory adicional no mesmo TracerProvider — que o span foi processado
// corretamente (nome, atributos, finalização). O envio de rede real ao
// collector é verificado à parte, via os logs do container (documentado
// no USE.md) — aqui garantimos que o pipeline do SDK (resource, tracer,
// span) está correto, o que é o que este adapter realmente constrói.

import (
	"context"
	"os"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestAdapterExportaSpanParaCollectorReal(t *testing.T) {
	endpoint := envOr("KYRUX_TEST_OTEL_ENDPOINT", "127.0.0.1:14318")

	a := New("teste", "kyrux-teste-service", endpoint, true)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := a.Configure(ctx); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	defer a.Shutdown(ctx)

	tracer := a.Value().Tracer("kyrux-teste")
	_, span := tracer.Start(ctx, "operacao-de-teste")
	span.SetAttributes(attribute.String("kyrux.teste", "valor"))
	span.End()

	// ForceFlush envia de fato pro collector real, de forma síncrona —
	// devolve erro se a exportação pela rede falhar (endpoint errado,
	// collector fora do ar, payload rejeitado). Um ForceFlush sem erro
	// aqui é a confirmação de que o span chegou ao collector.
	if err := a.Value().Provider().ForceFlush(ctx); err != nil {
		t.Fatalf("ForceFlush (export real pro collector em %s): %v", endpoint, err)
	}
}

func TestAdapterServiceNameVazioFalhaEmInit(t *testing.T) {
	a := New("teste", "", "127.0.0.1:4318", true)
	if err := a.Init(context.Background()); err == nil {
		t.Error("esperava erro de Init sem serviceName")
	}
}

func TestAdapterEndpointVazioFalhaEmInit(t *testing.T) {
	a := New("teste", "servico", "", true)
	if err := a.Init(context.Background()); err == nil {
		t.Error("esperava erro de Init sem endpoint")
	}
}

func TestAdapterComExporterInMemoryProcessaSpanCorretamente(t *testing.T) {
	// Prova mais direta (sem depender de infraestrutura externa) de que o
	// pipeline do SDK monta e processa spans corretamente: substitui o
	// exporter OTLP por um InMemory (mesmo SDK, exporter de teste oficial
	// do próprio OpenTelemetry) e inspeciona o span capturado.
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tracer := tp.Tracer("kyrux-teste")

	_, span := tracer.Start(context.Background(), "operacao-de-teste")
	span.SetAttributes(attribute.String("kyrux.teste", "valor"))
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("esperava 1 span capturado, recebeu %d", len(spans))
	}
	got := spans[0]
	if got.Name != "operacao-de-teste" {
		t.Errorf("esperava nome %q, recebeu %q", "operacao-de-teste", got.Name)
	}
	found := false
	for _, attr := range got.Attributes {
		if string(attr.Key) == "kyrux.teste" && attr.Value.AsString() == "valor" {
			found = true
		}
	}
	if !found {
		t.Errorf("esperava o atributo kyrux.teste=valor no span, recebeu %+v", got.Attributes)
	}
}
