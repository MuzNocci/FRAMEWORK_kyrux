// Package redis é um client Redis de propósito geral — não confundir com
// core/cache (Cache) e core/queue (Queue), que já usam Redis internamente
// mas só como armazenamento chave/valor com TTL e fila, respectivamente.
// Este pacote expõe as estruturas de dados reais do Redis (hash, lista,
// conjunto, conjunto ordenado, pub/sub) para uso como banco de dados de
// propósito geral, não só cache.
//
// Como MongoDB (core/nosql/mongo), este pacote NÃO é importado por
// core/bootstrap — o driver só entra no binário se você mesmo importar
// kyrux/core/nosql/redis em algum lugar do seu código. Isso é deliberado:
// evita pesar o binário de projetos que não usam Redis como banco (embora
// go-redis já esteja no binário se você usa Cache/Queue com driver redis —
// nesse caso este pacote reaproveita a mesma dependência, sem custo extra).
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// ErrNil é devolvido quando uma chave/campo não existe — equivalente ao
// sql.ErrNoRows do ORM relacional e ao mongo.ErrNoDocuments do client Mongo.
var ErrNil = goredis.Nil

// Client é uma conexão com um servidor Redis usado como banco de dados de
// propósito geral (não como cache/fila).
type Client struct {
	raw *goredis.Client
}

// New conecta a um servidor Redis em addr (ex: "127.0.0.1:6379"). password é
// opcional (vazio = sem AUTH); db é o índice do banco lógico (0-15 por
// padrão no Redis). Faz PING antes de retornar.
func New(addr, password string, db int) (*Client, error) {
	raw := goredis.NewClient(&goredis.Options{Addr: addr, Password: password, DB: db, MaxRetries: -1})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := raw.Ping(ctx).Err(); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("redis: conectar em %s: %w", addr, err)
	}
	return &Client{raw: raw}, nil
}

// Close encerra a conexão.
func (c *Client) Close() error { return c.raw.Close() }

// Raw devolve o *redis.Client nativo do driver oficial (go-redis) — escape
// hatch para scripts Lua, pipelines, transações (MULTI/EXEC) e qualquer
// coisa que este wrapper não cobre.
func (c *Client) Raw() *goredis.Client { return c.raw }

// ── Strings / chaves genéricas ───────────────────────────────────────────────

// Set grava value como string. ttl <= 0 significa sem expiração.
func (c *Client) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := c.raw.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis: set: %w", err)
	}
	return nil
}

// Get lê o valor de key. Retorna ErrNil se a chave não existir.
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	v, err := c.raw.Get(ctx, key).Result()
	if err != nil {
		return "", err
	}
	return v, nil
}

// SetJSON serializa value em JSON e grava — conveniência para guardar
// structs/slices/maps sem se preocupar em serializar manualmente.
func (c *Client) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("redis: setjson: %w", err)
	}
	return c.Set(ctx, key, string(data), ttl)
}

// GetJSON lê key e decodifica o JSON em dest (um ponteiro). Retorna ErrNil
// se a chave não existir.
func (c *Client) GetJSON(ctx context.Context, key string, dest any) error {
	v, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(v), dest); err != nil {
		return fmt.Errorf("redis: getjson: %w", err)
	}
	return nil
}

// Del remove uma ou mais chaves. Retorna quantas existiam e foram removidas.
func (c *Client) Del(ctx context.Context, keys ...string) (int64, error) {
	n, err := c.raw.Del(ctx, keys...).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: del: %w", err)
	}
	return n, nil
}

// Exists conta quantas das chaves informadas existem.
func (c *Client) Exists(ctx context.Context, keys ...string) (int64, error) {
	n, err := c.raw.Exists(ctx, keys...).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: exists: %w", err)
	}
	return n, nil
}

// Expire define um TTL numa chave já existente. Retorna false se a chave não existir.
func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	ok, err := c.raw.Expire(ctx, key, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis: expire: %w", err)
	}
	return ok, nil
}

// TTL retorna o tempo restante até a expiração. -1 = sem expiração, -2 = chave não existe.
func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	d, err := c.raw.TTL(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: ttl: %w", err)
	}
	return d, nil
}

// ── Hash ──────────────────────────────────────────────────────────────────

// HSet grava um ou mais campos de um hash.
func (c *Client) HSet(ctx context.Context, key string, values map[string]any) error {
	if err := c.raw.HSet(ctx, key, values).Err(); err != nil {
		return fmt.Errorf("redis: hset: %w", err)
	}
	return nil
}

// HGet lê um campo de um hash. Retorna ErrNil se o campo (ou a chave) não existir.
func (c *Client) HGet(ctx context.Context, key, field string) (string, error) {
	v, err := c.raw.HGet(ctx, key, field).Result()
	if err != nil {
		return "", err
	}
	return v, nil
}

// HGetAll lê todos os campos de um hash. Mapa vazio se a chave não existir.
func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	m, err := c.raw.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: hgetall: %w", err)
	}
	return m, nil
}

// HDel remove um ou mais campos de um hash.
func (c *Client) HDel(ctx context.Context, key string, fields ...string) (int64, error) {
	n, err := c.raw.HDel(ctx, key, fields...).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: hdel: %w", err)
	}
	return n, nil
}

// ── Lista ─────────────────────────────────────────────────────────────────

// LPush insere um ou mais valores no início da lista. Retorna o tamanho final.
func (c *Client) LPush(ctx context.Context, key string, values ...any) (int64, error) {
	n, err := c.raw.LPush(ctx, key, values...).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: lpush: %w", err)
	}
	return n, nil
}

// RPush insere um ou mais valores no final da lista. Retorna o tamanho final.
func (c *Client) RPush(ctx context.Context, key string, values ...any) (int64, error) {
	n, err := c.raw.RPush(ctx, key, values...).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: rpush: %w", err)
	}
	return n, nil
}

// LRange retorna o intervalo [start, stop] da lista (inclusive; -1 = último elemento).
func (c *Client) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	items, err := c.raw.LRange(ctx, key, start, stop).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: lrange: %w", err)
	}
	return items, nil
}

// LPop remove e retorna o primeiro elemento da lista. Retorna ErrNil se vazia.
func (c *Client) LPop(ctx context.Context, key string) (string, error) {
	v, err := c.raw.LPop(ctx, key).Result()
	if err != nil {
		return "", err
	}
	return v, nil
}

// RPop remove e retorna o último elemento da lista. Retorna ErrNil se vazia.
func (c *Client) RPop(ctx context.Context, key string) (string, error) {
	v, err := c.raw.RPop(ctx, key).Result()
	if err != nil {
		return "", err
	}
	return v, nil
}

// LLen retorna o tamanho da lista (0 se não existir).
func (c *Client) LLen(ctx context.Context, key string) (int64, error) {
	n, err := c.raw.LLen(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: llen: %w", err)
	}
	return n, nil
}

// ── Conjunto (Set) ───────────────────────────────────────────────────────────

// SAdd adiciona um ou mais membros a um conjunto. Retorna quantos foram
// adicionados de fato (membros já existentes não contam).
func (c *Client) SAdd(ctx context.Context, key string, members ...any) (int64, error) {
	n, err := c.raw.SAdd(ctx, key, members...).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: sadd: %w", err)
	}
	return n, nil
}

// SMembers retorna todos os membros de um conjunto (ordem não garantida).
func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	m, err := c.raw.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: smembers: %w", err)
	}
	return m, nil
}

// SIsMember reporta se member pertence ao conjunto.
func (c *Client) SIsMember(ctx context.Context, key string, member any) (bool, error) {
	ok, err := c.raw.SIsMember(ctx, key, member).Result()
	if err != nil {
		return false, fmt.Errorf("redis: sismember: %w", err)
	}
	return ok, nil
}

// SRem remove um ou mais membros de um conjunto. Retorna quantos foram removidos.
func (c *Client) SRem(ctx context.Context, key string, members ...any) (int64, error) {
	n, err := c.raw.SRem(ctx, key, members...).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: srem: %w", err)
	}
	return n, nil
}

// SCard retorna o número de membros de um conjunto.
func (c *Client) SCard(ctx context.Context, key string) (int64, error) {
	n, err := c.raw.SCard(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: scard: %w", err)
	}
	return n, nil
}

// ── Conjunto ordenado (Sorted Set) ───────────────────────────────────────────

// ZMember é um membro com sua pontuação — usado por ZAdd.
type ZMember struct {
	Score  float64
	Member any
}

// ZAdd adiciona um ou mais membros com pontuação a um conjunto ordenado.
// Retorna quantos membros novos foram adicionados (atualizar a pontuação de
// um membro existente não conta).
func (c *Client) ZAdd(ctx context.Context, key string, members ...ZMember) (int64, error) {
	zs := make([]goredis.Z, len(members))
	for i, m := range members {
		zs[i] = goredis.Z{Score: m.Score, Member: m.Member}
	}
	n, err := c.raw.ZAdd(ctx, key, zs...).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: zadd: %w", err)
	}
	return n, nil
}

// ZRange retorna os membros no intervalo [start, stop] por posição (ordem
// crescente de pontuação; -1 = último). Para paginação tipo "top N", use
// ZRange(ctx, key, 0, N-1) — o Redis já devolve em ordem crescente de score.
func (c *Client) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	items, err := c.raw.ZRange(ctx, key, start, stop).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: zrange: %w", err)
	}
	return items, nil
}

// ZScore retorna a pontuação de member. Retorna ErrNil se o membro não existir.
func (c *Client) ZScore(ctx context.Context, key, member string) (float64, error) {
	s, err := c.raw.ZScore(ctx, key, member).Result()
	if err != nil {
		return 0, err
	}
	return s, nil
}

// ZRem remove um ou mais membros de um conjunto ordenado.
func (c *Client) ZRem(ctx context.Context, key string, members ...any) (int64, error) {
	n, err := c.raw.ZRem(ctx, key, members...).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: zrem: %w", err)
	}
	return n, nil
}

// ── Pub/Sub ───────────────────────────────────────────────────────────────

// Publish envia message (serializado com fmt.Sprint) para channel. Retorna
// quantos subscribers ativos receberam a mensagem.
func (c *Client) Publish(ctx context.Context, channel string, message any) (int64, error) {
	n, err := c.raw.Publish(ctx, channel, message).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: publish: %w", err)
	}
	return n, nil
}

// PubSub é uma assinatura ativa em um ou mais canais.
type PubSub struct {
	raw *goredis.PubSub
}

// Subscribe assina um ou mais canais. Chame Close quando terminar.
func (c *Client) Subscribe(ctx context.Context, channels ...string) *PubSub {
	return &PubSub{raw: c.raw.Subscribe(ctx, channels...)}
}

// Channel devolve o canal Go que recebe as mensagens publicadas.
func (p *PubSub) Channel() <-chan *goredis.Message { return p.raw.Channel() }

// Close encerra a assinatura.
func (p *PubSub) Close() error { return p.raw.Close() }
