// Package dynamodb é um client DynamoDB idiomático para o Kyrux — não faz
// parte do ORM relacional (core/orm). DynamoDB não tem linguagem de query:
// é uma API de operações (GetItem, Query, Scan, PutItem...) sobre uma chave
// primária obrigatória (partition key, + sort key opcional). GetItem exige a
// chave exata; Query filtra pela partition key (e uma condição na sort key);
// qualquer outro filtro exige Scan — varredura da tabela inteira, cara e
// lenta, ou um Global Secondary Index criado antecipadamente. Não existe
// JOIN. Tentar encaixar isso no Query[T] do ORM fingiria uma portabilidade
// que não existe.
//
// Como os outros clients NoSQL deste framework, este pacote NÃO é importado
// por core/bootstrap — o SDK oficial da AWS (aws-sdk-go-v2), uma dependência
// consideravelmente pesada, só entra no binário se você mesmo importar
// kyrux/core/nosql/dynamodb.
package dynamodb

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// Client é uma conexão com o DynamoDB (AWS real ou um serviço compatível,
// como o DynamoDB Local para desenvolvimento).
type Client struct {
	raw *dynamodb.Client
}

// New cria um client DynamoDB.
//
// region é obrigatório mesmo apontando para um endpoint local (o SDK exige
// alguma região configurada, mesmo que o serviço de destino não seja a AWS
// de verdade).
//
// endpoint vazio usa os endpoints reais da AWS, com credenciais resolvidas
// pela cadeia padrão do SDK (variáveis de ambiente, ~/.aws/credentials,
// role IAM etc. — accessKey/secretKey são ignorados nesse caso). Um
// endpoint não-vazio (ex: "http://localhost:8000" pro DynamoDB Local) exige
// accessKey/secretKey explícitos — DynamoDB Local não valida essas
// credenciais, mas o SDK exige que existam estruturalmente.
func New(ctx context.Context, region, endpoint, accessKey, secretKey string) (*Client, error) {
	optFns := []func(*config.LoadOptions) error{config.WithRegion(region)}
	if endpoint != "" {
		optFns = append(optFns, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}

	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("dynamodb: carregar config: %w", err)
	}

	var dynOpts []func(*dynamodb.Options)
	if endpoint != "" {
		dynOpts = append(dynOpts, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	raw := dynamodb.NewFromConfig(cfg, dynOpts...)

	// ListTables com limite 1 serve de "ping" — confirma que o endpoint
	// responde antes de devolver um client que pareceria pronto mas não é.
	if _, err := raw.ListTables(ctx, &dynamodb.ListTablesInput{Limit: aws.Int32(1)}); err != nil {
		return nil, fmt.Errorf("dynamodb: conectar: %w", err)
	}

	return &Client{raw: raw}, nil
}

// Raw devolve o *dynamodb.Client nativo do SDK oficial — escape hatch para
// transações (TransactWriteItems), batch operations, streams e qualquer
// coisa que este wrapper não cobre.
func (c *Client) Raw() *dynamodb.Client { return c.raw }

// Table é uma tabela DynamoDB tipada — T é a struct Go que representa o
// item (tags `dynamodbav:"..."` controlam o nome dos atributos).
type Table[T any] struct {
	client *Client
	name   string
}

// TableOf abre a tabela name, tipada para T.
func TableOf[T any](c *Client, name string) *Table[T] {
	return &Table[T]{client: c, name: name}
}

// Put grava (cria ou substitui) um item inteiro.
func (t *Table[T]) Put(ctx context.Context, item *T) error {
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("dynamodb: put: marshal: %w", err)
	}
	_, err = t.client.raw.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(t.name),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("dynamodb: put: %w", err)
	}
	return nil
}

// Get busca um item pela chave primária exata — key contém a partition key
// (e a sort key, se a tabela tiver uma), ex: mongo-like map[string]any{"id": "abc"}.
// Retorna (nil, false, nil) se não existir.
func (t *Table[T]) Get(ctx context.Context, key map[string]any) (*T, bool, error) {
	avKey, err := attributevalue.MarshalMap(key)
	if err != nil {
		return nil, false, fmt.Errorf("dynamodb: get: marshal key: %w", err)
	}
	out, err := t.client.raw.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(t.name),
		Key:       avKey,
	})
	if err != nil {
		return nil, false, fmt.Errorf("dynamodb: get: %w", err)
	}
	if out.Item == nil {
		return nil, false, nil
	}
	var result T
	if err := attributevalue.UnmarshalMap(out.Item, &result); err != nil {
		return nil, false, fmt.Errorf("dynamodb: get: unmarshal: %w", err)
	}
	return &result, true, nil
}

// Delete remove um item pela chave primária exata.
func (t *Table[T]) Delete(ctx context.Context, key map[string]any) error {
	avKey, err := attributevalue.MarshalMap(key)
	if err != nil {
		return fmt.Errorf("dynamodb: delete: marshal key: %w", err)
	}
	_, err = t.client.raw.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(t.name),
		Key:       avKey,
	})
	if err != nil {
		return fmt.Errorf("dynamodb: delete: %w", err)
	}
	return nil
}

// Query busca itens pela partition key (e opcionalmente uma condição na
// sort key) — a única forma eficiente de filtrar no DynamoDB sem Scan.
// keyCondition usa a KeyConditionExpression nativa do DynamoDB (ex:
// "pk = :pk AND sk > :min"); values fornece os valores dos placeholders
// (ex: map[string]any{":pk": "user#1", ":min": 100}).
func (t *Table[T]) Query(ctx context.Context, keyCondition string, values map[string]any) ([]T, error) {
	avValues, err := attributevalue.MarshalMap(values)
	if err != nil {
		return nil, fmt.Errorf("dynamodb: query: marshal values: %w", err)
	}
	out, err := t.client.raw.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(t.name),
		KeyConditionExpression:    aws.String(keyCondition),
		ExpressionAttributeValues: avValues,
	})
	if err != nil {
		return nil, fmt.Errorf("dynamodb: query: %w", err)
	}
	results := make([]T, 0, len(out.Items))
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &results); err != nil {
		return nil, fmt.Errorf("dynamodb: query: unmarshal: %w", err)
	}
	return results, nil
}

// Scan varre a tabela inteira, ignorando a chave primária — é a operação
// mais cara do DynamoDB (custo e latência crescem com o tamanho da
// tabela). Use só quando não há como expressar a busca via Get/Query (ou
// um Global Secondary Index dedicado); nunca no caminho quente de produção
// de uma tabela grande.
func (t *Table[T]) Scan(ctx context.Context) ([]T, error) {
	out, err := t.client.raw.Scan(ctx, &dynamodb.ScanInput{TableName: aws.String(t.name)})
	if err != nil {
		return nil, fmt.Errorf("dynamodb: scan: %w", err)
	}
	results := make([]T, 0, len(out.Items))
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &results); err != nil {
		return nil, fmt.Errorf("dynamodb: scan: unmarshal: %w", err)
	}
	return results, nil
}
