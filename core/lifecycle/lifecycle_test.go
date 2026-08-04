package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"kyrux/core/events"
)

// trackingModule registra, num log compartilhado, cada fase chamada nele —
// usado para verificar ordem de execução (Init/Configure na ativação,
// Start/Shutdown nas chamadas em lote do Manager).
type trackingModule struct {
	name string
	log  *[]string
	mu   *sync.Mutex

	failInit      bool
	failConfigure bool
	failStart     bool
	failShutdown  bool
}

func (m *trackingModule) record(phase string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	*m.log = append(*m.log, m.name+":"+phase)
}

func (m *trackingModule) Name() string { return m.name }

func (m *trackingModule) Init(ctx context.Context) error {
	m.record("init")
	if m.failInit {
		return errors.New("falha proposital em init")
	}
	return nil
}

func (m *trackingModule) Configure(ctx context.Context) error {
	m.record("configure")
	if m.failConfigure {
		return errors.New("falha proposital em configure")
	}
	return nil
}

func (m *trackingModule) Start(ctx context.Context) error {
	m.record("start")
	if m.failStart {
		return errors.New("falha proposital em start")
	}
	return nil
}

func (m *trackingModule) Shutdown(ctx context.Context) error {
	m.record("shutdown")
	if m.failShutdown {
		return errors.New("falha proposital em shutdown")
	}
	return nil
}

func newTrackingModule(name string, log *[]string, mu *sync.Mutex) *trackingModule {
	return &trackingModule{name: name, log: log, mu: mu}
}

func TestAddChamaInitEConfigureNaOrdem(t *testing.T) {
	var log []string
	var mu sync.Mutex
	mgr := NewManager(events.NewBus())

	mod := newTrackingModule("a", &log, &mu)
	if err := mgr.Add(context.Background(), mod); err != nil {
		t.Fatalf("Add: %v", err)
	}

	want := []string{"a:init", "a:configure"}
	if len(log) != len(want) || log[0] != want[0] || log[1] != want[1] {
		t.Errorf("ordem incorreta: %v", log)
	}
}

func TestAddComFalhaEmInitNaoAtivaModulo(t *testing.T) {
	var log []string
	var mu sync.Mutex
	mgr := NewManager(events.NewBus())

	mod := newTrackingModule("a", &log, &mu)
	mod.failInit = true
	if err := mgr.Add(context.Background(), mod); err == nil {
		t.Fatal("esperava erro de Add quando Init falha")
	}
	if len(mgr.Modules()) != 0 {
		t.Error("módulo com Init falho não deveria entrar na lista de ativos")
	}
	// Configure não deve ter sido chamado — Init falhou antes.
	for _, entry := range log {
		if entry == "a:configure" {
			t.Error("Configure não deveria rodar depois de Init falhar")
		}
	}
}

func TestAddComFalhaEmConfigureNaoAtivaModulo(t *testing.T) {
	var log []string
	var mu sync.Mutex
	mgr := NewManager(events.NewBus())

	mod := newTrackingModule("a", &log, &mu)
	mod.failConfigure = true
	if err := mgr.Add(context.Background(), mod); err == nil {
		t.Fatal("esperava erro de Add quando Configure falha")
	}
	if len(mgr.Modules()) != 0 {
		t.Error("módulo com Configure falho não deveria entrar na lista de ativos")
	}
}

func TestStartAllRespeitaOrdemDeAtivacao(t *testing.T) {
	var log []string
	var mu sync.Mutex
	mgr := NewManager(events.NewBus())

	a := newTrackingModule("a", &log, &mu)
	b := newTrackingModule("b", &log, &mu)
	if err := mgr.Add(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Add(context.Background(), b); err != nil {
		t.Fatal(err)
	}

	log = nil // limpa o log de Init/Configure pra focar só no Start
	if err := mgr.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll: %v", err)
	}

	want := []string{"a:start", "b:start"}
	if len(log) != 2 || log[0] != want[0] || log[1] != want[1] {
		t.Errorf("StartAll não respeitou a ordem de ativação: %v", log)
	}
}

func TestStartAllParaNoPrimeiroErro(t *testing.T) {
	var log []string
	var mu sync.Mutex
	mgr := NewManager(events.NewBus())

	a := newTrackingModule("a", &log, &mu)
	a.failStart = true
	b := newTrackingModule("b", &log, &mu)
	if err := mgr.Add(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Add(context.Background(), b); err != nil {
		t.Fatal(err)
	}

	log = nil
	if err := mgr.StartAll(context.Background()); err == nil {
		t.Fatal("esperava erro de StartAll quando um módulo falha em Start")
	}
	for _, entry := range log {
		if entry == "b:start" {
			t.Error("StartAll não deveria continuar pros módulos seguintes após um erro")
		}
	}
}

func TestShutdownAllOrdemReversaETentaTodos(t *testing.T) {
	var log []string
	var mu sync.Mutex
	mgr := NewManager(events.NewBus())

	a := newTrackingModule("a", &log, &mu)
	b := newTrackingModule("b", &log, &mu)
	b.failShutdown = true
	c := newTrackingModule("c", &log, &mu)
	for _, m := range []*trackingModule{a, b, c} {
		if err := mgr.Add(context.Background(), m); err != nil {
			t.Fatal(err)
		}
	}

	log = nil
	errs := mgr.ShutdownAll(context.Background())
	if len(errs) != 1 {
		t.Fatalf("esperava 1 erro (de b), recebeu %d: %v", len(errs), errs)
	}

	want := []string{"c:shutdown", "b:shutdown", "a:shutdown"}
	if len(log) != 3 || log[0] != want[0] || log[1] != want[1] || log[2] != want[2] {
		t.Errorf("ShutdownAll deveria rodar em ordem reversa e tentar todos mesmo com erro no meio: %v", log)
	}
}

func TestEventosDeFaseSaoPublicados(t *testing.T) {
	bus := events.NewBus()
	mgr := NewManager(bus)

	var mu sync.Mutex
	received := map[string]int{}
	done := make(chan struct{}, 10)
	for _, ev := range []string{BeforeInit, AfterInit, BeforeStart, AfterStart, BeforeShutdown, AfterShutdown} {
		ev := ev
		bus.Subscribe(ev, func(payload any) {
			mu.Lock()
			received[ev]++
			mu.Unlock()
			done <- struct{}{}
		})
	}

	var log []string
	var logMu sync.Mutex
	mod := newTrackingModule("a", &log, &logMu)
	if err := mgr.Add(context.Background(), mod); err != nil {
		t.Fatal(err)
	}
	if err := mgr.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	mgr.ShutdownAll(context.Background())

	// O Bus despacha cada handler em goroutine própria (fire-and-forget) —
	// espera os 6 eventos chegarem, com timeout curto em vez de sleep fixo.
	for i := 0; i < 6; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout esperando eventos de fase")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for _, ev := range []string{BeforeInit, AfterInit, BeforeStart, AfterStart, BeforeShutdown, AfterShutdown} {
		if received[ev] != 1 {
			t.Errorf("evento %q: esperava 1 publicação, recebeu %d", ev, received[ev])
		}
	}
}
