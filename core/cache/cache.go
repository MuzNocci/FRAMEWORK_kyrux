package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultMaxEntries limita o número de chaves no cache em memória. Sem teto,
// um padrão de chave por usuário/por query cresce sem limite (DoS de memória
// lento). Não se aplica ao modo Redis, que já tem sua própria política de
// memória (maxmemory + eviction, configurada no servidor).
const DefaultMaxEntries = 100_000

type entry struct {
	value   any
	expires time.Time
}

// Cache é chave/valor com TTL, em dois modos possíveis:
//   - memória (New): mapa local ao processo, guarda o valor Go original por
//     referência — Get devolve exatamente o que foi passado a Set.
//   - Redis (NewRedis): chaves compartilhadas entre todas as réplicas do
//     processo, mas os valores passam por json.Marshal/Unmarshal — um struct
//     salvo com Set volta de Get como map[string]any, não no tipo original
//     (limitação inerente a serializar para fora do processo). Prefira tipos
//     simples (string, número, slice, map) neste modo, ou decodifique você
//     mesmo com sua própria struct-alvo a partir do map retornado.
type Cache struct {
	mu         sync.RWMutex
	entries    map[string]entry
	maxEntries int
	stop       chan struct{}
	stopOnce   sync.Once

	// redis != nil ativa o modo Redis — todos os métodos abaixo checam esse
	// campo primeiro e desviam para rdb* antes de tocar em entries/mu.
	redis    *redis.Client
	redisCtx context.Context
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

// NewRedis conecta a um servidor Redis em addr (ex: "127.0.0.1:6379") e o
// usa como backend do cache. password é opcional (vazio = sem AUTH, para
// servidores sem requirepass). Faz PING antes de retornar — se o servidor
// não responder, retorna erro para o chamador decidir (o bootstrap cai para
// memória e loga um aviso, em vez de recusar subir por causa de um cache
// indisponível).
func NewRedis(addr, password string) (*Cache, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: password, MaxRetries: -1})

	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		rdb.Close()
		return nil, fmt.Errorf("cache: conectar ao redis em %s: %w", addr, err)
	}
	return &Cache{redis: rdb, redisCtx: context.Background()}, nil
}

// SetMaxEntries ajusta o teto de chaves do modo memória (sem efeito em modo Redis).
func (c *Cache) SetMaxEntries(n int) {
	c.mu.Lock()
	c.maxEntries = n
	c.mu.Unlock()
}

func (c *Cache) Set(key string, value any, ttl time.Duration) {
	if c.redis != nil {
		data, err := json.Marshal(value)
		if err != nil {
			log.Printf("cache: valor para %q não é serializável em JSON (modo redis): %v\n", key, err)
			return
		}
		if err := c.redis.Set(c.redisCtx, key, data, ttl).Err(); err != nil {
			log.Printf("cache: falha ao gravar %q no redis: %v\n", key, err)
		}
		return
	}

	c.mu.Lock()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxEntries {
		c.evictLocked()
	}
	c.entries[key] = entry{value: value, expires: time.Now().Add(ttl)}
	c.mu.Unlock()
}

// evictLocked abre espaço quando o cache em memória está cheio (chamar com lock):
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
	if c.redis != nil {
		data, err := c.redis.Get(c.redisCtx, key).Bytes()
		if err != nil {
			return nil, false
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			log.Printf("cache: valor de %q no redis não é JSON válido: %v\n", key, err)
			return nil, false
		}
		return v, true
	}

	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (c *Cache) Delete(key string) {
	if c.redis != nil {
		if err := c.redis.Del(c.redisCtx, key).Err(); err != nil {
			log.Printf("cache: falha ao remover %q do redis: %v\n", key, err)
		}
		return
	}

	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Len retorna o número de entradas no cache. Em modo memória, inclui
// expiradas ainda não coletadas pelo GC. Em modo Redis, é o DBSIZE do banco
// conectado — preciso apenas se o Redis for dedicado ao cache (não
// compartilhado com outro uso).
func (c *Cache) Len() int {
	if c.redis != nil {
		n, err := c.redis.DBSize(c.redisCtx).Result()
		if err != nil {
			return 0
		}
		return int(n)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Close encerra a conexão (modo Redis) ou a goroutine de GC (modo memória). Idempotente.
func (c *Cache) Close() {
	if c.redis != nil {
		c.redis.Close()
		return
	}
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
