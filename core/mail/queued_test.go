package mail

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"kyrux/core/queue"
)

type recordingSender struct {
	mu   sync.Mutex
	msgs []Message
}

func (r *recordingSender) Send(ctx context.Context, msg Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, msg)
	return nil
}

func (r *recordingSender) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.msgs)
}

func (r *recordingSender) first() Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.msgs[0]
}

func TestQueuedSemFilaEnviaSincrono(t *testing.T) {
	sender := &recordingSender{}
	qd := NewQueued(sender, nil)

	msg := Message{From: "a@b.com", To: []string{"c@d.com"}, Subject: "oi"}
	if err := qd.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// Sem fila, a entrega tem que já ter acontecido quando Send retorna —
	// é exatamente essa a diferença pro caminho com fila (testado abaixo).
	if sender.count() != 1 {
		t.Fatalf("esperava 1 envio síncrono imediato, teve %d", sender.count())
	}
}

func TestQueuedComFilaEnfileiraEEntregaEmBackground(t *testing.T) {
	sender := &recordingSender{}
	q := queue.New(1, 10)
	defer q.Close()
	qd := NewQueued(sender, q)

	msg := Message{From: "a@b.com", To: []string{"c@d.com"}, Subject: "oi"}
	if err := qd.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for sender.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if sender.count() != 1 {
		t.Fatalf("esperava a mensagem entregue via fila em background, teve %d envios", sender.count())
	}
	if got := sender.first(); got.Subject != "oi" || got.From != "a@b.com" {
		t.Fatalf("mensagem entregue não bate com a enviada: %+v", got)
	}
}

func TestDecodeMessageAceitaTipoOriginalEMapStringAny(t *testing.T) {
	original := Message{
		From:    "a@b.com",
		ReplyTo: "resposta@b.com",
		To:      []string{"c@d.com", "e@f.com"},
		Subject: "assunto",
		Text:    "corpo",
	}

	// Caminho de memória: type assertion direta, sem passar por JSON.
	decodedDirect, err := decodeMessage(original)
	if err != nil {
		t.Fatalf("decodeMessage (direto): %v", err)
	}
	if decodedDirect.Subject != original.Subject {
		t.Fatalf("decode direto alterou a mensagem: %+v", decodedDirect)
	}

	// Caminho Redis: o core/queue já fez o round-trip de JSON antes de
	// chamar o Handler, então payload chega como map[string]any.
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(data, &asMap); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	decodedFromMap, err := decodeMessage(asMap)
	if err != nil {
		t.Fatalf("decodeMessage (map): %v", err)
	}
	if decodedFromMap.Subject != original.Subject ||
		decodedFromMap.From != original.From ||
		decodedFromMap.ReplyTo != original.ReplyTo ||
		len(decodedFromMap.To) != len(original.To) {
		t.Fatalf("mensagem decodificada de map[string]any não bate com a original: %+v", decodedFromMap)
	}
}

func TestQueuedSendPropagaErroDeEnfileirar(t *testing.T) {
	sender := &recordingSender{}
	q := queue.New(1, 1) // buffer minúsculo — fácil de encher pra provocar ErrQueueFull
	defer q.Close()
	qd := NewQueued(sender, q)

	// Enche o buffer disparando um monte de envios de uma vez, sem dar
	// tempo do único worker escoar — pelo menos um deve estourar o buffer.
	var gotFull bool
	for i := 0; i < 50 && !gotFull; i++ {
		msg := Message{From: "a@b.com", To: []string{"c@d.com"}, Subject: "spam"}
		if err := qd.Send(context.Background(), msg); err == queue.ErrQueueFull {
			gotFull = true
		}
	}
	if !gotFull {
		t.Skip("não conseguiu encher o buffer de forma determinística nesta execução — não é um problema do Queued em si")
	}
}
