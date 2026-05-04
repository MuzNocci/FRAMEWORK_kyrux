package benchmark_test

// Layer 3 — Throughput: stack completo (bootstrap + apps + templates)
//
// Mede req/s com o servidor real em modo produção.
// Requer ao menos um app instalado com rota GET /.
//
// Uso:
//   go test ./core/router/benchmark/ -run TestThroughputStack -v -count=1

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "kyrux/core/apps"
	"kyrux/core/bootstrap"
	"kyrux/core/render"
)

func TestThroughputStack(t *testing.T) {
	os.Setenv("APP_ENV", "production")

	render.AppsDir = "../../../apps"

	fw, err := bootstrap.Init("../../../.env")
	if err != nil {
		t.Fatalf("bootstrap.Init: %v", err)
	}

	render.SetDebug(false)

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

	warmupClient := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: workers * 8}}
	for i := 0; i < workers*10; i++ {
		resp, err := warmupClient.Get(base + "/")
		if err == nil {
			resp.Body.Close()
		}
	}

	const (
		duration   = 5 * time.Second
		goroutines = 8
	)
	concurrency := workers * goroutines

	type scenario struct {
		name string
		url  string
	}
	scenarios := []scenario{
		{"HTML render  GET /", base + "/"},
	}

	fmt.Printf("\n╔══════════════════════════════════════════════════════╗\n")
	fmt.Printf("║      Kyrux — Throughput (stack completo)             ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Workers (GOMAXPROCS): %-4d                          ║\n", workers)
	fmt.Printf("║  Goroutines clientes:  %-4d (%d por worker)          ║\n", concurrency, goroutines)
	fmt.Printf("║  Duração por cenário:  %-4s                          ║\n", duration)
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
					resp, err := client.Get(sc.url)
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

		fmt.Printf("  Cenário : %s\n", sc.name)
		fmt.Printf("  Total   : %d requisições\n", total.Load())
		fmt.Printf("  Erros   : %d (%.2f%%)\n", errs.Load(), errRate)
		fmt.Printf("  ► Throughput: %.0f req/s\n\n", rps)

		t.Logf("[%s] %.0f req/s | total=%d erros=%d workers=%d concurrency=%d",
			sc.name, rps, total.Load(), errs.Load(), workers, concurrency)
	}
}
