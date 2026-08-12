package mail

import (
	"context"
	"encoding/json"
	"fmt"

	"kyrux/core/queue"
)

const queueTaskName = "kyrux.mail.send"

// Queued envolve um Sender de verdade (SMTP, SendGrid, SES...) e faz Send
// devolver o controle quase na hora — a entrega em si acontece depois, num
// worker de fw.Queue (pool de workers, retry com backoff automático em
// falha transitória, drenagem no shutdown). Existe pra uma requisição HTTP
// não ficar presa esperando a ida e volta de uma sessão SMTP, que pode
// levar segundos — ver core/queue pro racional completo (fila vs. EventBus
// vs. Cache).
//
// Se q for nil (QUEUE_ENABLED=false), Queued cai para envio síncrono
// direto no Sender de baixo — Send nunca falha silenciosamente só porque a
// fila não existe.
type Queued struct {
	sender Sender
	queue  *queue.Queue
}

// NewQueued registra o handler de envio em q (se não-nil) e devolve um
// Sender que enfileira em vez de bloquear. Chame uma única vez — Register
// não é seguro chamar de novo depois que a fila já está processando
// tarefas concorrentemente com o mesmo nome.
func NewQueued(sender Sender, q *queue.Queue) *Queued {
	qd := &Queued{sender: sender, queue: q}
	if q != nil {
		q.Register(queueTaskName, qd.handle)
	}
	return qd
}

// Send enfileira msg pra envio em background. O erro devolvido é só de
// ENFILEIRAR (fila cheia, fila encerrada) — não de entrega: falhas de SMTP
// acontecem depois, assíncronas, aparecem nos logs (queue: tarefa...) e no
// retry automático da fila, nunca de volta pro caller. Sem fila
// configurada (q nil), Send é síncrono e o erro devolvido já é o de
// entrega de verdade.
func (qd *Queued) Send(ctx context.Context, msg Message) error {
	if qd.queue == nil {
		return qd.sender.Send(ctx, msg)
	}
	return qd.queue.Enqueue(queueTaskName, msg)
}

// handle é o queue.Handler registrado na fila — roda no worker, não na
// goroutine que chamou Send.
func (qd *Queued) handle(payload any) error {
	msg, err := decodeMessage(payload)
	if err != nil {
		return err
	}
	return qd.sender.Send(context.Background(), msg)
}

// decodeMessage normaliza payload pro tipo Message. Em fila de memória o
// valor original chega intacto (type assertion direta); em fila Redis
// chega como map[string]any (serializado em JSON pelo core/queue), daí o
// round-trip via encoding/json pra reconstruir o struct — inclusive
// Attachments, cujo []byte vira base64 no JSON e volta corretamente na
// decodificação.
func decodeMessage(payload any) (Message, error) {
	if msg, ok := payload.(Message); ok {
		return msg, nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return Message{}, fmt.Errorf("mail: payload da fila não serializável: %w", err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return Message{}, fmt.Errorf("mail: decodificar payload da fila: %w", err)
	}
	return msg, nil
}
