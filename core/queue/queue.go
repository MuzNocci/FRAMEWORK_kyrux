// Package queue fornece uma fila de tarefas com pool de workers, retries
// com backoff e shutdown gracioso — em dois modos possíveis.
//
// Papel de cada componente do Kyrux (não se sobrepõem):
//   - Cache    — armazenamento chave/valor com TTL (leitura rápida).
//   - EventBus — pub/sub fire-and-forget: TODOS os subscribers recebem,
//     sem retry e sem garantia de processamento.
//   - Queue    — cada tarefa é processada por UM worker, com retry em caso
//     de erro e drenagem na parada do servidor. Use para trabalho que não
//     pode se perder num pico: e-mails, webhooks, processamento de mídia.
//
// Modos:
//   - memória (New): fila local ao processo (channel Go) — não compartilhada
//     entre réplicas, perdida num restart.
//   - Redis (NewRedis): lista Redis (LPUSH/BRPOP) compartilhada entre todas
//     as réplicas — cada réplica registra os mesmos handlers via Register e
//     qualquer uma pode processar uma tarefa enfileirada por outra. Payload é
//     serializado com encoding/json: um struct enfileirado com Enqueue volta
//     ao Handler como map[string]any, não no tipo original (mesma limitação
//     do cache Redis — prefira tipos simples ou decodifique você mesmo).
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
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

	redisListKey = "kyrux:queue:tasks"

	// redisBRPopTimeout é quanto cada worker fica bloqueado por ciclo
	// esperando um item novo na lista (BRPOP volta com redis.Nil se ninguém
	// enfileirar nesse intervalo, e o worker tenta de novo). O ReadTimeout do
	// client precisa ficar folgadamente acima disso — ver NewRedis.
	redisBRPopTimeout = 2 * time.Second
)

// Handler processa o payload de uma tarefa. Erro ≠ nil provoca retry
// (até MaxRetries) com backoff progressivo.
type Handler func(payload any) error

type task struct {
	name    string
	payload any
	attempt int
}

// wireTask é a representação serializada de task no modo Redis — task tem
// campos não-exportados (não dá pra marshalar direto).
type wireTask struct {
	Name    string `json:"name"`
	Payload any    `json:"payload"`
	Attempt int    `json:"attempt"`
}

// Queue é uma fila de tarefas com workers em background.
// Crie com New (memória) ou NewRedis (compartilhada) e registre os handlers
// antes de enfileirar.
type Queue struct {
	mu         sync.RWMutex
	handlers   map[string]Handler
	tasks      chan task // modo memória; nil em modo Redis
	wg         sync.WaitGroup
	closed     bool
	maxRetries int

	// redis != nil ativa o modo Redis — todos os métodos abaixo checam esse
	// campo primeiro e desviam para a lista Redis antes de tocar em tasks.
	redis  *redis.Client
	maxLen int64

	// redisCtx é usado por Enqueue/Len (nunca cancelado enquanto redis != nil
	// — só fechado via q.redis.Close() no Close()). workerCtx é exclusivo do
	// loop de BRPop em redisWorker: cancelá-lo desbloqueia o BRPop em
	// andamento sem afetar chamadas concorrentes de Enqueue/Len.
	redisCtx     context.Context
	workerCtx    context.Context
	workerCancel context.CancelFunc
}

// New cria a fila em memória e inicia os workers. workers/buffer ≤ 0 usam os defaults.
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

// NewRedis conecta a um servidor Redis em addr e o usa como backend da
// fila — tarefas ficam numa lista Redis compartilhada entre réplicas, ao
// contrário do modo memória. password é opcional (vazio = sem AUTH).
// workers/maxLen ≤ 0 usam os defaults. Faz PING antes de retornar — se o
// servidor não responder, retorna erro para o chamador decidir (o bootstrap
// cai para memória e loga um aviso, em vez de recusar subir).
func NewRedis(addr, password string, workers, maxLen int) (*Queue, error) {
	if workers <= 0 {
		workers = DefaultWorkers
	}
	if maxLen <= 0 {
		maxLen = DefaultBufferSize
	}

	// ReadTimeout precisa ficar bem acima de redisBRPopTimeout: o BRPOP dos
	// workers fica bloqueado no servidor por até redisBRPopTimeout esperando
	// item novo, e se o ReadTimeout do client for menor (ou só um pouco
	// maior — o default do go-redis é 3s, folga de só 1s sobre os 2s do
	// BRPOP) o socket estoura o prazo ANTES do servidor responder,
	// derrubando a conexão como "i/o timeout" em praticamente todo ciclo.
	// Isso não é só log poluído: a reconexão constante consome o pool de
	// conexões compartilhado com o Enqueue() do caminho HTTP, atrasando (ou
	// travando) o enfileiramento de quem só queria mandar e-mail em
	// background — o sintoma observado foi candidatura em Carreiras
	// demorando até estourar 502 no proxy, quando devia ser instantâneo.
	readTimeout := redisBRPopTimeout + 5*time.Second
	rdb := redis.NewClient(&redis.Options{
		Addr:        addr,
		Password:    password,
		MaxRetries:  -1,
		ReadTimeout: readTimeout,
	})
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		rdb.Close()
		return nil, fmt.Errorf("queue: conectar ao redis em %s: %w", addr, err)
	}

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	q := &Queue{
		handlers:     make(map[string]Handler),
		maxRetries:   DefaultMaxRetries,
		redis:        rdb,
		redisCtx:     context.Background(),
		workerCtx:    workerCtx,
		workerCancel: cancelWorker,
		maxLen:       int64(maxLen),
	}
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.redisWorker()
	}
	return q, nil
}

// SetMaxRetries ajusta o número máximo de tentativas por tarefa (padrão: 3).
func (q *Queue) SetMaxRetries(n int) {
	q.mu.Lock()
	q.maxRetries = n
	q.mu.Unlock()
}

// Register associa um handler ao nome da tarefa.
// Chame no Register do app, antes de qualquer Enqueue. Em modo Redis, TODAS
// as réplicas precisam registrar os mesmos handlers — qualquer uma pode
// receber uma tarefa enfileirada por outra.
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
	closed := q.closed
	_, hasHandler := q.handlers[name]
	q.mu.RUnlock()
	if closed {
		return ErrQueueClosed
	}
	if !hasHandler {
		return ErrNoHandler
	}
	return q.enqueueTask(task{name: name, payload: payload})
}

func (q *Queue) enqueueTask(t task) error {
	if q.redis != nil {
		n, err := q.redis.LLen(q.redisCtx, redisListKey).Result()
		if err == nil && n >= q.maxLen {
			return ErrQueueFull
		}
		data, err := json.Marshal(wireTask{Name: t.name, Payload: t.payload, Attempt: t.attempt})
		if err != nil {
			return fmt.Errorf("queue: payload de %q não é serializável em JSON (modo redis): %w", t.name, err)
		}
		if err := q.redis.LPush(q.redisCtx, redisListKey, data).Err(); err != nil {
			return fmt.Errorf("queue: falha ao enfileirar %q no redis: %w", t.name, err)
		}
		return nil
	}

	select {
	case q.tasks <- t:
		return nil
	default:
		return ErrQueueFull
	}
}

// Len retorna o número de tarefas aguardando processamento.
func (q *Queue) Len() int {
	if q.redis != nil {
		n, err := q.redis.LLen(q.redisCtx, redisListKey).Result()
		if err != nil {
			return 0
		}
		return int(n)
	}
	return len(q.tasks)
}

// Close para de aceitar tarefas e espera os workers atuais terminarem.
// Chamado automaticamente pelo bootstrap no shutdown do servidor. Idempotente.
//
// Em modo Redis, tarefas ainda na lista NÃO são perdidas (drenagem real só
// no modo memória) — continuam lá para a próxima vez que este processo (ou
// outra réplica) subir e voltar a consumir a fila.
func (q *Queue) Close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true

	if q.redis != nil {
		q.mu.Unlock()
		q.workerCancel() // desbloqueia BRPop em andamento
		q.wg.Wait()
		q.redis.Close()
		return
	}

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

func (q *Queue) redisWorker() {
	defer q.wg.Done()
	for {
		res, err := q.redis.BRPop(q.workerCtx, redisBRPopTimeout, redisListKey).Result()
		if err != nil {
			if q.workerCtx.Err() != nil {
				return // Close() cancelou o contexto
			}
			if errors.Is(err, redis.Nil) {
				continue // timeout do BRPOP sem itens na lista — tenta de novo
			}
			log.Printf("queue: erro ao ler do redis: %v\n", err)
			time.Sleep(time.Second)
			continue
		}
		if len(res) != 2 {
			continue
		}
		var wt wireTask
		if err := json.Unmarshal([]byte(res[1]), &wt); err != nil {
			log.Printf("queue: item inválido na lista do redis, descartado: %v\n", err)
			continue
		}
		q.process(task{name: wt.Name, payload: wt.Payload, attempt: wt.Attempt})
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
	time.AfterFunc(backoff, func() { q.requeue(t) })
}

// requeue tenta reenfileirar uma tarefa após falha (retry). Usado tanto no
// modo memória quanto Redis — ver process().
func (q *Queue) requeue(t task) {
	q.mu.RLock()
	closed := q.closed
	q.mu.RUnlock()
	if closed {
		log.Printf("queue: tarefa %q perdida — fila encerrada durante retry", t.name)
		return
	}
	if err := q.enqueueTask(t); err != nil {
		log.Printf("queue: tarefa %q perdida — %v", t.name, err)
	}
}
