package queue

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func newTestRedisQueue(t *testing.T, workers, maxLen int) *Queue {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	q, err := NewRedis(mr.Addr(), "", workers, maxLen)
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	return q
}

// newTestRedisQueueNoWorkers cria uma fila Redis sem NENHUM worker consumindo
// (NewRedis força ao menos DefaultWorkers) — usado pelos testes que
// verificam o estado bruto da lista (backpressure, sobrevivência ao Close).
func newTestRedisQueueNoWorkers(t *testing.T, maxLen int) *Queue {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	full, err := NewRedis(mr.Addr(), "", 1, maxLen)
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	full.workerCancel()
	full.wg.Wait()
	// cancel() por si só não marca closed — a fila continua aceitando
	// Enqueue/Close normalmente, só não tem worker consumindo.
	return full
}

func TestRedisProcessamentoBasico(t *testing.T) {
	q := newTestRedisQueue(t, 2, 16)
	var done atomic.Int32
	q.Register("soma", func(payload any) error {
		// payload volta do JSON como float64, não int (limitação documentada).
		done.Add(int32(payload.(float64)))
		return nil
	})

	for i := 0; i < 10; i++ {
		if err := q.Enqueue("soma", 1); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for done.Load() != 10 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	q.Close()

	if done.Load() != 10 {
		t.Errorf("esperava 10 tarefas processadas, recebeu %d", done.Load())
	}
}

func TestRedisEnqueueSemHandler(t *testing.T) {
	q := newTestRedisQueue(t, 1, 4)
	defer q.Close()
	if err := q.Enqueue("inexistente", nil); !errors.Is(err, ErrNoHandler) {
		t.Errorf("esperava ErrNoHandler, recebeu %v", err)
	}
}

func TestRedisBackpressure(t *testing.T) {
	q := newTestRedisQueueNoWorkers(t, 2) // sem workers consumindo — a lista só cresce
	q.Register("lenta", func(any) error { return nil })

	if err := q.Enqueue("lenta", nil); err != nil {
		t.Fatalf("enqueue 1: %v", err)
	}
	if err := q.Enqueue("lenta", nil); err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}
	if err := q.Enqueue("lenta", nil); !errors.Is(err, ErrQueueFull) {
		t.Errorf("esperava ErrQueueFull, recebeu %v", err)
	}
	q.Close()
}

func TestRedisEnqueueDepoisDeClose(t *testing.T) {
	q := newTestRedisQueue(t, 1, 4)
	q.Register("x", func(any) error { return nil })
	q.Close()
	if err := q.Enqueue("x", nil); !errors.Is(err, ErrQueueClosed) {
		t.Errorf("esperava ErrQueueClosed, recebeu %v", err)
	}
}

func TestRedisPanicNaoDerrubaWorker(t *testing.T) {
	q := newTestRedisQueue(t, 1, 4)
	var ok atomic.Bool
	q.Register("explode", func(any) error { panic("boom") })
	q.Register("normal", func(any) error { ok.Store(true); return nil })

	q.Enqueue("explode", nil)
	q.Enqueue("normal", nil)

	deadline := time.Now().Add(2 * time.Second)
	for !ok.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	q.Close()

	if !ok.Load() {
		t.Error("worker deveria sobreviver ao panic e processar a próxima tarefa")
	}
}

// TestRedisTarefaSobreviveAoClose confirma que itens ainda na lista quando
// Close() é chamado NÃO são perdidos (diferente do modo memória) — ficam no
// Redis para a próxima subida consumir.
func TestRedisTarefaSobreviveAoClose(t *testing.T) {
	q := newTestRedisQueueNoWorkers(t, 8) // sem workers consumindo
	q.Register("x", func(any) error { return nil })
	q.Enqueue("x", "a")
	q.Enqueue("x", "b")

	if n := q.Len(); n != 2 {
		t.Fatalf("esperava 2 itens na lista, encontrou %d", n)
	}
	q.Close()
}

func TestNewRedisQueueFalhaComEnderecoInvalido(t *testing.T) {
	if _, err := NewRedis("127.0.0.1:1", "", 1, 0); err == nil {
		t.Error("esperava erro ao conectar em endereço inválido")
	}
}
