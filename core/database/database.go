package database

import (
	"container/list"
	"context"
	"database/sql"
	"fmt"
	"kyrux/core/environment"
	"strconv"
	"sync"
	"time"
)

const maxCachedStmts = 512

// stmtCache é um cache LRU de prepared statements. Ao evictar, o statement
// é fechado — sem isso, statements órfãos acumulam no servidor de banco.
type stmtCache struct {
	mu    sync.Mutex
	items map[string]*list.Element
	order *list.List // frente = usado mais recentemente
}

type stmtEntry struct {
	key  string
	stmt *sql.Stmt
}

func newStmtCache() *stmtCache {
	return &stmtCache{items: make(map[string]*list.Element), order: list.New()}
}

func (c *stmtCache) get(key string) *sql.Stmt {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*stmtEntry).stmt
	}
	return nil
}

// put insere o statement e retorna os que devem ser fechados pelo caller:
// o evictado por capacidade e/ou o próprio s, se outro goroutine já tiver
// cacheado a mesma query (nesse caso `use` é o statement existente).
func (c *stmtCache) put(key string, s *sql.Stmt) (use *sql.Stmt, toClose []*sql.Stmt) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*stmtEntry).stmt, []*sql.Stmt{s}
	}
	c.items[key] = c.order.PushFront(&stmtEntry{key: key, stmt: s})
	if c.order.Len() > maxCachedStmts {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		e := oldest.Value.(*stmtEntry)
		delete(c.items, e.key)
		toClose = append(toClose, e.stmt)
	}
	return s, toClose
}

func (c *stmtCache) closeAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, el := range c.items {
		if s := el.Value.(*stmtEntry).stmt; s != nil {
			_ = s.Close()
		}
	}
	c.items = make(map[string]*list.Element)
	c.order.Init()
}

// DB encapsula *sql.DB com o nome do driver e um schema opcional.
type DB struct {
	*sql.DB
	Driver string
	Schema string
	stmts  *stmtCache
}

// WithSchema retorna uma cópia de DB com o schema definido — útil para multi-tenant.
// O cache de statements é compartilhado: pertence à conexão, não ao schema.
//
//	db := fw.DB.Use().WithSchema("tenant_abc")
func (db *DB) WithSchema(schema string) *DB {
	return &DB{DB: db.DB, Driver: db.Driver, Schema: schema, stmts: db.stmts}
}

// Tx embrulha *sql.Tx com o driver e o schema da conexão de origem,
// permitindo usar o ORM dentro da transação (orm.FromTx / orm.CreateTx).
// Os métodos de *sql.Tx (Exec, Query, QueryRow...) continuam disponíveis.
type Tx struct {
	*sql.Tx
	Driver string
	Schema string
}

// Transaction executa fn dentro de uma transação: commit se fn devolver nil,
// rollback caso contrário — inclusive em panic (que é re-propagado).
//
//	err := fw.DB.Use().Transaction(func(tx *database.Tx) error {
//	    if err := orm.CreateTx(tx, &pedido); err != nil { return err }
//	    return orm.FromTx[Saldo](tx).Where("id = ?", 1).Update(map[string]any{"valor": novo})
//	})
func (db *DB) Transaction(fn func(tx *Tx) error) error {
	sqlTx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			sqlTx.Rollback()
			panic(r)
		}
	}()
	if err := fn(&Tx{Tx: sqlTx, Driver: db.Driver, Schema: db.Schema}); err != nil {
		sqlTx.Rollback()
		return err
	}
	return sqlTx.Commit()
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
	return &DB{DB: db, Driver: driver, stmts: newStmtCache()}, nil
}

// PrepareCached prepara (se necessário) e retorna um *sql.Stmt do cache LRU.
// A chave do cache é a query completa (após rewrite). Ao atingir o limite,
// o statement menos usado é evictado e fechado.
func (db *DB) PrepareCached(query string) (*sql.Stmt, error) {
	if db.stmts == nil {
		return db.DB.Prepare(query)
	}
	if s := db.stmts.get(query); s != nil {
		return s, nil
	}
	s, err := db.DB.Prepare(query)
	if err != nil {
		return nil, err
	}
	use, toClose := db.stmts.put(query, s)
	for _, old := range toClose {
		_ = old.Close()
	}
	return use, nil
}

// Close fecha statements em cache e a conexão subjacente.
func (db *DB) Close() error {
	if db == nil || db.DB == nil {
		return nil
	}
	if db.stmts != nil {
		db.stmts.closeAll()
	}
	return db.DB.Close()
}
