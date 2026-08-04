package core_test

// Prova de ponta a ponta do adapter Prometheus: registra um contador de
// verdade, monta o handler /metrics no core.API.REST e faz uma requisição
// HTTP real, verificando que o formato de texto do Prometheus contém a
// métrica com o valor certo.

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"

	"kyrux/core"
	prometheusadapter "kyrux/core/adapters/prometheus"
	"kyrux/core/router"
)

func TestCorePrometheusExpoeMetricasReais(t *testing.T) {
	addr := envOr("KYRUX_TEST_PROMETHEUS_ADDR", "127.0.0.1:18093")

	c := core.New()

	metrics, err := core.UseModule[*prometheusadapter.Client](c, prometheusadapter.New("principal"), "observability.prometheus.principal")
	if err != nil {
		t.Fatalf("UseModule: %v", err)
	}

	requestsTotal := prom.NewCounter(prom.CounterOpts{
		Name: "kyrux_teste_requests_total",
		Help: "contador de teste do kyrux",
	})
	metrics.MustRegister(requestsTotal)
	requestsTotal.Add(3)

	api, err := c.API.REST(addr)
	if err != nil {
		t.Fatalf("API.REST: %v", err)
	}
	api.HandlePrefix("/metrics", metrics.Handler())
	api.Handle("GET /ping", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})

	if err := c.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer c.Shutdown()

	var resp *http.Response
	for i := 0; i < 20; i++ {
		resp, err = http.Get("http://" + addr + "/metrics")
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	text := string(body)

	if !strings.Contains(text, "kyrux_teste_requests_total 3") {
		t.Errorf("esperava a métrica com valor 3 no corpo, recebeu:\n%s", text)
	}
	if !strings.Contains(text, "go_goroutines") {
		t.Error("esperava os coletores padrão do processo Go (go_goroutines) no corpo")
	}
}
