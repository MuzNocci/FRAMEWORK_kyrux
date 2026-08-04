package elasticsearch

// Testes de integração real contra um Elasticsearch de verdade (container
// Docker local). Pulados (t.Skip) se o servidor não estiver acessível.

import (
	"context"
	"os"
	"testing"

	"github.com/elastic/go-elasticsearch/v8/esapi"
)

type artigoDoc struct {
	Titulo   string `json:"titulo"`
	Conteudo string `json:"conteudo"`
	Ativo    bool   `json:"ativo"`
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openTestClient(t *testing.T) *Client {
	t.Helper()
	addr := envOr("KYRUX_TEST_ES_ADDR", "http://127.0.0.1:9200")
	c, err := New([]string{addr})
	if err != nil {
		t.Skipf("elasticsearch indisponível em %s: %v", addr, err)
	}
	return c
}

func freshIndex(t *testing.T, c *Client) *Index[artigoDoc] {
	t.Helper()
	ctx := context.Background()
	name := "kyrux_test_artigos"

	// Remove o índice de tentativas anteriores (ignora erro se não existir).
	req := esapi.IndicesDeleteRequest{Index: []string{name}, IgnoreUnavailable: boolPtr(true)}
	res, err := req.Do(ctx, c.Raw())
	if err != nil {
		t.Fatalf("limpar índice de teste: %v", err)
	}
	res.Body.Close()

	return IndexOf[artigoDoc](c, name)
}

func boolPtr(b bool) *bool { return &b }

func TestElasticsearchPutGet(t *testing.T) {
	c := openTestClient(t)
	idx := freshIndex(t, c)
	ctx := context.Background()

	id, err := idx.Put(ctx, "1", &artigoDoc{Titulo: "Kyrux", Conteudo: "framework em Go", Ativo: true})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if id != "1" {
		t.Errorf("esperava id '1', recebeu %q", id)
	}

	got, ok, err := idx.Get(ctx, "1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatal("esperava encontrar o documento")
	}
	if got.Titulo != "Kyrux" || !got.Ativo {
		t.Errorf("documento incorreto: %+v", got)
	}
}

func TestElasticsearchGetInexistente(t *testing.T) {
	c := openTestClient(t)
	idx := freshIndex(t, c)

	_, ok, err := idx.Get(context.Background(), "nao-existe")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ok {
		t.Error("esperava ok=false para documento inexistente")
	}
}

func TestElasticsearchPutAutoID(t *testing.T) {
	c := openTestClient(t)
	idx := freshIndex(t, c)
	ctx := context.Background()

	id, err := idx.Put(ctx, "", &artigoDoc{Titulo: "Sem ID", Conteudo: "gerado automaticamente"})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if id == "" {
		t.Error("esperava um ID gerado automaticamente pelo Elasticsearch")
	}
}

func TestElasticsearchSearch(t *testing.T) {
	c := openTestClient(t)
	idx := freshIndex(t, c)
	ctx := context.Background()

	docs := map[string]*artigoDoc{
		"1": {Titulo: "Introdução ao Go", Conteudo: "Aprenda golang do zero", Ativo: true},
		"2": {Titulo: "ORM em Go", Conteudo: "kyrux usa golang e reflection", Ativo: true},
		"3": {Titulo: "Receita de bolo", Conteudo: "misture os ingredientes", Ativo: false},
	}
	for id, d := range docs {
		if _, err := idx.Put(ctx, id, d); err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
	}
	if err := idx.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	results, err := idx.Search(ctx, map[string]any{
		"query": map[string]any{
			"match": map[string]any{"conteudo": "golang"},
		},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("esperava 2 resultados, recebeu %d: %+v", len(results), results)
	}

	filtered, err := idx.Search(ctx, map[string]any{
		"query": map[string]any{
			"term": map[string]any{"ativo": false},
		},
	})
	if err != nil {
		t.Fatalf("search ativo=false: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Titulo != "Receita de bolo" {
		t.Errorf("esperava só 'Receita de bolo', recebeu %+v", filtered)
	}
}

func TestElasticsearchCount(t *testing.T) {
	c := openTestClient(t)
	idx := freshIndex(t, c)
	ctx := context.Background()

	idx.Put(ctx, "1", &artigoDoc{Titulo: "A", Ativo: true})
	idx.Put(ctx, "2", &artigoDoc{Titulo: "B", Ativo: true})
	idx.Put(ctx, "3", &artigoDoc{Titulo: "C", Ativo: false})
	if err := idx.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	total, err := idx.Count(ctx, nil)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 3 {
		t.Errorf("esperava 3 documentos, recebeu %d", total)
	}

	ativos, err := idx.Count(ctx, map[string]any{"term": map[string]any{"ativo": true}})
	if err != nil {
		t.Fatalf("count ativos: %v", err)
	}
	if ativos != 2 {
		t.Errorf("esperava 2 ativos, recebeu %d", ativos)
	}
}

func TestElasticsearchDelete(t *testing.T) {
	c := openTestClient(t)
	idx := freshIndex(t, c)
	ctx := context.Background()

	idx.Put(ctx, "1", &artigoDoc{Titulo: "Temp"})
	if err := idx.Delete(ctx, "1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, ok, err := idx.Get(ctx, "1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ok {
		t.Error("documento deveria ter sido removido")
	}
}

func TestNewElasticsearchFalhaComEnderecoInvalido(t *testing.T) {
	if _, err := New([]string{"http://127.0.0.1:1"}); err == nil {
		t.Error("esperava erro ao conectar em endereço inválido")
	}
}
