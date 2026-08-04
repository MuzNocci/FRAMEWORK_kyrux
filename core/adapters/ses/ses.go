// Package ses é o adapter que expõe um client de envio de e-mail via
// Amazon SES (github.com/aws/aws-sdk-go-v2/service/ses — o SDK oficial da
// AWS, API v1 do SES) como um Module do Core (kyrux/core). Funciona tanto
// com o SES real da AWS quanto com qualquer emulador compatível (ex:
// LocalStack) — mesmo padrão de endpoint configurável já usado por
// core/nosql/dynamodb e core/adapters/s3.
//
// Ao contrário de restapi/sqlpostgres, este pacote NÃO é importado por
// kyrux/core — o SDK da AWS é uma dependência pesada de verdade que a
// maioria dos projetos Kyrux nunca vai usar. Importe este pacote você
// mesmo, na sua aplicação, só se for enviar e-mail pelo SES — mesma
// filosofia dos clients NoSQL (core/nosql/*) e dos outros adapters.
//
// Ativação: como este adapter recebe parâmetros de construção, ele não
// passa pelo registro por nome (core/registry) — construa com New e ative
// com core.UseModule:
//
//	client, err := core.UseModule[*ses.Client](c, ses.New("principal", "us-east-1", "", "", ""), "mail.ses.principal")
//	err = client.Send(ctx, mail.Message{From: "...", To: []string{"..."}, Subject: "...", Text: "..."})
package ses

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"

	kymail "kyrux/core/mail"
)

// Client envia e-mails via Amazon SES. Implementa kymail.Sender.
type Client struct {
	raw *ses.Client
}

// Send monta e envia o e-mail via SendEmail (corpo simples —
// From/To/Cc/Bcc/Subject/Text/HTML — anexos exigem MIME cru, ver Raw() e
// SendRawEmail).
func (c *Client) Send(ctx context.Context, msg kymail.Message) error {
	body := &types.Body{}
	if msg.Text != "" {
		body.Text = &types.Content{Data: aws.String(msg.Text)}
	}
	if msg.HTML != "" {
		body.Html = &types.Content{Data: aws.String(msg.HTML)}
	}

	_, err := c.raw.SendEmail(ctx, &ses.SendEmailInput{
		Source: aws.String(msg.From),
		Destination: &types.Destination{
			ToAddresses:  msg.To,
			CcAddresses:  msg.Cc,
			BccAddresses: msg.Bcc,
		},
		Message: &types.Message{
			Subject: &types.Content{Data: aws.String(msg.Subject)},
			Body:    body,
		},
	})
	if err != nil {
		return fmt.Errorf("ses: enviar: %w", err)
	}
	return nil
}

// Raw devolve o *ses.Client nativo do SDK oficial — escape hatch para
// e-mails com anexo (SendRawEmail, MIME cru) e qualquer outra operação que
// este wrapper não cobre.
func (c *Client) Raw() *ses.Client { return c.raw }

// Adapter implementa registry.Module para um client SES nomeado.
type Adapter struct {
	name      string
	region    string
	endpoint  string
	accessKey string
	secretKey string
	client    *Client
}

// New cria (mas ainda não conecta — isso só acontece em Configure) um
// adapter SES. name identifica esta configuração entre outras do mesmo
// tipo. region é obrigatória mesmo apontando pra um endpoint não-AWS (o SDK
// exige alguma região configurada). endpoint vazio usa o SES real da AWS,
// com credenciais resolvidas pela cadeia padrão do SDK (accessKey/
// secretKey são ignorados nesse caso); um endpoint não-vazio (ex:
// LocalStack) exige accessKey/secretKey explícitos.
func New(name, region, endpoint, accessKey, secretKey string) *Adapter {
	return &Adapter{name: name, region: region, endpoint: endpoint, accessKey: accessKey, secretKey: secretKey}
}

func (a *Adapter) Name() string { return "mail.ses." + a.name }

func (a *Adapter) Init(ctx context.Context) error {
	if a.region == "" {
		return fmt.Errorf("ses: região vazia para %q", a.name)
	}
	return nil
}

// Configure resolve a config do SDK e testa a conectividade (GetSendQuota)
// antes de devolver um client que pareceria pronto mas não é.
func (a *Adapter) Configure(ctx context.Context) error {
	optFns := []func(*config.LoadOptions) error{config.WithRegion(a.region)}
	if a.endpoint != "" {
		optFns = append(optFns, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(a.accessKey, a.secretKey, ""),
		))
	}
	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return fmt.Errorf("ses: carregar config (%s): %w", a.name, err)
	}

	var sesOpts []func(*ses.Options)
	if a.endpoint != "" {
		sesOpts = append(sesOpts, func(o *ses.Options) {
			o.BaseEndpoint = aws.String(a.endpoint)
		})
	}
	raw := ses.NewFromConfig(cfg, sesOpts...)

	if _, err := raw.GetSendQuota(ctx, &ses.GetSendQuotaInput{}); err != nil {
		return fmt.Errorf("ses: conectar (%s): %w", a.name, err)
	}

	a.client = &Client{raw: raw}
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

// Shutdown não faz nada — o client HTTP subjacente não precisa de
// encerramento explícito.
func (a *Adapter) Shutdown(ctx context.Context) error { return nil }

// Value devolve o *Client já pronto (implementa kymail.Sender).
func (a *Adapter) Value() *Client { return a.client }
