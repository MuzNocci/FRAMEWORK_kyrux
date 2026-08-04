// Package registry implementa o "hot plug" do Core do Kyrux: um módulo se
// registra sozinho (via init(), disparado só por quem realmente importa o
// pacote dele) e fica disponível para ativação pelo nome — sem que o Core
// ou qualquer outro módulo precise importar o pacote diretamente. É o mesmo
// truque do database/sql da stdlib (sql.Register, disparado pelo blank
// import de um driver), generalizado para qualquer tipo de módulo.
package registry

import (
	"context"
	"fmt"
	"sync"
)

// Module é o contrato que todo módulo plugável no Core precisa implementar.
// As quatro fases são chamadas nesta ordem pelo core/lifecycle: Init e
// Configure na ativação (core.X.Y()), Start em Core.Run(), Shutdown em
// Core.Shutdown() (na ordem reversa de ativação).
type Module interface {
	// Name identifica o módulo (ex: "cache.memory", "database.sql.postgres").
	// Usado em logs, eventos de lifecycle e diagnóstico — não precisa (e
	// normalmente não deve) ser igual ao nome usado no Register.
	Name() string

	// Init prepara o módulo (validação de config, alocação de estruturas
	// internas) — não deve, ainda, abrir conexões externas custosas.
	Init(ctx context.Context) error

	// Configure roda depois de Init — normalmente é aqui que a conexão
	// externa de fato acontece (conectar no banco, no broker, etc).
	Configure(ctx context.Context) error

	// Start roda quando a aplicação sobe de verdade (Core.Run) — para
	// módulos que precisam de uma etapa "ativa" separada da conexão (ex:
	// começar a consumir uma fila). Módulos sem essa necessidade podem
	// simplesmente retornar nil.
	Start(ctx context.Context) error

	// Shutdown encerra o módulo de forma organizada (fechar conexões,
	// parar goroutines). Chamado na ordem reversa de ativação.
	Shutdown(ctx context.Context) error
}

// Factory cria uma nova instância de um módulo — chamada uma vez por
// ativação (core.X.Y()), nunca reaproveitada entre ativações diferentes.
type Factory func() Module

var (
	mu    sync.RWMutex
	items = map[string]Factory{}
)

// Register registra um módulo pelo nome. Chamado no init() do pacote que
// implementa o módulo — nunca pelo Core nem por código de aplicação
// diretamente. Entra em panic em nome duplicado: um Register duplicado só
// acontece por erro de programação (dois pacotes competindo pelo mesmo
// nome), nunca em uso normal.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := items[name]; exists {
		panic("registry: módulo já registrado: " + name)
	}
	items[name] = f
}

// New instancia um módulo registrado pelo nome. Retorna erro se o pacote
// correspondente nunca foi importado — sem blank import, o init() que
// chamaria Register nunca roda, e o nome simplesmente não existe aqui.
func New(name string) (Module, error) {
	mu.RLock()
	f, ok := items[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("registry: módulo %q não registrado — importe o pacote do adapter correspondente (import _ \"...\")", name)
	}
	return f(), nil
}

// Names lista os nomes de módulos registrados no momento da chamada (não
// preserva ordem de registro).
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	return names
}
