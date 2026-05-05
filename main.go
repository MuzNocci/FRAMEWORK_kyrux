package main

import (
	"fmt"
	"kyrux/core/bootstrap"
	"kyrux/core/cli"
	"kyrux/core/environment"
	"log"
	"os"
	"os/exec"

	_ "kyrux/core/apps"
	_ "github.com/lib/pq"
)

func main() {
	if len(os.Args) > 1 {
		cli.Run(os.Args[1:])
		return
	}

	_ = environment.Load(".env")

	if environment.GetOr("APP_ENV", "production") == "development" && os.Getenv("KYRUX_INSIDE_AIR") == "" {
		startWithAir()
		return
	}

	fw, err := bootstrap.Init(".env")
	if err != nil {
		log.Fatal(err)
	}
	if err := fw.Run(); err != nil {
		log.Fatal(err)
	}
}

func airPath() string {
	if p, err := exec.LookPath("air"); err == nil {
		return p
	}
	for _, dir := range []string{
		os.Getenv("GOPATH") + "/bin",
		os.Getenv("HOME") + "/go/bin",
	} {
		p := dir + "/air"
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func startWithAir() {
	air := airPath()
	if air == "" {
		fmt.Fprintln(os.Stderr, "Air não encontrado. Instale com:")
		fmt.Fprintln(os.Stderr, "  go install github.com/air-verse/air@latest")
		os.Exit(1)
	}
	os.Setenv("KYRUX_INSIDE_AIR", "1")
	fmt.Println("Kyrux: modo desenvolvimento, iniciando com Air...")
	cmd := exec.Command(air, "-c", "core/air/.air.toml")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "erro ao iniciar Air: %v\n", err)
		os.Exit(1)
	}
}
