package orm

import (
	"fmt"
	"kyrux/core/database"
	"sync"
)

var (
	connsMu     sync.RWMutex
	connections = make(map[string]*database.DB)
	connOrder   []string // nomes na ordem de registro; o primeiro é o padrão
)

// DB retorna a conexão registrada pelo nome. O nome "default" resolve para
// o primeiro banco registrado quando nenhum bloco se chama literalmente
// "default" — mesmo comportamento de Manager.Use().
// Faz panic se a conexão não existir — falha intencional no startup.
func DB(name string) *database.DB {
	connsMu.RLock()
	db, ok := connections[name]
	if !ok && name == "default" && len(connOrder) > 0 {
		db, ok = connections[connOrder[0]], true
	}
	connsMu.RUnlock()
	if !ok {
		panic(fmt.Sprintf("orm: database '%s' não encontrada; verifique os blocos DB_NAME no .env", name))
	}
	return db
}

// Register adiciona (ou substitui) uma conexão nomeada no registry global.
// Chamado automaticamente por LoadDatabases e pode ser usado para conexões
// criadas manualmente fora do .env.
func Register(name string, db *database.DB) {
	connsMu.Lock()
	if _, exists := connections[name]; !exists {
		connOrder = append(connOrder, name)
	}
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
