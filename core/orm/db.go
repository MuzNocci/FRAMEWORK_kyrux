package orm

import (
	"fmt"
	"kyrux/core/database"
	"sync"
)

var (
	connsMu     sync.RWMutex
	connections = make(map[string]*database.DB)
)

// DB retorna a conexão registrada pelo nome.
// Faz panic se a conexão não existir — falha intencional no startup.
func DB(name string) *database.DB {
	connsMu.RLock()
	db, ok := connections[name]
	connsMu.RUnlock()
	if !ok {
		panic(fmt.Sprintf("orm: database '%s' não encontrada; verifique DATABASES_JSON", name))
	}
	return db
}

// Register adiciona (ou substitui) uma conexão nomeada no registry global.
// Chamado automaticamente por LoadDatabases e pode ser usado para conexões
// criadas fora do DATABASES_JSON.
func Register(name string, db *database.DB) {
	connsMu.Lock()
	connections[name] = db
	connsMu.Unlock()
}

// HasConnections reporta se há ao menos uma conexão registrada.
func HasConnections() bool {
	connsMu.RLock()
	n := len(connections)
	connsMu.RUnlock()
	return n > 0
}
