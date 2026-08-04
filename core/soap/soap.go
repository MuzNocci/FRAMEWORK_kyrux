// Package soap implementa SOAP 1.1 (envelope, fault, RPC via HTTP POST) —
// cliente e servidor — usando só encoding/xml da stdlib. Não existe uma
// biblioteca SOAP amplamente adotada e mantida no ecossistema Go (ao
// contrário de REST/GraphQL/gRPC); dado que o protocolo em si é só XML
// bem definido sobre HTTP, implementar direto contra a stdlib evita
// depender de um pacote de terceiros de qualidade incerta.
//
// Uso típico no Brasil: integração com webservices legados/governo (SEFAZ/
// NFe, INSS, etc. expõem SOAP) — tanto para consumir esses serviços
// (Client) quanto, mais raramente, para expor os seus próprios (Server).
package soap

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const envelopeNS = "http://schemas.xmlsoap.org/soap/envelope/"

// Fault representa um <soap:Fault> — a forma padrão de SOAP reportar erros.
type Fault struct {
	Code   string `xml:"faultcode"`
	String string `xml:"faultstring"`
}

func (f *Fault) Error() string {
	return fmt.Sprintf("soap fault [%s]: %s", f.Code, f.String)
}

// envelope é usado tanto para montar requisições quanto para decodificar
// respostas — Content captura o XML interno de <Body> cru (",innerxml"),
// permitindo decodificar separadamente no tipo específico da operação;
// Fault é preenchido em paralelo, se um <Fault> estiver presente.
type envelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	Body    body     `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
}

type body struct {
	Fault   *Fault `xml:"Fault"`
	Content []byte `xml:",innerxml"`
}

// Client chama operações SOAP via HTTP POST contra um endpoint fixo.
type Client struct {
	endpoint string
	http     *http.Client
}

// NewClient cria um Client SOAP para endpoint. httpClient é opcional — nil
// usa um *http.Client padrão com timeout de 30s.
func NewClient(endpoint string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{endpoint: endpoint, http: httpClient}
}

// Call serializa request (via encoding/xml) como o conteúdo de <soap:Body>,
// envia como POST com o header SOAPAction, e decodifica a resposta em
// response (ignorado se nil). Devolve um *Fault (via errors.As) se o
// servidor responder com <soap:Fault>.
func (c *Client) Call(ctx context.Context, soapAction string, request, response any) error {
	reqBody, err := xml.Marshal(request)
	if err != nil {
		return fmt.Errorf("soap: serializar requisição: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString(`<soap:Envelope xmlns:soap="` + envelopeNS + `"><soap:Body>`)
	buf.Write(reqBody)
	buf.WriteString(`</soap:Body></soap:Envelope>`)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, &buf)
	if err != nil {
		return fmt.Errorf("soap: montar requisição: %w", err)
	}
	httpReq.Header.Set("Content-Type", "text/xml; charset=utf-8")
	if soapAction != "" {
		httpReq.Header.Set("SOAPAction", `"`+soapAction+`"`)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("soap: chamar %s: %w", c.endpoint, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("soap: ler resposta: %w", err)
	}

	var env envelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("soap: decodificar envelope (status %d): %w", resp.StatusCode, err)
	}
	if env.Body.Fault != nil {
		return env.Body.Fault
	}
	if response != nil {
		if err := xml.Unmarshal(env.Body.Content, response); err != nil {
			return fmt.Errorf("soap: decodificar resposta: %w", err)
		}
	}
	return nil
}

// HandlerFunc processa uma operação SOAP — requestXML é o XML cru do
// elemento raiz dentro de <Body>; o retorno é o XML cru da resposta
// (normalmente produzido com xml.Marshal no seu próprio tipo).
type HandlerFunc func(ctx context.Context, requestXML []byte) ([]byte, error)

// Server roteia operações SOAP pelo nome do elemento raiz de <Body> — não
// é o roteamento "oficial" do SOAP (que tipicamente usa WSDL/SOAPAction),
// mas é a forma mais direta de despachar sem depender de um WSDL. Server
// implementa http.Handler — monte num *router.Router de core.API.REST via
// HandlePrefix("/soap", server), do mesmo jeito que core/adapters/prometheus.
type Server struct {
	mu       sync.RWMutex
	handlers map[string]HandlerFunc
}

// NewServer cria um Server SOAP vazio.
func NewServer() *Server {
	return &Server{handlers: make(map[string]HandlerFunc)}
}

// Handle registra h para a operação identificada pelo nome do elemento
// raiz dentro de <Body> (ex: "GetUserRequest").
func (s *Server) Handle(operation string, h HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[operation] = h
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeFault(w, "soap:Client", "erro lendo o corpo da requisição")
		return
	}

	var env envelope
	if err := xml.Unmarshal(data, &env); err != nil {
		writeFault(w, "soap:Client", "envelope XML inválido: "+err.Error())
		return
	}

	opName, err := rootElementName(env.Body.Content)
	if err != nil {
		writeFault(w, "soap:Client", "corpo SOAP vazio ou inválido")
		return
	}

	s.mu.RLock()
	handler, ok := s.handlers[opName]
	s.mu.RUnlock()
	if !ok {
		writeFault(w, "soap:Client", "operação desconhecida: "+opName)
		return
	}

	respXML, err := handler(r.Context(), env.Body.Content)
	if err != nil {
		writeFault(w, "soap:Server", err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>`))
	w.Write([]byte(`<soap:Envelope xmlns:soap="` + envelopeNS + `"><soap:Body>`))
	w.Write(respXML)
	w.Write([]byte(`</soap:Body></soap:Envelope>`))
}

// rootElementName devolve o nome local do primeiro elemento em data.
func rootElementName(data []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name.Local, nil
		}
	}
}

func writeFault(w http.ResponseWriter, code, message string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>`+
		`<soap:Envelope xmlns:soap="%s"><soap:Body><soap:Fault>`+
		`<faultcode>%s</faultcode><faultstring>%s</faultstring>`+
		`</soap:Fault></soap:Body></soap:Envelope>`,
		envelopeNS, xmlEscape(code), xmlEscape(message))
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	xml.EscapeText(&buf, []byte(s))
	return buf.String()
}
