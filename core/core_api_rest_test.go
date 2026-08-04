package core_test

// Prova de conceito de ponta a ponta do core.API.REST: registra uma rota de
// verdade no *router.Router devolvido pela ativação, sobe o servidor via
// Core.Run() e faz uma requisição HTTP real contra ele.

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"kyrux/core"
	"kyrux/core/router"
)

func TestCoreAPIRESTRequisicaoReal(t *testing.T) {
	addr := envOr("KYRUX_TEST_REST_ADDR", "127.0.0.1:18080")

	c := core.New()

	r, err := c.API.REST(addr)
	if err != nil {
		t.Fatalf("API.REST: %v", err)
	}
	r.Handle("GET /ping", func(ctx *router.Context) {
		ctx.JSON(http.StatusOK, map[string]string{"pong": "kyrux"})
	})

	if err := c.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer c.Shutdown()

	// O Start abre a porta de forma síncrona (net.Listen antes de retornar),
	// mas Serve roda em goroutine própria — uma pequena espera evita uma
	// corrida rara entre a goroutine subir e o client conectar.
	var resp *http.Response
	for i := 0; i < 20; i++ {
		resp, err = http.Get("http://" + addr + "/ping")
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["pong"] != "kyrux" {
		t.Errorf("esperava pong=kyrux, recebeu %+v", body)
	}
}

func TestCoreAPIRESTEnderecoJaEmUsoFalha(t *testing.T) {
	addr := envOr("KYRUX_TEST_REST_ADDR_CONFLITO", "127.0.0.1:18081")

	c1 := core.New()
	if _, err := c1.API.REST(addr); err != nil {
		t.Fatalf("primeira ativação: %v", err)
	}
	if err := c1.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer c1.Shutdown()

	c2 := core.New()
	if _, err := c2.API.REST(addr); err != nil {
		t.Fatalf("segunda ativação (Configure) não deveria falhar: %v", err)
	}
	if err := c2.Run(); err == nil {
		t.Error("esperava erro ao subir um segundo servidor na mesma porta já em uso")
	}
}
