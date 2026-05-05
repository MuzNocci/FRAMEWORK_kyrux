package benchmark_test

// Layer 3 — Throughput: rotas reais da aplicação
//
// Usa bootstrap.Init() para carregar o app completo (DB, templates, middlewares)
// e descobre todas as rotas GET via fw.Router.Routes(). Cada rota é testada
// individualmente via TCP real durante 5 segundos.
//
// Rotas com parâmetros de path recebem valores de exemplo definidos em
// testParamByName e testParamByType. Ajuste esses mapas se alguma rota
// precisar de um ID ou slug específico que exista no banco.
//
// Uso:
//
//	go test ./core/router/benchmark/ -run TestThroughputReal -v -count=1

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/lib/pq"
	_ "kyrux/core/apps"
	"kyrux/core/bootstrap"
	"kyrux/core/render"
)

// testParamByName substitui parâmetros pelo nome da variável.
// Tem prioridade sobre testParamByType.
var testParamByName = map[string]string{
	// Exemplo: "id": "1", "slug": "meu-post"
}

// testParamByType substitui parâmetros pelo tipo detectado.
// Usado quando o nome não está em testParamByName.
var testParamByType = map[string]string{
	"path": "example",
}

// reDisplayParam captura <nome> e <nome:tipo> retornados por displayPath.
var reDisplayParam = regexp.MustCompile(`<([a-zA-Z_][a-zA-Z0-9_]*)(?::([a-zA-Z]+))?>`)

// resolveTestURL substitui parâmetros de path por valores de exemplo concretos.
// Ex: /posts/<slug>/ → /posts/example/
//
//	/files/<arquivo:path>/ → /files/example/
func resolveTestURL(path string) string {
	return reDisplayParam.ReplaceAllStringFunc(path, func(m string) string {
		sub := reDisplayParam.FindStringSubmatch(m)
		name, typ := sub[1], sub[2]
		if v, ok := testParamByName[name]; ok {
			return v
		}
		if v, ok := testParamByType[typ]; ok {
			return v
		}
		return "1"
	})
}

func TestThroughputReal(t *testing.T) {
	render.AppsDir = "../../../apps"

	// Força production antes de environment.Load() — o package respeita vars já definidas.
	// Valores fixos garantem que as verificações de segurança do bootstrap passam.
	os.Setenv("APP_ENV", "production")
	os.Setenv("SECRET_KEY", "kyrux-benchmark-secret-key-placeholder-32")
	os.Setenv("PASSWORD_PEPPER", "kyrux-benchmark-pepper-placeholder-32ch")

	fw, err := bootstrap.Init("../../../.env")
	if err != nil {
		t.Fatalf("bootstrap.Init: %v", err)
	}

	type scenario struct {
		pattern string
		url     string
	}

	var scenarios []scenario
	for _, r := range fw.Router.Routes() {
		if r.Method != "GET" {
			continue
		}
		scenarios = append(scenarios, scenario{
			pattern: r.Path,
			url:     resolveTestURL(r.Path),
		})
	}

	if len(scenarios) == 0 {
		t.Skip("nenhuma rota GET registrada na aplicação")
	}

	workers := fw.Settings.Server.Workers
	prev := runtime.GOMAXPROCS(workers)
	defer runtime.GOMAXPROCS(prev)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{
		Handler:      fw.Router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()

	base := "http://" + ln.Addr().String()

	const (
		duration   = 5 * time.Second
		goroutines = 8
	)
	concurrency := workers * goroutines

	warmupClient := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: workers * 8}}
	for _, sc := range scenarios {
		for i := 0; i < workers*5; i++ {
			resp, err := warmupClient.Get(base + sc.url)
			if err == nil {
				resp.Body.Close()
			}
		}
	}

	fmt.Printf("\n╔══════════════════════════════════════════════════════╗\n")
	fmt.Printf("║    Kyrux — Throughput (rotas reais da aplicação)     ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Workers (SERVER_WORKERS): %-4d                      ║\n", workers)
	fmt.Printf("║  Goroutines clientes:      %-4d (%d por worker)      ║\n", concurrency, goroutines)
	fmt.Printf("║  Duração por cenário:  %-4s                          ║\n", duration)
	fmt.Printf("║  Rotas testadas:       %-4d                          ║\n", len(scenarios))
	fmt.Printf("╚══════════════════════════════════════════════════════╝\n\n")

	for _, sc := range scenarios {
		var total, errs atomic.Int64

		transport := &http.Transport{
			MaxIdleConns:        concurrency,
			MaxIdleConnsPerHost: concurrency,
			IdleConnTimeout:     30 * time.Second,
		}
		client := &http.Client{Transport: transport}

		deadline := time.Now().Add(duration)
		var wg sync.WaitGroup

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for time.Now().Before(deadline) {
					resp, err := client.Get(base + sc.url)
					if err != nil {
						errs.Add(1)
						continue
					}
					resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						total.Add(1)
					} else {
						errs.Add(1)
					}
				}
			}()
		}

		wg.Wait()

		rps := float64(total.Load()) / duration.Seconds()
		errRate := 0.0
		if sum := total.Load() + errs.Load(); sum > 0 {
			errRate = float64(errs.Load()) / float64(sum) * 100
		}

		fmt.Printf("  Rota    : GET %s\n", sc.pattern)
		if strings.Contains(sc.pattern, "<") {
			fmt.Printf("  URL     : GET %s\n", sc.url)
		}
		fmt.Printf("  Total   : %d requisições\n", total.Load())
		fmt.Printf("  Erros   : %d (%.2f%%)\n", errs.Load(), errRate)
		fmt.Printf("  ► Throughput: %.0f req/s\n\n", rps)

		t.Logf("[GET %s] %.0f req/s | total=%d erros=%d workers=%d concurrency=%d",
			sc.pattern, rps, total.Load(), errs.Load(), workers, concurrency)
	}
}
