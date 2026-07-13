// Package queue fornece uma fila de tarefas em memória com pool de workers,
// retries com backoff e shutdown gracioso.
//
// Papel de cada componente do Kyrux (não se sobrepõem):
//   - Cache    — armazenamento chave/valor com TTL (leitura rápida).
//   - EventBus — pub/sub fire-and-forget: TODOS os subscribers recebem,
//     sem retry e sem garantia de processamento.
//   - Queue    — cada tarefa é processada por UM worker, com retry em caso
//     de erro e drenagem na parada do servidor. Use para trabalho que não
//     pode se perder num pico: e-mails, webhooks, processamento de mídia.
//
// O driver atual é "memory" (por processo). O driver "redis" (fila
// compartilhada entre réplicas) está no roadmap — a API não mudará.
package queue

import (
	"errors"
	"log"
	"runtime/debug"
	"sync"
	"time"
)

var (
	ErrQueueFull   = errors.New("queue: fila cheia — tarefa recusada (backpressure)")
	ErrQueueClosed = errors.New("queue: fila encerrada")
	ErrNoHandler   = errors.New("queue: nenhum handler registrado para a tarefa")
)

const (
	DefaultWorkers    = 4
	DefaultBufferSize = 1024
	DefaultMaxRetries = 3
)

// Handler processa o payload de uma tarefa. Erro ≠ nil provoca retry
// (até MaxRetries) com backoff progressivo.
type Handler func(payload any) error

type task struct {
	name    string
	payload any
	attempt int
}

// Queue é uma fila de tarefas com workers em background.
// Crie com New e registre os handlers antes de enfileirar.
type Queue struct {
	mu         sync.RWMutex
	handlers   map[string]Handler
	tasks      chan task
	wg         sync.WaitGroup
	closed     bool
	maxRetries int
}

// New cria a fila e inicia os workers. workers/buffer ≤ 0 usam os defaults.
func New(workers, buffer int) *Queue {
	if workers <= 0 {
		workers = DefaultWorkers
	}
	if buffer <= 0 {
		buffer = DefaultBufferSize
	}
	q := &Queue{
		handlers:   make(map[string]Handler),
		tasks:      make(chan task, buffer),
		maxRetries: DefaultMaxRetries,
	}
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker()
	}
	return q
}

// SetMaxRetries ajusta o número máximo de tentativas por tarefa (padrão: 3).
func (q *Queue) SetMaxRetries(n int) {
	q.mu.Lock()
	q.maxRetries = n
	q.mu.Unlock()
}

// Register associa um handler ao nome da tarefa.
// Chame no Register do app, antes de qualquer Enqueue.
func (q *Queue) Register(name string, h Handler) {
	q.mu.Lock()
	q.handlers[name] = h
	q.mu.Unlock()
}

// Enqueue enfileira uma tarefa para processamento em background.
// Retorna ErrQueueFull se o buffer estiver cheio (backpressure explícito —
// decida no caller entre descartar, responder 503 ou tentar de novo) e
// ErrNoHandler se nenhum handler foi registrado para o nome.
func (q *Queue) Enqueue(name string, payload any) error {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return ErrQueueClosed
	}
	if _, ok := q.handlers[name]; !ok {
		return ErrNoHandler
	}
	select {
	case q.tasks <- task{name: name, payload: payload}:
		return nil
	default:
		return ErrQueueFull
	}
}

// Len retorna o número de tarefas aguardando processamento.
func (q *Queue) Len() int { return len(q.tasks) }

// Close para de aceitar tarefas e espera os workers drenarem a fila.
// Chamado automaticamente pelo bootstrap no shutdown do servidor. Idempotente.
func (q *Queue) Close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	close(q.tasks)
	q.mu.Unlock()
	q.wg.Wait()
}

func (q *Queue) worker() {
	defer q.wg.Done()
	for t := range q.tasks {
		q.process(t)
	}
}

func (q *Queue) process(t task) {
	// Panic num handler não pode derrubar o worker (nem o processo).
	defer func() {
		if r := recover(); r != nil {
			log.Printf("queue: panic na tarefa %q: %v\n%s", t.name, r, debug.Stack())
		}
	}()

	q.mu.RLock()
	h := q.handlers[t.name]
	maxRetries := q.maxRetries
	q.mu.RUnlock()
	if h == nil {
		log.Printf("queue: tarefa %q sem handler — descartada", t.name)
		return
	}

	err := h(t.payload)
	if err == nil {
		return
	}

	if t.attempt+1 >= maxRetries {
		log.Printf("queue: tarefa %q descartada após %d tentativas: %v", t.name, t.attempt+1, err)
		return
	}

	// Retry com backoff progressivo (1s, 2s, 3s...) sem bloquear o worker.
	t.attempt++
	backoff := time.Duration(t.attempt) * time.Second
	log.Printf("queue: tarefa %q falhou (tentativa %d/%d), retry em %s: %v",
		t.name, t.attempt, maxRetries, backoff, err)
	time.AfterFunc(backoff, func() {
		q.mu.RLock()
		defer q.mu.RUnlock()
		if q.closed {
			log.Printf("queue: tarefa %q perdida — fila encerrada durante retry", t.name)
			return
		}
		select {
		case q.tasks <- t:
		default:
			log.Printf("queue: tarefa %q perdida — fila cheia durante retry", t.name)
		}
	})
}
