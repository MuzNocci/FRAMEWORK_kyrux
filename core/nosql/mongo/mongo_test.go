package mongo

// Testes de integração real contra um MongoDB de verdade (container Docker
// local). Pulados (t.Skip) se o servidor não estiver acessível, para não
// quebrar em máquinas sem essa infra.

import (
	"context"
	"os"
	"testing"
	"time"
)

type produtoDoc struct {
	Nome  string  `bson:"nome"`
	Preco float64 `bson:"preco"`
	Ativo bool    `bson:"ativo"`
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openTestClient(t *testing.T) *Client {
	t.Helper()
	uri := envOr("KYRUX_TEST_MONGO_URI", "mongodb://127.0.0.1:27018")
	c, err := New(uri, "kyrux_test")
	if err != nil {
		t.Skipf("mongodb indisponível em %s: %v", uri, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Close(ctx)
	})
	return c
}

func freshCollection(t *testing.T, c *Client) *Coll[produtoDoc] {
	t.Helper()
	ctx := context.Background()
	name := "produtos_teste"
	if err := c.db.Collection(name).Drop(ctx); err != nil {
		t.Fatalf("drop collection: %v", err)
	}
	return CollectionOf[produtoDoc](c, name)
}

func TestMongoInsertOneFindOne(t *testing.T) {
	c := openTestClient(t)
	coll := freshCollection(t, c)
	ctx := context.Background()

	if err := coll.InsertOne(ctx, &produtoDoc{Nome: "Caneca", Preco: 29.9, Ativo: true}); err != nil {
		t.Fatalf("insertone: %v", err)
	}

	got, err := coll.FindOne(ctx, M{"nome": "Caneca"})
	if err != nil {
		t.Fatalf("findone: %v", err)
	}
	if got.Preco != 29.9 || !got.Ativo {
		t.Errorf("documento incorreto: %+v", got)
	}
}

func TestMongoFindOneNaoEncontrado(t *testing.T) {
	c := openTestClient(t)
	coll := freshCollection(t, c)

	if _, err := coll.FindOne(context.Background(), M{"nome": "não existe"}); err != ErrNoDocuments {
		t.Errorf("esperava ErrNoDocuments, recebeu %v", err)
	}
}

func TestMongoInsertManyFind(t *testing.T) {
	c := openTestClient(t)
	coll := freshCollection(t, c)
	ctx := context.Background()

	docs := []*produtoDoc{
		{Nome: "A", Preco: 10, Ativo: true},
		{Nome: "B", Preco: 20, Ativo: true},
		{Nome: "C", Preco: 30, Ativo: false},
	}
	if err := coll.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insertmany: %v", err)
	}

	all, err := coll.Find(ctx, M{})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("esperava 3 documentos, recebeu %d", len(all))
	}

	ativos, err := coll.Find(ctx, M{"ativo": true})
	if err != nil {
		t.Fatalf("find ativos: %v", err)
	}
	if len(ativos) != 2 {
		t.Errorf("esperava 2 documentos ativos, recebeu %d", len(ativos))
	}
}

func TestMongoUpdateOneExigeOperador(t *testing.T) {
	c := openTestClient(t)
	coll := freshCollection(t, c)
	ctx := context.Background()

	if err := coll.InsertOne(ctx, &produtoDoc{Nome: "Caneca", Preco: 29.9, Ativo: true}); err != nil {
		t.Fatalf("insertone: %v", err)
	}

	// Update sem operador ($set) deve falhar — mesma regra do driver nativo.
	if _, err := coll.UpdateOne(ctx, M{"nome": "Caneca"}, M{"preco": 39.9}); err == nil {
		t.Error("esperava erro ao atualizar sem operador $set")
	}

	n, err := coll.UpdateOne(ctx, M{"nome": "Caneca"}, M{"$set": M{"preco": 39.9}})
	if err != nil {
		t.Fatalf("updateone: %v", err)
	}
	if n != 1 {
		t.Errorf("esperava 1 documento modificado, recebeu %d", n)
	}

	got, err := coll.FindOne(ctx, M{"nome": "Caneca"})
	if err != nil {
		t.Fatalf("findone: %v", err)
	}
	if got.Preco != 39.9 {
		t.Errorf("esperava preco=39.9 após update, recebeu %v", got.Preco)
	}
}

func TestMongoUpdateManyDeleteManyCount(t *testing.T) {
	c := openTestClient(t)
	coll := freshCollection(t, c)
	ctx := context.Background()

	docs := []*produtoDoc{
		{Nome: "A", Preco: 10, Ativo: false},
		{Nome: "B", Preco: 20, Ativo: false},
		{Nome: "C", Preco: 30, Ativo: true},
	}
	if err := coll.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insertmany: %v", err)
	}

	n, err := coll.UpdateMany(ctx, M{"ativo": false}, M{"$set": M{"ativo": true}})
	if err != nil {
		t.Fatalf("updatemany: %v", err)
	}
	if n != 2 {
		t.Errorf("esperava 2 documentos modificados, recebeu %d", n)
	}

	total, err := coll.Count(ctx, M{"ativo": true})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 3 {
		t.Errorf("esperava 3 ativos após update, recebeu %d", total)
	}

	deleted, err := coll.DeleteMany(ctx, M{"ativo": true})
	if err != nil {
		t.Fatalf("deletemany: %v", err)
	}
	if deleted != 3 {
		t.Errorf("esperava 3 documentos removidos, recebeu %d", deleted)
	}

	remaining, err := coll.Count(ctx, M{})
	if err != nil {
		t.Fatalf("count final: %v", err)
	}
	if remaining != 0 {
		t.Errorf("esperava 0 documentos restantes, recebeu %d", remaining)
	}
}

func TestMongoDeleteOne(t *testing.T) {
	c := openTestClient(t)
	coll := freshCollection(t, c)
	ctx := context.Background()

	if err := coll.InsertMany(ctx, []*produtoDoc{
		{Nome: "A", Preco: 10},
		{Nome: "A", Preco: 20},
	}); err != nil {
		t.Fatalf("insertmany: %v", err)
	}

	n, err := coll.DeleteOne(ctx, M{"nome": "A"})
	if err != nil {
		t.Fatalf("deleteone: %v", err)
	}
	if n != 1 {
		t.Errorf("esperava 1 documento removido, recebeu %d", n)
	}

	remaining, err := coll.Count(ctx, M{"nome": "A"})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 1 {
		t.Errorf("esperava 1 documento restante, recebeu %d", remaining)
	}
}

func TestNewMongoFalhaComURIInvalida(t *testing.T) {
	if _, err := New("mongodb://127.0.0.1:1/?connectTimeoutMS=500&serverSelectionTimeoutMS=1000", "x"); err == nil {
		t.Error("esperava erro ao conectar em endereço inválido")
	}
}
