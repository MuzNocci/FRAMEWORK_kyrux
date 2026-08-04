// Package mail define o tipo de mensagem compartilhado por todos os
// adapters de envio de e-mail do Kyrux (core/adapters/smtp,
// core/adapters/sendgrid, core/adapters/ses) — montar um e-mail não muda
// ao trocar de provedor, só a ativação do adapter muda.
package mail

import "context"

// Attachment é um arquivo anexado a uma Message.
type Attachment struct {
	Filename    string
	Content     []byte
	ContentType string
}

// Message é um e-mail genérico, aceito por qualquer Sender.
type Message struct {
	From        string
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Text        string // corpo em texto puro
	HTML        string // corpo em HTML (opcional — se vazio, envia só Text)
	Attachments []Attachment
}

// Sender é implementado por qualquer adapter de envio de e-mail — o
// *Client de core/adapters/smtp, core/adapters/sendgrid e
// core/adapters/ses satisfazem esta interface.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}
