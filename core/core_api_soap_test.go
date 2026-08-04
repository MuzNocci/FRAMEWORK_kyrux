package core_test

// Prova de ponta a ponta do SOAP: sobe um servidor SOAP de verdade
// (core/soap.Server, montado no *router.Router de core.API.REST via
// HandlePrefix — mesmo padrão do core/adapters/prometheus), com uma
// operação real de soma, e chama contra ele com o client ativado via
// core.API.SOAPClient — round-trip HTTP real, incluindo o caminho de erro
// (soap:Fault).

import (
	"context"
	"encoding/xml"
	"fmt"
	"net"
	"testing"
	"time"

	"kyrux/core"
	"kyrux/core/soap"
)

type somarRequest struct {
	XMLName xml.Name `xml:"SomarRequest"`
	A       int      `xml:"A"`
	B       int      `xml:"B"`
}

type somarResponse struct {
	XMLName   xml.Name `xml:"SomarResponse"`
	Resultado int      `xml:"Resultado"`
}

func TestCoreAPISOAPClienteEServidorReais(t *testing.T) {
	addr := envOr("KYRUX_TEST_REST_ADDR_SOAP", "127.0.0.1:18094")

	server := soap.NewServer()
	server.Handle("SomarRequest", func(ctx context.Context, requestXML []byte) ([]byte, error) {
		var req somarRequest
		if err := xml.Unmarshal(requestXML, &req); err != nil {
			return nil, fmt.Errorf("requisição inválida: %w", err)
		}
		if req.B == 0 {
			return nil, fmt.Errorf("divisão por zero não faz sentido aqui, mas B=0 é proibido pra este teste")
		}
		return xml.Marshal(somarResponse{Resultado: req.A + req.B})
	})

	c := core.New()
	api, err := c.API.REST(addr)
	if err != nil {
		t.Fatalf("API.REST: %v", err)
	}
	api.HandlePrefix("/soap", server)

	client, err := c.API.SOAPClient("principal", "http://"+addr+"/soap")
	if err != nil {
		t.Fatalf("API.SOAPClient: %v", err)
	}

	if err := c.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer c.Shutdown()

	var resp somarResponse
	waitForServer(t, addr)
	if err := client.Call(context.Background(), "Somar", somarRequest{A: 4, B: 5}, &resp); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Resultado != 9 {
		t.Errorf("esperava 9, recebeu %d", resp.Resultado)
	}

	// Caminho de erro: o handler devolve erro -> servidor responde com
	// soap:Fault -> client decodifica como *soap.Fault.
	err = client.Call(context.Background(), "Somar", somarRequest{A: 1, B: 0}, &resp)
	if err == nil {
		t.Fatal("esperava erro (soap:Fault) para B=0")
	}
	var fault *soap.Fault
	if !isFault(err, &fault) {
		t.Fatalf("esperava um *soap.Fault, recebeu %T: %v", err, err)
	}
}

func isFault(err error, target **soap.Fault) bool {
	f, ok := err.(*soap.Fault)
	if ok {
		*target = f
	}
	return ok
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()
	for i := 0; i < 20; i++ {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("servidor em %s não respondeu a tempo", addr)
}
