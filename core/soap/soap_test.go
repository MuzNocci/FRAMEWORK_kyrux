package soap

import (
	"context"
	"encoding/xml"
	"net/http/httptest"
	"testing"
)

type pingRequest struct {
	XMLName xml.Name `xml:"PingRequest"`
	Valor   string   `xml:"Valor"`
}

type pingResponse struct {
	XMLName xml.Name `xml:"PingResponse"`
	Eco     string   `xml:"Eco"`
}

func newTestServer(t *testing.T) (*httptest.Server, *Client) {
	t.Helper()
	server := NewServer()
	server.Handle("PingRequest", func(ctx context.Context, requestXML []byte) ([]byte, error) {
		var req pingRequest
		if err := xml.Unmarshal(requestXML, &req); err != nil {
			return nil, err
		}
		return xml.Marshal(pingResponse{Eco: req.Valor})
	})
	httpSrv := httptest.NewServer(server)
	t.Cleanup(httpSrv.Close)
	return httpSrv, NewClient(httpSrv.URL, nil)
}

func TestClientEServerRoundTrip(t *testing.T) {
	_, client := newTestServer(t)

	var resp pingResponse
	if err := client.Call(context.Background(), "Ping", pingRequest{Valor: "olá"}, &resp); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Eco != "olá" {
		t.Errorf("esperava eco %q, recebeu %q", "olá", resp.Eco)
	}
}

func TestServerOperacaoDesconhecidaDevolveFault(t *testing.T) {
	_, client := newTestServer(t)

	type outraRequest struct {
		XMLName xml.Name `xml:"OutraRequest"`
	}
	err := client.Call(context.Background(), "Outra", outraRequest{}, nil)
	if err == nil {
		t.Fatal("esperava erro para operação desconhecida")
	}
	fault, ok := err.(*Fault)
	if !ok {
		t.Fatalf("esperava *Fault, recebeu %T: %v", err, err)
	}
	if fault.Code != "soap:Client" {
		t.Errorf("esperava faultcode soap:Client, recebeu %q", fault.Code)
	}
}

func TestServerHandlerComErroDevolveFault(t *testing.T) {
	server := NewServer()
	server.Handle("PingRequest", func(ctx context.Context, requestXML []byte) ([]byte, error) {
		return nil, errBoom
	})
	httpSrv := httptest.NewServer(server)
	defer httpSrv.Close()
	client := NewClient(httpSrv.URL, nil)

	err := client.Call(context.Background(), "Ping", pingRequest{Valor: "x"}, nil)
	if err == nil {
		t.Fatal("esperava erro")
	}
	fault, ok := err.(*Fault)
	if !ok {
		t.Fatalf("esperava *Fault, recebeu %T: %v", err, err)
	}
	if fault.Code != "soap:Server" {
		t.Errorf("esperava faultcode soap:Server, recebeu %q", fault.Code)
	}
}

var errBoom = &Fault{Code: "boom", String: "erro proposital do handler"}

func TestRootElementNameExtraiNomeDoElementoRaiz(t *testing.T) {
	name, err := rootElementName([]byte(`<PingRequest><Valor>x</Valor></PingRequest>`))
	if err != nil {
		t.Fatalf("rootElementName: %v", err)
	}
	if name != "PingRequest" {
		t.Errorf("esperava %q, recebeu %q", "PingRequest", name)
	}
}

func TestRootElementNameXMLInvalidoDevolveErro(t *testing.T) {
	if _, err := rootElementName([]byte(`não é xml`)); err == nil {
		t.Error("esperava erro para XML inválido")
	}
}
