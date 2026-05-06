package database

import (
	"context"
	"database/sql"
	"fmt"
	"kyrux/core/environment"
	"strconv"
	"sync"
	"time"
)

// DB encapsula *sql.DB com o nome do driver e um schema opcional.
type DB struct {
	*sql.DB
	Driver string
	Schema string
	stmtMu sync.RWMutex
	stmts  map[string]*sql.Stmt
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
	order []string
}

func NewManager() *Manager {
	return &Manager{conns: make(map[string]*DB)}
}

// AddDB registra uma conexão já aberta no Manager sem reabri-la.
// Útil quando a conexão foi criada externamente (ex: orm.LoadDatabases).
func (m *Manager) AddDB(name string, db *DB) {
	m.mu.Lock()
	if _, exists := m.conns[name]; !exists {
		m.order = append(m.order, name)
	}
	m.conns[name] = db
	m.mu.Unlock()
}

// Add abre e registra uma conexão nomeada.
// Use "default" como name para a conexão principal.
func (m *Manager) Add(name, driver, dsn string) error {
	db, err := open(driver, dsn)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if _, exists := m.conns[name]; !exists {
		m.order = append(m.order, name)
	}
	m.conns[name] = db
	m.mu.Unlock()
	return nil
}

// ConnInfo descreve uma conexão registrada no Manager.
type ConnInfo struct {
	Name   string
	Driver string
	Status string // "online" | "offline"
}

// Info retorna o status de todas as conexões registradas na ordem de inserção.
func (m *Manager) Info() []ConnInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ConnInfo, 0, len(m.order))
	for _, name := range m.order {
		db := m.conns[name]
		status := "online"
		if err := db.PingContext(ctx); err != nil {
			status = "offline"
		}
		result = append(result, ConnInfo{Name: name, Driver: db.Driver, Status: status})
	}
	return result
}

// Use retorna a conexão pelo nome. Sem argumento retorna a conexão "default";
// se não houver uma conexão chamada "default", retorna a primeira registrada.
func (m *Manager) Use(name ...string) *DB {
	key := "default"
	if len(name) > 0 && name[0] != "" {
		key = name[0]
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if db, ok := m.conns[key]; ok {
		return db
	}
	if len(m.order) > 0 {
		return m.conns[m.order[0]]
	}
	return nil
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
	// Configuráveis via env vars (se presentes) — fallback para valores sensatos.
	maxOpen := 25
	if v := environment.Get("DB_MAX_OPEN_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxOpen = n
		}
	}
	maxIdle := 5
	if v := environment.Get("DB_MAX_IDLE_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxIdle = n
		}
	}
	maxLifetime := 30 * time.Minute
	if v := environment.Get("DB_CONN_MAX_LIFETIME_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxLifetime = time.Duration(n) * time.Second
		}
	}
	maxIdleTime := 5 * time.Minute
	if v := environment.Get("DB_CONN_MAX_IDLE_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxIdleTime = time.Duration(n) * time.Second
		}
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(maxLifetime)
	db.SetConnMaxIdleTime(maxIdleTime)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("database: ping [%s]: %w", driver, err)
	}
	return &DB{DB: db, Driver: driver, stmts: make(map[string]*sql.Stmt)}, nil
}

// PrepareCached prepara (se necessário) e retorna um *sql.Stmt armazenado em cache
// para a query fornecida. A chave do cache é a query completa (após rewrite).
func (db *DB) PrepareCached(query string) (*sql.Stmt, error) {
	db.stmtMu.RLock()
	if s, ok := db.stmts[query]; ok {
		db.stmtMu.RUnlock()
		return s, nil
	}
	db.stmtMu.RUnlock()

	db.stmtMu.Lock()
	defer db.stmtMu.Unlock()
	if db.stmts == nil {
		db.stmts = make(map[string]*sql.Stmt)
	}
	if s, ok := db.stmts[query]; ok {
		return s, nil
	}
	s, err := db.DB.Prepare(query)
	if err != nil {
		return nil, err
	}
	db.stmts[query] = s
	return s, nil
}

// Close fecha statements em cache e a conexão subjacente.
func (db *DB) Close() error {
	if db == nil || db.DB == nil {
		return nil
	}
	db.stmtMu.Lock()
	for _, s := range db.stmts {
		if s != nil {
			_ = s.Close()
		}
	}
	db.stmts = nil
	db.stmtMu.Unlock()
	return db.DB.Close()
}
