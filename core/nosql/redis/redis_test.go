package redis

// Testes de integração real contra um Redis de verdade (container Docker
// dedicado, separado do redis-cache/redis-queue reais do projeto). Pulados
// (t.Skip) se o servidor não estiver acessível.

import (
	"context"
	"os"
	"testing"
	"time"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openTestClient(t *testing.T) *Client {
	t.Helper()
	addr := envOr("KYRUX_TEST_REDIS_ADDR", "127.0.0.1:6390")
	c, err := New(addr, "", 0)
	if err != nil {
		t.Skipf("redis indisponível em %s: %v", addr, err)
	}
	ctx := context.Background()
	if err := c.raw.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushdb: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestRedisSetGet(t *testing.T) {
	c := openTestClient(t)
	ctx := context.Background()

	if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "v" {
		t.Errorf("esperava 'v', recebeu %q", got)
	}
}

func TestRedisGetInexistente(t *testing.T) {
	c := openTestClient(t)
	if _, err := c.Get(context.Background(), "nao-existe"); err != ErrNil {
		t.Errorf("esperava ErrNil, recebeu %v", err)
	}
}

func TestRedisSetJSONGetJSON(t *testing.T) {
	c := openTestClient(t)
	ctx := context.Background()

	type Produto struct {
		Nome  string
		Preco float64
	}
	want := Produto{Nome: "Caneca", Preco: 29.9}
	if err := c.SetJSON(ctx, "produto:1", want, time.Minute); err != nil {
		t.Fatalf("setjson: %v", err)
	}

	var got Produto
	if err := c.GetJSON(ctx, "produto:1", &got); err != nil {
		t.Fatalf("getjson: %v", err)
	}
	if got != want {
		t.Errorf("esperava %+v, recebeu %+v", want, got)
	}
}

func TestRedisDelExistsExpireTTL(t *testing.T) {
	c := openTestClient(t)
	ctx := context.Background()

	c.Set(ctx, "a", "1", 0)
	c.Set(ctx, "b", "2", 0)

	n, err := c.Exists(ctx, "a", "b", "c")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if n != 2 {
		t.Errorf("esperava 2 chaves existentes, recebeu %d", n)
	}

	ok, err := c.Expire(ctx, "a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("expire: ok=%v err=%v", ok, err)
	}
	ttl, err := c.TTL(ctx, "a")
	if err != nil {
		t.Fatalf("ttl: %v", err)
	}
	if ttl <= 0 || ttl > time.Minute {
		t.Errorf("ttl fora do esperado: %v", ttl)
	}

	deleted, err := c.Del(ctx, "a", "b")
	if err != nil {
		t.Fatalf("del: %v", err)
	}
	if deleted != 2 {
		t.Errorf("esperava 2 chaves removidas, recebeu %d", deleted)
	}
}

func TestRedisHash(t *testing.T) {
	c := openTestClient(t)
	ctx := context.Background()

	if err := c.HSet(ctx, "user:1", map[string]any{"nome": "Ana", "idade": 30}); err != nil {
		t.Fatalf("hset: %v", err)
	}
	nome, err := c.HGet(ctx, "user:1", "nome")
	if err != nil {
		t.Fatalf("hget: %v", err)
	}
	if nome != "Ana" {
		t.Errorf("esperava 'Ana', recebeu %q", nome)
	}

	all, err := c.HGetAll(ctx, "user:1")
	if err != nil {
		t.Fatalf("hgetall: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("esperava 2 campos, recebeu %d: %v", len(all), all)
	}

	n, err := c.HDel(ctx, "user:1", "idade")
	if err != nil {
		t.Fatalf("hdel: %v", err)
	}
	if n != 1 {
		t.Errorf("esperava 1 campo removido, recebeu %d", n)
	}
}

func TestRedisLista(t *testing.T) {
	c := openTestClient(t)
	ctx := context.Background()

	if _, err := c.RPush(ctx, "fila", "a", "b", "c"); err != nil {
		t.Fatalf("rpush: %v", err)
	}
	items, err := c.LRange(ctx, "fila", 0, -1)
	if err != nil {
		t.Fatalf("lrange: %v", err)
	}
	if len(items) != 3 || items[0] != "a" || items[2] != "c" {
		t.Errorf("esperava [a b c], recebeu %v", items)
	}

	n, err := c.LLen(ctx, "fila")
	if err != nil {
		t.Fatalf("llen: %v", err)
	}
	if n != 3 {
		t.Errorf("esperava tamanho 3, recebeu %d", n)
	}

	first, err := c.LPop(ctx, "fila")
	if err != nil || first != "a" {
		t.Errorf("lpop: esperava 'a', recebeu %q (err=%v)", first, err)
	}
	last, err := c.RPop(ctx, "fila")
	if err != nil || last != "c" {
		t.Errorf("rpop: esperava 'c', recebeu %q (err=%v)", last, err)
	}
}

func TestRedisConjunto(t *testing.T) {
	c := openTestClient(t)
	ctx := context.Background()

	n, err := c.SAdd(ctx, "tags", "go", "web", "go")
	if err != nil {
		t.Fatalf("sadd: %v", err)
	}
	if n != 2 {
		t.Errorf("esperava 2 membros novos (dedup), recebeu %d", n)
	}

	ok, err := c.SIsMember(ctx, "tags", "go")
	if err != nil || !ok {
		t.Errorf("sismember: esperava true, recebeu %v (err=%v)", ok, err)
	}

	members, err := c.SMembers(ctx, "tags")
	if err != nil {
		t.Fatalf("smembers: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("esperava 2 membros, recebeu %d: %v", len(members), members)
	}

	card, err := c.SCard(ctx, "tags")
	if err != nil || card != 2 {
		t.Errorf("scard: esperava 2, recebeu %d (err=%v)", card, err)
	}

	removed, err := c.SRem(ctx, "tags", "go")
	if err != nil || removed != 1 {
		t.Errorf("srem: esperava 1, recebeu %d (err=%v)", removed, err)
	}
}

func TestRedisConjuntoOrdenado(t *testing.T) {
	c := openTestClient(t)
	ctx := context.Background()

	n, err := c.ZAdd(ctx, "ranking",
		ZMember{Score: 10, Member: "joao"},
		ZMember{Score: 30, Member: "maria"},
		ZMember{Score: 20, Member: "pedro"},
	)
	if err != nil {
		t.Fatalf("zadd: %v", err)
	}
	if n != 3 {
		t.Errorf("esperava 3 membros novos, recebeu %d", n)
	}

	ordered, err := c.ZRange(ctx, "ranking", 0, -1)
	if err != nil {
		t.Fatalf("zrange: %v", err)
	}
	want := []string{"joao", "pedro", "maria"} // ordem crescente de score
	if len(ordered) != len(want) {
		t.Fatalf("esperava %v, recebeu %v", want, ordered)
	}
	for i := range want {
		if ordered[i] != want[i] {
			t.Errorf("posição %d: esperava %q, recebeu %q", i, want[i], ordered[i])
		}
	}

	score, err := c.ZScore(ctx, "ranking", "maria")
	if err != nil || score != 30 {
		t.Errorf("zscore: esperava 30, recebeu %v (err=%v)", score, err)
	}

	removed, err := c.ZRem(ctx, "ranking", "joao")
	if err != nil || removed != 1 {
		t.Errorf("zrem: esperava 1, recebeu %d (err=%v)", removed, err)
	}
}

func TestRedisPubSub(t *testing.T) {
	c := openTestClient(t)
	ctx := context.Background()

	sub := c.Subscribe(ctx, "canal-teste")
	defer sub.Close()

	// Aguarda a assinatura ser confirmada pelo servidor antes de publicar
	// (sem isso, a mensagem pode ser publicada antes do subscribe completar).
	if _, err := sub.raw.Receive(ctx); err != nil {
		t.Fatalf("receive (subscribe confirmation): %v", err)
	}

	n, err := c.Publish(ctx, "canal-teste", "ola mundo")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 1 {
		t.Errorf("esperava 1 subscriber recebendo, recebeu %d", n)
	}

	select {
	case msg := <-sub.Channel():
		if msg.Payload != "ola mundo" {
			t.Errorf("esperava 'ola mundo', recebeu %q", msg.Payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout esperando mensagem do pub/sub")
	}
}

func TestNewRedisFalhaComEnderecoInvalido(t *testing.T) {
	if _, err := New("127.0.0.1:1", "", 0); err == nil {
		t.Error("esperava erro ao conectar em endereço inválido")
	}
}
