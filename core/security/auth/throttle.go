package auth

import (
	"errors"
	"sync"
	"time"
)

// ErrTooManyAttempts é retornado pelo Login quando uma conta acumula tentativas
// falhas demais dentro da janela — freia brute-force mesmo que o desenvolvedor
// não tenha aplicado o middleware RateLimit na rota.
var ErrTooManyAttempts = errors.New("too many failed login attempts")

const (
	defaultMaxAttempts = 10          // falhas permitidas por chave antes do bloqueio
	defaultLockWindow  = time.Minute // janela de contagem / duração do bloqueio
)

type attemptRecord struct {
	count   int
	resetAt time.Time
}

// loginThrottle limita tentativas falhas de login por chave (login+IP).
// É defesa em profundidade junto ao custo do Argon2id: sem ele, um atacante
// pode testar senhas continuamente contra uma conta conhecida.
type loginThrottle struct {
	mu          sync.Mutex
	records     map[string]*attemptRecord
	maxAttempts int
	window      time.Duration
	lastSweep   time.Time
}

var throttle = &loginThrottle{
	records:     make(map[string]*attemptRecord),
	maxAttempts: defaultMaxAttempts,
	window:      defaultLockWindow,
	lastSweep:   time.Now(),
}

// SetLoginThrottle ajusta o limite de tentativas falhas e a janela.
// Chame no bootstrap se quiser políticas diferentes das padrão (10/minuto).
func SetLoginThrottle(maxAttempts int, window time.Duration) {
	throttle.mu.Lock()
	if maxAttempts > 0 {
		throttle.maxAttempts = maxAttempts
	}
	if window > 0 {
		throttle.window = window
	}
	throttle.mu.Unlock()
}

// blocked reporta se a chave está bloqueada por excesso de falhas.
func (t *loginThrottle) blocked(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweepLocked()
	r, ok := t.records[key]
	if !ok || time.Now().After(r.resetAt) {
		return false
	}
	return r.count >= t.maxAttempts
}

// fail registra uma tentativa falha para a chave.
func (t *loginThrottle) fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	r, ok := t.records[key]
	if !ok || now.After(r.resetAt) {
		t.records[key] = &attemptRecord{count: 1, resetAt: now.Add(t.window)}
		return
	}
	r.count++
}

// reset limpa o contador após um login bem-sucedido.
func (t *loginThrottle) reset(key string) {
	t.mu.Lock()
	delete(t.records, key)
	t.mu.Unlock()
}

// sweepLocked remove registros expirados (deve ser chamado com lock).
func (t *loginThrottle) sweepLocked() {
	now := time.Now()
	if now.Sub(t.lastSweep) < t.window {
		return
	}
	for k, r := range t.records {
		if now.After(r.resetAt) {
			delete(t.records, k)
		}
	}
	t.lastSweep = now
}
