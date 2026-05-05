package database

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// DB encapsula *sql.DB com o nome do driver e um schema opcional.
type DB struct {
	*sql.DB
	Driver string
	Schema string
}

// WithSchema retorna uma cópia de DB com o schema definido — útil para multi-tenant.
//
//	db := fw.DB.Use().WithSchema("tenant_abc")
func (db *DB) WithSchema(schema string) *DB {
	return &DB{DB: db.DB, Driver: db.Driver, Schema: schema}
}

func (db *DB) Transaction(fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Manager gerencia múltiplas conexões SQL nomeadas.
type Manager struct {
	mu    sync.RWMutex
	conns map[string]*DB
}

func NewManager() *Manager {
	return &Manager{conns: make(map[string]*DB)}
}

// Add abre e registra uma conexão nomeada.
// Use "default" como name para a conexão principal.
func (m *Manager) Add(name, driver, dsn string) error {
	db, err := open(driver, dsn)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.conns[name] = db
	m.mu.Unlock()
	return nil
}

// Use retorna a conexão pelo nome (padrão: "default").
func (m *Manager) Use(name ...string) *DB {
	key := "default"
	if len(name) > 0 && name[0] != "" {
		key = name[0]
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conns[key]
}

// Close encerra todas as conexões registradas.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, db := range m.conns {
		db.Close()
	}
}

// Open abre uma conexão direta sem registrá-la no Manager.
// Indicado para ferramentas CLI que não precisam do Manager completo.
func Open(driver, dsn string) (*DB, error) {
	return open(driver, dsn)
}

func open(driver, dsn string) (*DB, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("database: open [%s]: %w", driver, err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("database: ping [%s]: %w", driver, err)
	}
	return &DB{DB: db, Driver: driver}, nil
}
