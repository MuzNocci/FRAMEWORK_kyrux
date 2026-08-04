// Package elasticsearch é um client Elasticsearch idiomático para o Kyrux —
// não faz parte do ORM relacional (core/orm). Elasticsearch é um motor de
// busca/documentos: índices sem schema fixo (mapping dinâmico), consultas
// via Query DSL em JSON (não SQL), sem transações ACID, e busca em "quase
// tempo real" — um documento gravado só aparece na busca depois do próximo
// refresh do índice (~1s por padrão, configurável). Não existe JOIN nem
// WHERE no sentido relacional. Tentar encaixar isso no Query[T] fingiria
// uma portabilidade que não existe.
//
// Como os outros clients NoSQL (core/nosql/mongo, core/nosql/redis,
// core/nosql/cassandra), este pacote NÃO é importado por core/bootstrap —
// o driver oficial (github.com/elastic/go-elasticsearch/v8) só entra no
// binário se você mesmo importar kyrux/core/nosql/elasticsearch.
package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	es "github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// Client é uma conexão com um cluster Elasticsearch.
type Client struct {
	raw *es.Client
}

// New conecta a um cluster Elasticsearch em addresses (ex:
// []string{"http://localhost:9200"}). Faz um Info() antes de retornar —
// se o servidor não responder, retorna erro para o chamador decidir.
func New(addresses []string) (*Client, error) {
	raw, err := es.NewClient(es.Config{Addresses: addresses})
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: criar client: %w", err)
	}

	res, err := raw.Info()
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: conectar em %v: %w", addresses, err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch: conectar em %v: %s", addresses, res.String())
	}

	return &Client{raw: raw}, nil
}

// Raw devolve o *elasticsearch.Client nativo do driver oficial — escape
// hatch para agregações, bulk API, ILM e qualquer coisa que este wrapper
// não cobre.
func (c *Client) Raw() *es.Client { return c.raw }

// Index é um índice Elasticsearch tipado — T é a struct Go que representa o
// documento (tags `json:"..."` controlam os nomes de campo no _source).
type Index[T any] struct {
	client *Client
	name   string
}

// IndexOf abre o índice name, tipado para T.
func IndexOf[T any](c *Client, name string) *Index[T] {
	return &Index[T]{client: c, name: name}
}

func readBody(res *esapi.Response) ([]byte, error) {
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: ler resposta: %w", err)
	}
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch: %s: %s", res.Status(), string(body))
	}
	return body, nil
}

// Put indexa (cria ou substitui) um documento. Com id vazio, o Elasticsearch
// gera um ID automaticamente — o ID usado é sempre o retornado.
func (idx *Index[T]) Put(ctx context.Context, id string, doc *T) (string, error) {
	body, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("elasticsearch: put: %w", err)
	}

	req := esapi.IndexRequest{Index: idx.name, DocumentID: id, Body: bytes.NewReader(body)}
	res, err := req.Do(ctx, idx.client.raw)
	if err != nil {
		return "", fmt.Errorf("elasticsearch: put: %w", err)
	}
	respBody, err := readBody(res)
	if err != nil {
		return "", err
	}

	var parsed struct {
		ID string `json:"_id"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("elasticsearch: put: decode: %w", err)
	}
	return parsed.ID, nil
}

// Get busca um documento pelo ID. Retorna (nil, false, nil) se não existir.
func (idx *Index[T]) Get(ctx context.Context, id string) (*T, bool, error) {
	req := esapi.GetRequest{Index: idx.name, DocumentID: id}
	res, err := req.Do(ctx, idx.client.raw)
	if err != nil {
		return nil, false, fmt.Errorf("elasticsearch: get: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return nil, false, nil
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, false, fmt.Errorf("elasticsearch: get: %w", err)
	}
	if res.IsError() {
		return nil, false, fmt.Errorf("elasticsearch: get: %s: %s", res.Status(), string(body))
	}

	var parsed struct {
		Found  bool `json:"found"`
		Source T    `json:"_source"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false, fmt.Errorf("elasticsearch: get: decode: %w", err)
	}
	if !parsed.Found {
		return nil, false, nil
	}
	return &parsed.Source, true, nil
}

// Delete remove um documento pelo ID.
func (idx *Index[T]) Delete(ctx context.Context, id string) error {
	req := esapi.DeleteRequest{Index: idx.name, DocumentID: id}
	res, err := req.Do(ctx, idx.client.raw)
	if err != nil {
		return fmt.Errorf("elasticsearch: delete: %w", err)
	}
	if _, err := readBody(res); err != nil {
		return err
	}
	return nil
}

// Search executa uma query DSL do Elasticsearch (documento JSON, ex:
// mongo-like mas é sintaxe própria do ES: map[string]any{"query":
// map[string]any{"match": map[string]any{"titulo": "golang"}}}) e devolve
// os documentos encontrados decodificados em T.
func (idx *Index[T]) Search(ctx context.Context, query map[string]any) ([]T, error) {
	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: search: %w", err)
	}

	req := esapi.SearchRequest{Index: []string{idx.name}, Body: bytes.NewReader(body)}
	res, err := req.Do(ctx, idx.client.raw)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: search: %w", err)
	}
	respBody, err := readBody(res)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Hits struct {
			Hits []struct {
				Source T `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("elasticsearch: search: decode: %w", err)
	}

	results := make([]T, 0, len(parsed.Hits.Hits))
	for _, h := range parsed.Hits.Hits {
		results = append(results, h.Source)
	}
	return results, nil
}

// Count conta documentos que correspondem a query. query vazio/nil conta o
// índice inteiro.
func (idx *Index[T]) Count(ctx context.Context, query map[string]any) (int64, error) {
	var bodyReader io.Reader
	if query != nil {
		body, err := json.Marshal(map[string]any{"query": query})
		if err != nil {
			return 0, fmt.Errorf("elasticsearch: count: %w", err)
		}
		bodyReader = bytes.NewReader(body)
	}

	req := esapi.CountRequest{Index: []string{idx.name}, Body: bodyReader}
	res, err := req.Do(ctx, idx.client.raw)
	if err != nil {
		return 0, fmt.Errorf("elasticsearch: count: %w", err)
	}
	respBody, err := readBody(res)
	if err != nil {
		return 0, err
	}

	var parsed struct {
		Count int64 `json:"count"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return 0, fmt.Errorf("elasticsearch: count: decode: %w", err)
	}
	return parsed.Count, nil
}

// Refresh força a atualização do índice, tornando gravações recentes
// imediatamente buscáveis. Use com moderação em produção (custa
// performance) — pensado principalmente para testes, onde esperar o
// refresh automático (~1s) tornaria os testes lentos e não-determinísticos.
func (idx *Index[T]) Refresh(ctx context.Context) error {
	req := esapi.IndicesRefreshRequest{Index: []string{idx.name}}
	res, err := req.Do(ctx, idx.client.raw)
	if err != nil {
		return fmt.Errorf("elasticsearch: refresh: %w", err)
	}
	if _, err := readBody(res); err != nil {
		return err
	}
	return nil
}
