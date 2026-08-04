// Package lifecycle orquestra as fases dos módulos ativos do Core
// (Init/Configure na ativação, Start em Core.Run, Shutdown em
// Core.Shutdown). A orquestração em si é síncrona e ordenada de propósito:
// Start de um módulo só roda depois que Init+Configure de todos os módulos
// já ativados terminaram sem erro, e Shutdown desce na ordem reversa da
// ativação (o último módulo a subir é o primeiro a descer, como um defer) —
// nada disso seria garantido se a orquestração passasse pelo core/events.Bus
// existente, que despacha cada handler numa goroutine própria e não espera
// nenhum deles terminar. Por isso o Bus aqui só recebe eventos informativos
// (Publish fire-and-forget, para quem quiser observar/logar), nunca decide
// ordem ou espera nada acontecer.
package lifecycle

import (
	"context"
	"fmt"

	"kyrux/core/events"
	"kyrux/core/registry"
)

// Nomes dos eventos de fase, publicados no Bus fornecido a NewManager.
const (
	BeforeInit     = "lifecycle:before_init"
	AfterInit      = "lifecycle:after_init"
	BeforeStart    = "lifecycle:before_start"
	AfterStart     = "lifecycle:after_start"
	BeforeShutdown = "lifecycle:before_shutdown"
	AfterShutdown  = "lifecycle:after_shutdown"
)

// Manager orquestra o ciclo de vida dos módulos ativos de um Core.
type Manager struct {
	bus    *events.Bus
	active []registry.Module
}

// NewManager cria um Manager que publica eventos de fase em bus.
func NewManager(bus *events.Bus) *Manager {
	return &Manager{bus: bus}
}

// Add ativa mod: chama Init e, se bem-sucedido, Configure. Só entra na lista
// de módulos ativos (e portanto só recebe Start/Shutdown depois) se as duas
// chamadas terminarem sem erro — um módulo não fica "pela metade" ativo.
func (m *Manager) Add(ctx context.Context, mod registry.Module) error {
	m.bus.Publish(BeforeInit, mod.Name())
	if err := mod.Init(ctx); err != nil {
		return fmt.Errorf("lifecycle: %s: init: %w", mod.Name(), err)
	}
	if err := mod.Configure(ctx); err != nil {
		return fmt.Errorf("lifecycle: %s: configure: %w", mod.Name(), err)
	}
	m.active = append(m.active, mod)
	m.bus.Publish(AfterInit, mod.Name())
	return nil
}

// StartAll chama Start em todos os módulos ativos, na ordem de ativação.
// Para no primeiro erro — módulos já iniciados continuam rodando (o
// chamador decide se quer Shutdown em seguida).
func (m *Manager) StartAll(ctx context.Context) error {
	m.bus.Publish(BeforeStart, nil)
	for _, mod := range m.active {
		if err := mod.Start(ctx); err != nil {
			return fmt.Errorf("lifecycle: %s: start: %w", mod.Name(), err)
		}
	}
	m.bus.Publish(AfterStart, nil)
	return nil
}

// ShutdownAll chama Shutdown em todos os módulos ativos, em ordem reversa à
// ativação. Não para no primeiro erro — tenta encerrar todos os módulos e
// devolve a lista de erros ocorridos (vazia se tudo correu bem).
func (m *Manager) ShutdownAll(ctx context.Context) []error {
	m.bus.Publish(BeforeShutdown, nil)
	var errs []error
	for i := len(m.active) - 1; i >= 0; i-- {
		mod := m.active[i]
		if err := mod.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("lifecycle: %s: shutdown: %w", mod.Name(), err))
		}
	}
	m.bus.Publish(AfterShutdown, nil)
	return errs
}

// Modules lista os módulos atualmente ativos, na ordem de ativação.
func (m *Manager) Modules() []registry.Module {
	return append([]registry.Module{}, m.active...)
}
