package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func runBenchmark() error {
	now := time.Now()
	filename := fmt.Sprintf("benchmark/benchmark_%s.txt", now.Format("2006-01-02_15-04-05"))

	if err := os.MkdirAll("benchmark", 0750); err != nil {
		return fmt.Errorf("criar pasta benchmark: %w", err)
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("criar arquivo %s: %w", filename, err)
	}
	defer f.Close()

	out := io.MultiWriter(os.Stdout, f)

	fmt.Fprintf(out, "=== KYRUX BENCHMARK ===\n")
	fmt.Fprintf(out, "Data: %s\n", now.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(out, "Go:   %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	if cpu := cpuModel(); cpu != "" {
		fmt.Fprintf(out, "CPU:  %s\n", cpu)
	}
	fmt.Fprintln(out)

	layers := []struct {
		title string
		args  []string
	}{
		{
			"LAYER 1 — Microbenchmark (registro de rotas)",
			[]string{"test", "./core/router/benchmark/", "-bench=^Benchmark(Router|Handle)", "-benchmem", "-benchtime=3s", "-run=^$"},
		},
		{
			"LAYER 2 — Framework benchmark (sem TCP)",
			[]string{"test", "./core/router/benchmark/", "-bench=^Benchmark(Route|Middleware|Parallel)", "-benchmem", "-benchtime=3s", "-run=^$"},
		},
		{
			"LAYER 2 — Regressão automática",
			[]string{"test", "./core/router/benchmark/", "-run", "TestRegressionCheck", "-v", "-count=1"},
		},
		{
			"LAYER 3 — Throughput sintético: router puro",
			[]string{"test", "./core/router/benchmark/", "-run", "TestThroughputRouter", "-v", "-count=1"},
		},
		{
			"LAYER 3 — Throughput: rotas reais da aplicação",
			[]string{"test", "./core/router/benchmark/", "-run", "TestThroughputReal", "-v", "-count=1"},
		},
	}

	sep := strings.Repeat("═", 40)
	for _, layer := range layers {
		fmt.Fprintf(out, "%s\n %s\n%s\n", sep, layer.title, sep)
		cmd := exec.Command("go", layer.args...)
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(out, "[FALHOU]\n")
		}
		fmt.Fprintln(out)
	}

	fmt.Fprintf(os.Stdout, "resultado salvo em %s\n", filename)
	return nil
}

func cpuModel() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "model name") {
			if _, val, ok := strings.Cut(line, ":"); ok {
				return strings.TrimSpace(val)
			}
		}
	}
	return ""
}
