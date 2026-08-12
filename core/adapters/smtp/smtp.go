// Package smtp é o adapter que expõe um client de envio de e-mail via SMTP
// (net/smtp + MIME manual — nenhuma dependência de terceiros) como um
// Module do Core (kyrux/core). Diferente de sendgrid/ses, não precisa de
// nenhuma conta em serviço externo — funciona com qualquer servidor SMTP
// (um relay corporativo, Gmail, Mailtrap/Mailpit em desenvolvimento).
//
// Porta 465 usa TLS implícito (SMTPS) automaticamente — o handshake TLS
// começa antes de qualquer byte do protocolo SMTP, então StartTLS não se
// aplica e é ignorado nesse caso. Qualquer outra porta segue o
// comportamento de sempre: texto plano, com STARTTLS opcional via useTLS
// (necessário na maioria dos servidores modernos, ex: porta 587).
package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"

	kymail "kyrux/core/mail"
)

// portImplicitTLS é a porta convencional do SMTPS (TLS implícito) — RFC
// 8314. Diferente de STARTTLS (porta 587, negociado depois do EHLO em
// texto plano), aqui o handshake TLS é a primeira coisa na conexão.
const portImplicitTLS = "465"

// Client envia e-mails via SMTP. Implementa kymail.Sender.
type Client struct {
	host     string
	port     string
	username string
	password string
	useTLS   bool // STARTTLS — a maioria dos servidores modernos (porta 587) exige

	// implicitTLS é decidido uma vez, na construção (port == "465"), em vez
	// de comparado a cada Send — e dá ao teste um jeito de exercitar o
	// caminho SMTPS num listener de porta efêmera em loopback, sem precisar
	// bind privilegiado na porta 465 de verdade.
	implicitTLS bool

	// tlsConfig, quando não-nil, substitui o *tls.Config construído por
	// padrão (ServerName: host) nas conexões TLS (implícita ou STARTTLS).
	// Fica nil em todo uso real — existe só para o teste conseguir confiar
	// num certificado autoassinado de um servidor SMTPS fake em loopback,
	// sem enfraquecer a verificação de certificado no caminho de produção.
	tlsConfig *tls.Config
}

func (c *Client) tlsClientConfig() *tls.Config {
	if c.tlsConfig != nil {
		return c.tlsConfig
	}
	return &tls.Config{ServerName: c.host}
}

// Send monta o e-mail (texto, HTML e anexos, se houver) e o envia via uma
// sessão SMTP nova. O contexto só cobre a etapa de conexão TCP — o
// protocolo SMTP em si (net/smtp) não é cancelável depois de iniciado.
func (c *Client) Send(ctx context.Context, msg kymail.Message) error {
	body, contentType, err := buildMIME(msg)
	if err != nil {
		return fmt.Errorf("smtp: montar mensagem: %w", err)
	}

	var headers strings.Builder
	fmt.Fprintf(&headers, "From: %s\r\n", msg.From)
	if msg.ReplyTo != "" {
		fmt.Fprintf(&headers, "Reply-To: %s\r\n", msg.ReplyTo)
	}
	fmt.Fprintf(&headers, "To: %s\r\n", strings.Join(msg.To, ", "))
	if len(msg.Cc) > 0 {
		fmt.Fprintf(&headers, "Cc: %s\r\n", strings.Join(msg.Cc, ", "))
	}
	fmt.Fprintf(&headers, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", msg.Subject))
	headers.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&headers, "Content-Type: %s\r\n\r\n", contentType)

	addr := net.JoinHostPort(c.host, c.port)

	var conn net.Conn
	if c.implicitTLS {
		// SMTPS: o TLS é a primeira coisa na conexão, antes de qualquer
		// byte SMTP — StartTLS não entra em jogo aqui.
		var d tls.Dialer
		d.Config = c.tlsClientConfig()
		conn, err = d.DialContext(ctx, "tcp", addr)
	} else {
		var d net.Dialer
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp: conectar em %s: %w", addr, err)
	}

	client, err := smtp.NewClient(conn, c.host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp: handshake: %w", err)
	}
	defer client.Close()

	if c.useTLS && !c.implicitTLS {
		if err := client.StartTLS(c.tlsClientConfig()); err != nil {
			return fmt.Errorf("smtp: starttls: %w", err)
		}
	}

	if c.username != "" {
		if ok, _ := client.Extension("AUTH"); ok {
			auth := smtp.PlainAuth("", c.username, c.password, c.host)
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp: autenticar: %w", err)
			}
		}
	}

	if err := client.Mail(msg.From); err != nil {
		return fmt.Errorf("smtp: MAIL FROM: %w", err)
	}
	recipients := append(append(append([]string{}, msg.To...), msg.Cc...), msg.Bcc...)
	for _, rcpt := range recipients {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp: RCPT TO %s: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: DATA: %w", err)
	}
	if _, err := w.Write([]byte(headers.String() + body)); err != nil {
		return fmt.Errorf("smtp: escrever corpo: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: finalizar DATA: %w", err)
	}

	return client.Quit()
}

// buildMIME monta o corpo e o Content-Type — texto puro se não houver HTML
// nem anexos; multipart/alternative se houver HTML; multipart/mixed
// envolvendo o alternative se houver anexos.
func buildMIME(msg kymail.Message) (string, string, error) {
	hasHTML := msg.HTML != ""
	hasAttachments := len(msg.Attachments) > 0

	if !hasHTML && !hasAttachments {
		return msg.Text, "text/plain; charset=UTF-8", nil
	}

	var altBuf bytes.Buffer
	altWriter := multipart.NewWriter(&altBuf)
	if msg.Text != "" {
		part, err := altWriter.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=UTF-8"}})
		if err != nil {
			return "", "", err
		}
		part.Write([]byte(msg.Text))
	}
	if hasHTML {
		part, err := altWriter.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/html; charset=UTF-8"}})
		if err != nil {
			return "", "", err
		}
		part.Write([]byte(msg.HTML))
	}
	if err := altWriter.Close(); err != nil {
		return "", "", err
	}

	if !hasAttachments {
		return altBuf.String(), "multipart/alternative; boundary=" + altWriter.Boundary(), nil
	}

	var mixedBuf bytes.Buffer
	mixedWriter := multipart.NewWriter(&mixedBuf)
	altPart, err := mixedWriter.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"multipart/alternative; boundary=" + altWriter.Boundary()},
	})
	if err != nil {
		return "", "", err
	}
	altPart.Write(altBuf.Bytes())

	for _, att := range msg.Attachments {
		header := textproto.MIMEHeader{}
		header.Set("Content-Type", att.ContentType)
		header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, att.Filename))
		header.Set("Content-Transfer-Encoding", "base64")
		part, err := mixedWriter.CreatePart(header)
		if err != nil {
			return "", "", err
		}
		encoded := base64.StdEncoding.EncodeToString(att.Content)
		for i := 0; i < len(encoded); i += 76 {
			end := min(i+76, len(encoded))
			part.Write([]byte(encoded[i:end] + "\r\n"))
		}
	}
	if err := mixedWriter.Close(); err != nil {
		return "", "", err
	}

	return mixedBuf.String(), "multipart/mixed; boundary=" + mixedWriter.Boundary(), nil
}

// Adapter implementa registry.Module para um client SMTP nomeado.
type Adapter struct {
	name     string
	host     string
	port     string
	username string
	password string
	useTLS   bool
	client   *Client
}

// New cria (mas ainda não conecta — isso só acontece em Configure) um
// adapter SMTP. name identifica esta configuração entre outras do mesmo
// tipo. useTLS ativa STARTTLS (necessário na maioria dos servidores
// modernos, ex: porta 587) — ignorado quando port é "465", que já usa TLS
// implícito (SMTPS) automaticamente, sem StartTLS.
func New(name, host, port, username, password string, useTLS bool) *Adapter {
	return &Adapter{name: name, host: host, port: port, username: username, password: password, useTLS: useTLS}
}

func (a *Adapter) Name() string { return "mail.smtp." + a.name }

func (a *Adapter) Init(ctx context.Context) error {
	if a.host == "" || a.port == "" {
		return fmt.Errorf("smtp: host/port vazios para %q", a.name)
	}
	return nil
}

// Configure testa a conectividade TCP com o servidor (sem autenticar
// ainda) antes de devolver um client que pareceria pronto mas não é.
func (a *Adapter) Configure(ctx context.Context) error {
	addr := net.JoinHostPort(a.host, a.port)
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp: conectar em %s: %w", addr, err)
	}
	conn.Close()
	a.client = &Client{
		host:        a.host,
		port:        a.port,
		username:    a.username,
		password:    a.password,
		useTLS:      a.useTLS,
		implicitTLS: a.port == portImplicitTLS,
	}
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

// Shutdown não faz nada — cada Send abre e fecha sua própria sessão SMTP.
func (a *Adapter) Shutdown(ctx context.Context) error { return nil }

// Value devolve o *Client já pronto (implementa kymail.Sender).
func (a *Adapter) Value() *Client { return a.client }
