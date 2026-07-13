package queue

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestProcessamentoBasico(t *testing.T) {
	q := New(2, 16)
	var done atomic.Int32
	q.Register("soma", func(payload any) error {
		done.Add(int32(payload.(int)))
		return nil
	})

	for i := 0; i < 10; i++ {
		if err := q.Enqueue("soma", 1); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
	q.Close() // drena antes de retornar

	if done.Load() != 10 {
		t.Errorf("esperava 10 tarefas processadas, recebeu %d", done.Load())
	}
}

func TestEnqueueSemHandler(t *testing.T) {
	q := New(1, 4)
	defer q.Close()
	if err := q.Enqueue("inexistente", nil); !errors.Is(err, ErrNoHandler) {
		t.Errorf("esperava ErrNoHandler, recebeu %v", err)
	}
}

func TestBackpressure(t *testing.T) {
	q := New(1, 1)
	block := make(chan struct{})
	q.Register("lenta", func(any) error { <-block; return nil })

	// 1 no worker + 1 no buffer; a terceira deve ser recusada.
	q.Enqueue("lenta", nil)
	deadline := time.Now().Add(2 * time.Second)
	for q.Len() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond) // espera o worker consumir a primeira
	}
	q.Enqueue("lenta", nil)

	if err := q.Enqueue("lenta", nil); !errors.Is(err, ErrQueueFull) {
		t.Errorf("esperava ErrQueueFull, recebeu %v", err)
	}
	close(block)
	q.Close()
}

func TestEnqueueDepoisDeClose(t *testing.T) {
	q := New(1, 4)
	q.Register("x", func(any) error { return nil })
	q.Close()
	if err := q.Enqueue("x", nil); !errors.Is(err, ErrQueueClosed) {
		t.Errorf("esperava ErrQueueClosed, recebeu %v", err)
	}
}

func TestPanicNoHandlerNaoDerrubaWorker(t *testing.T) {
	q := New(1, 4)
	var ok atomic.Bool
	q.Register("explode", func(any) error { panic("boom") })
	q.Register("normal", func(any) error { ok.Store(true); return nil })

	q.Enqueue("explode", nil)
	q.Enqueue("normal", nil)
	q.Close()

	if !ok.Load() {
		t.Error("worker deveria sobreviver ao panic e processar a próxima tarefa")
	}
}
