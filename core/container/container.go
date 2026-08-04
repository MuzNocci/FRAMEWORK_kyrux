// Package container implementa um Container de Dependency Injection simples
// para o Core do Kyrux: cada valor é guardado sob um nome (não um tipo) —
// necessário porque é normal ter mais de uma instância do mesmo tipo Go
// coexistindo (ex: dois *database.DB, um Postgres e um MySQL, ativos ao
// mesmo tempo, cada um sob seu próprio nome). O tipo T em Provide/Resolve só
// garante segurança de tipo na borda de cada chamada — a chave real é name.
package container

import (
	"fmt"
	"sync"
)

// Container guarda valores nomeados, fornecidos pelos módulos ativos do
// Core, para serem resolvidos por outras partes da aplicação (ou por outros
// módulos, através do próprio Core — módulos nunca se importam entre si).
type Container struct {
	mu     sync.RWMutex
	values map[string]any
}

// New cria um Container vazio.
func New() *Container {
	return &Container{values: make(map[string]any)}
}

// Provide registra value sob name, substituindo qualquer valor anterior com
// o mesmo nome.
func Provide[T any](c *Container, name string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[name] = value
}

// Resolve busca o valor registrado sob name e o converte para T. ok=false
// se não houver nada sob esse nome, ou se o valor existente não for do
// tipo T pedido.
func Resolve[T any](c *Container, name string) (T, bool) {
	c.mu.RLock()
	v, exists := c.values[name]
	c.mu.RUnlock()

	var zero T
	if !exists {
		return zero, false
	}
	typed, ok := v.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

// MustResolve é como Resolve, mas entra em panic se name não estiver
// registrado (ou não for do tipo T) — use só em código de inicialização
// onde a ausência é um erro de programação, não uma condição esperada em
// runtime.
func MustResolve[T any](c *Container, name string) T {
	v, ok := Resolve[T](c, name)
	if !ok {
		panic(fmt.Sprintf("container: %q não foi fornecido (ou não é do tipo esperado)", name))
	}
	return v
}

// Names lista os nomes atualmente registrados no Container.
func (c *Container) Names() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make([]string, 0, len(c.values))
	for name := range c.values {
		names = append(names, name)
	}
	return names
}
