// Package cassandra é um client Cassandra idiomático para o Kyrux — não faz
// parte do ORM relacional (core/orm) porque CQL (Cassandra Query Language),
// apesar de parecer SQL, tem restrições fundamentalmente diferentes: WHERE
// só filtra eficientemente por partition key (e clustering columns) — um
// WHERE arbitrário exige ALLOW FILTERING, que faz varredura completa do
// cluster (anti-padrão, nunca usado aqui). Não existe JOIN. Migrations no
// sentido do ORM relacional também não existem — o schema (keyspace/tabela)
// é definido em CQL e você aplica manualmente (ou via ferramenta própria).
// Tentar encaixar isso no Query[T] fingiria uma portabilidade que não existe.
//
// Como os outros clients NoSQL (core/nosql/mongo, core/nosql/redis), este
// pacote NÃO é importado por core/bootstrap — o driver oficial
// (github.com/gocql/gocql) só entra no binário se você mesmo importar
// kyrux/core/nosql/cassandra em algum lugar do seu código.
package cassandra

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gocql/gocql"
)

// Client é uma sessão com um cluster Cassandra.
type Client struct {
	session *gocql.Session
}

// New conecta a um cluster Cassandra. hosts são os nós de contato iniciais
// (ex: []string{"127.0.0.1"}); keyspace é o keyspace padrão das queries —
// pode ser vazio se você sempre qualificar as tabelas (keyspace.tabela).
func New(hosts []string, keyspace string) (*Client, error) {
	cluster := gocql.NewCluster(hosts...)
	cluster.Keyspace = keyspace
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 10 * time.Second
	cluster.ConnectTimeout = 10 * time.Second

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("cassandra: conectar em %v: %w", hosts, err)
	}
	return &Client{session: session}, nil
}

// Close encerra a sessão.
func (c *Client) Close() { c.session.Close() }

// Raw devolve a *gocql.Session nativa do driver oficial — escape hatch para
// batches, paginação manual (PageState), políticas de retry customizadas e
// qualquer coisa que este wrapper não cobre.
func (c *Client) Raw() *gocql.Session { return c.session }

// Exec executa uma instrução CQL sem retorno de linhas — INSERT, UPDATE,
// DELETE ou DDL (CREATE TABLE, CREATE KEYSPACE etc.).
func (c *Client) Exec(ctx context.Context, cql string, args ...any) error {
	if err := c.session.Query(cql, args...).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("cassandra: exec: %w", err)
	}
	return nil
}

// SelectMap executa uma query CQL e devolve cada linha como
// map[string]any — a forma mais direta e sempre correta de ler resultados,
// sem depender de conversão de tipos. Para os tipos comuns (texto, número,
// booleano, timestamp), prefira Select[T] por ergonomia.
func (c *Client) SelectMap(ctx context.Context, cql string, args ...any) ([]map[string]any, error) {
	iter := c.session.Query(cql, args...).WithContext(ctx).Iter()
	var results []map[string]any
	for {
		row := map[string]any{}
		if !iter.MapScan(row) {
			break
		}
		results = append(results, row)
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("cassandra: selectmap: %w", err)
	}
	return results, nil
}

// Select executa uma query CQL e decodifica cada linha em T, usando as tags
// `json:"..."` do struct (reaproveita encoding/json em vez de inventar uma
// nova convenção de tag). Funciona bem para os tipos comuns — texto, número,
// booleano, timestamp. Para tipos Cassandra que não roundtrippam limpo por
// JSON (uuid, list, set, map nativos), use SelectMap ou Raw().Query(...).Scan
// diretamente.
func Select[T any](ctx context.Context, c *Client, cql string, args ...any) ([]T, error) {
	rows, err := c.SelectMap(ctx, cql, args...)
	if err != nil {
		return nil, err
	}
	results := make([]T, 0, len(rows))
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil {
			return nil, fmt.Errorf("cassandra: select: %w", err)
		}
		var t T
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("cassandra: select: decode: %w", err)
		}
		results = append(results, t)
	}
	return results, nil
}
