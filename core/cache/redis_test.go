package cache

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func newTestRedisCache(t *testing.T) *Cache {
	c, _ := newTestRedisCacheWithServer(t)
	return c
}

// newTestRedisCacheWithServer também devolve o *miniredis.Miniredis
// subjacente, necessário para simular a passagem do tempo via FastForward
// (miniredis não expira chaves por TTL sozinho — precisa ser avisado).
func newTestRedisCacheWithServer(t *testing.T) (*Cache, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	c, err := NewRedis(mr.Addr(), "")
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	t.Cleanup(c.Close)
	return c, mr
}

func TestRedisSetGet(t *testing.T) {
	c := newTestRedisCache(t)

	c.Set("chave", "valor", time.Minute)
	v, ok := c.Get("chave")
	if !ok {
		t.Fatal("esperava encontrar a chave")
	}
	if v != "valor" {
		t.Errorf("esperava 'valor', recebeu %v", v)
	}
}

func TestRedisGetInexistente(t *testing.T) {
	c := newTestRedisCache(t)
	if _, ok := c.Get("nao-existe"); ok {
		t.Error("chave inexistente não deveria ser encontrada")
	}
}

func TestRedisTTLExpira(t *testing.T) {
	c, mr := newTestRedisCacheWithServer(t)
	c.Set("expira", "valor", 200*time.Millisecond)
	mr.FastForward(300 * time.Millisecond)
	if _, ok := c.Get("expira"); ok {
		t.Error("chave deveria ter expirado")
	}
}

func TestRedisDelete(t *testing.T) {
	c := newTestRedisCache(t)
	c.Set("chave", "valor", time.Minute)
	c.Delete("chave")
	if _, ok := c.Get("chave"); ok {
		t.Error("chave deveria ter sido removida")
	}
}

func TestRedisLen(t *testing.T) {
	c := newTestRedisCache(t)
	c.Set("a", 1, time.Minute)
	c.Set("b", 2, time.Minute)
	if n := c.Len(); n != 2 {
		t.Errorf("esperava Len()=2, recebeu %d", n)
	}
}

// TestRedisValorComplexoVoltaComoMap documenta a diferença de semântica em
// relação ao modo memória: um struct salvo via Set volta de Get como
// map[string]any (decodificado do JSON), não no tipo original.
func TestRedisValorComplexoVoltaComoMap(t *testing.T) {
	c := newTestRedisCache(t)

	type Produto struct {
		Nome  string
		Preco float64
	}
	c.Set("produto", Produto{Nome: "Caneca", Preco: 29.9}, time.Minute)

	v, ok := c.Get("produto")
	if !ok {
		t.Fatal("esperava encontrar a chave")
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("esperava map[string]any (JSON decodificado), recebeu %T", v)
	}
	if m["Nome"] != "Caneca" {
		t.Errorf("esperava Nome=Caneca, recebeu %v", m["Nome"])
	}
}

func TestRedisValorNaoSerializavelNaoQuebra(t *testing.T) {
	c := newTestRedisCache(t)
	// canais não são serializáveis em JSON — Set deve apenas logar e ignorar,
	// nunca panicar.
	c.Set("chave", make(chan int), time.Minute)
	if _, ok := c.Get("chave"); ok {
		t.Error("valor não serializável não deveria ter sido gravado")
	}
}

func TestNewRedisFalhaComEnderecoInvalido(t *testing.T) {
	if _, err := NewRedis("127.0.0.1:1", ""); err == nil {
		t.Error("esperava erro ao conectar em endereço inválido")
	}
}

func TestRedisComSenhaCorreta(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	mr.RequireAuth("segredo-forte")

	c, err := NewRedis(mr.Addr(), "segredo-forte")
	if err != nil {
		t.Fatalf("NewRedis com senha correta não deveria falhar: %v", err)
	}
	defer c.Close()

	c.Set("k", "v", time.Minute)
	if v, ok := c.Get("k"); !ok || v != "v" {
		t.Errorf("esperava ('v', true), recebeu (%v, %v)", v, ok)
	}
}

func TestRedisComSenhaErrada(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	mr.RequireAuth("segredo-forte")

	if _, err := NewRedis(mr.Addr(), "senha-errada"); err == nil {
		t.Error("esperava erro de autenticação com senha errada")
	}
}
