// Package sendgrid é o adapter que expõe um client de envio de e-mail via
// SendGrid (github.com/sendgrid/sendgrid-go — o SDK oficial) como um
// Module do Core (kyrux/core).
//
// Ao contrário de restapi/sqlpostgres, este pacote NÃO é importado por
// kyrux/core — o SDK do SendGrid é uma dependência real que só quem usa
// SendGrid precisa. Importe este pacote você mesmo, na sua aplicação, só
// se for enviar e-mail pelo SendGrid — mesma filosofia dos clients NoSQL
// (core/nosql/*) e dos outros adapters.
//
// Ativação: como este adapter recebe parâmetros de construção (API key),
// ele não passa pelo registro por nome (core/registry) — construa com New
// e ative com core.UseModule:
//
//	client, err := core.UseModule[*sendgrid.Client](c, sendgrid.New("SG.xxx"), "mail.sendgrid.principal")
//	err = client.Send(ctx, mail.Message{From: "...", To: []string{"..."}, Subject: "...", Text: "..."})
package sendgrid

import (
	"context"
	"encoding/base64"
	"fmt"

	sgrest "github.com/sendgrid/rest"
	sgo "github.com/sendgrid/sendgrid-go"
	sgmail "github.com/sendgrid/sendgrid-go/helpers/mail"

	kymail "kyrux/core/mail"
)

const defaultHost = "https://api.sendgrid.com"

// Client envia e-mails via a API do SendGrid. Implementa kymail.Sender.
type Client struct {
	sg *sgo.Client
}

// Send monta e envia o e-mail via POST /v3/mail/send.
func (c *Client) Send(ctx context.Context, msg kymail.Message) error {
	resp, err := c.sg.SendWithContext(ctx, buildSGMail(msg))
	if err != nil {
		return fmt.Errorf("sendgrid: enviar: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("sendgrid: resposta %d: %s", resp.StatusCode, resp.Body)
	}
	return nil
}

func buildSGMail(msg kymail.Message) *sgmail.SGMailV3 {
	m := sgmail.NewV3Mail()
	m.SetFrom(sgmail.NewEmail("", msg.From))
	m.Subject = msg.Subject

	p := sgmail.NewPersonalization()
	for _, to := range msg.To {
		p.AddTos(sgmail.NewEmail("", to))
	}
	for _, cc := range msg.Cc {
		p.AddCCs(sgmail.NewEmail("", cc))
	}
	for _, bcc := range msg.Bcc {
		p.AddBCCs(sgmail.NewEmail("", bcc))
	}
	m.AddPersonalizations(p)

	if msg.Text != "" {
		m.AddContent(sgmail.NewContent("text/plain", msg.Text))
	}
	if msg.HTML != "" {
		m.AddContent(sgmail.NewContent("text/html", msg.HTML))
	}
	for _, att := range msg.Attachments {
		a := sgmail.NewAttachment()
		a.SetContent(base64.StdEncoding.EncodeToString(att.Content))
		a.SetType(att.ContentType)
		a.SetFilename(att.Filename)
		a.SetDisposition("attachment")
		m.AddAttachment(a)
	}
	return m
}

// Adapter implementa registry.Module para um client SendGrid nomeado.
type Adapter struct {
	name   string
	apiKey string
	host   string // sobrescrito só em teste — vazio usa a API real do SendGrid
	client *Client
}

// New cria um adapter SendGrid. name identifica esta configuração entre
// outras do mesmo tipo.
func New(name, apiKey string) *Adapter {
	return &Adapter{name: name, apiKey: apiKey}
}

// newWithHost é usado nos testes deste pacote para apontar pra um servidor
// HTTP de mentirinha em vez da API real do SendGrid.
func newWithHost(name, apiKey, host string) *Adapter {
	return &Adapter{name: name, apiKey: apiKey, host: host}
}

func (a *Adapter) Name() string { return "mail.sendgrid." + a.name }

func (a *Adapter) Init(ctx context.Context) error {
	if a.apiKey == "" {
		return fmt.Errorf("sendgrid: api key vazia para %q", a.name)
	}
	return nil
}

// Configure monta o client — não há I/O aqui; uma api key inválida só se
// manifesta no primeiro Send (a API do SendGrid não tem um endpoint de
// "ping" dedicado que valeria a pena chamar aqui).
func (a *Adapter) Configure(ctx context.Context) error {
	host := a.host
	if host == "" {
		host = defaultHost
	}
	request := sgo.GetRequest(a.apiKey, "/v3/mail/send", host)
	request.Method = sgrest.Post
	a.client = &Client{sg: &sgo.Client{Request: request}}
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

// Shutdown não faz nada — cada Send é uma chamada HTTP independente.
func (a *Adapter) Shutdown(ctx context.Context) error { return nil }

// Value devolve o *Client já pronto (implementa kymail.Sender).
func (a *Adapter) Value() *Client { return a.client }
