package cache

import (
	"sync"
	"time"
)

type entry struct {
	value   any
	expires time.Time
}

type Cache struct {
	mu      sync.RWMutex
	entries map[string]entry
}

func New() *Cache {
	c := &Cache{entries: make(map[string]entry)}
	go c.gc()
	return c
}

func (c *Cache) Set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	c.entries[key] = entry{value: value, expires: time.Now().Add(ttl)}
	c.mu.Unlock()
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

func (c *Cache) gc() {
	for range time.Tick(time.Minute) {
		c.mu.Lock()
		for k, e := range c.entries {
			if time.Now().After(e.expires) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}
