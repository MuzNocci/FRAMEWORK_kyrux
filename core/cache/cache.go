package cache

import (
	"sync"
	"time"
)

// DefaultMaxEntries limita o número de chaves no cache. Sem teto, um padrão
// de chave por usuário/por query cresce sem limite (DoS de memória lento).
const DefaultMaxEntries = 100_000

type entry struct {
	value   any
	expires time.Time
}

type Cache struct {
	mu         sync.RWMutex
	entries    map[string]entry
	maxEntries int
	stop       chan struct{}
	stopOnce   sync.Once
}

func New() *Cache {
	c := &Cache{
		entries:    make(map[string]entry),
		maxEntries: DefaultMaxEntries,
		stop:       make(chan struct{}),
	}
	go c.gc()
	return c
}

// SetMaxEntries ajusta o teto de chaves (ex: via config).
func (c *Cache) SetMaxEntries(n int) {
	c.mu.Lock()
	c.maxEntries = n
	c.mu.Unlock()
}

func (c *Cache) Set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxEntries {
		c.evictLocked()
	}
	c.entries[key] = entry{value: value, expires: time.Now().Add(ttl)}
	c.mu.Unlock()
}

// evictLocked abre espaço quando o cache está cheio (chamar com lock):
// primeiro remove expiradas; se nada expirou, remove uma entrada arbitrária.
func (c *Cache) evictLocked() {
	now := time.Now()
	removed := false
	for k, e := range c.entries {
		if now.After(e.expires) {
			delete(c.entries, k)
			removed = true
		}
	}
	if !removed {
		for k := range c.entries {
			delete(c.entries, k)
			break
		}
	}
}

func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Len retorna o número de entradas atualmente no cache (incluindo expiradas ainda não coletadas).
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Close encerra a goroutine de GC. Idempotente.
func (c *Cache) Close() {
	c.stopOnce.Do(func() { close(c.stop) })
}

func (c *Cache) gc() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for k, e := range c.entries {
				if now.After(e.expires) {
					delete(c.entries, k)
				}
			}
			c.mu.Unlock()
		case <-c.stop:
			return
		}
	}
}
