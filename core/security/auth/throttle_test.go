package auth

import (
	"testing"
	"time"
)

func newThrottle(max int, window time.Duration) *loginThrottle {
	return &loginThrottle{
		records:     make(map[string]*attemptRecord),
		maxAttempts: max,
		window:      window,
		lastSweep:   time.Now(),
	}
}

func TestThrottleBloqueiaAposLimite(t *testing.T) {
	tr := newThrottle(3, time.Minute)
	key := "alice|10.0.0.1"

	for i := 0; i < 3; i++ {
		if tr.blocked(key) {
			t.Fatalf("não deveria bloquear antes do limite (tentativa %d)", i)
		}
		tr.fail(key)
	}
	if !tr.blocked(key) {
		t.Fatal("deveria bloquear após 3 falhas")
	}
}

func TestThrottleResetLimpaContador(t *testing.T) {
	tr := newThrottle(3, time.Minute)
	key := "bob|10.0.0.1"
	tr.fail(key)
	tr.fail(key)
	tr.reset(key) // login bem-sucedido
	tr.fail(key)
	if tr.blocked(key) {
		t.Error("após reset, o contador deveria recomeçar do zero")
	}
}

func TestThrottleExpiraJanela(t *testing.T) {
	tr := newThrottle(2, 20*time.Millisecond)
	key := "carol|10.0.0.1"
	tr.fail(key)
	tr.fail(key)
	if !tr.blocked(key) {
		t.Fatal("deveria estar bloqueada")
	}
	time.Sleep(30 * time.Millisecond)
	if tr.blocked(key) {
		t.Error("após a janela expirar, deveria desbloquear")
	}
}

func TestThrottleChavesIndependentes(t *testing.T) {
	tr := newThrottle(2, time.Minute)
	tr.fail("alice|10.0.0.1")
	tr.fail("alice|10.0.0.1")
	if tr.blocked("alice|10.0.0.2") {
		t.Error("IP diferente não deveria ser afetado pelo bloqueio de outro")
	}
}
