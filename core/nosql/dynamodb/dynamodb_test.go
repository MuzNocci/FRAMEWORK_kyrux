package dynamodb

// Testes de integração real contra o DynamoDB Local (container Docker —
// emula a API do DynamoDB sem custo/credenciais reais da AWS). Pulados
// (t.Skip) se o servidor não estiver acessível.

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type produtoItem struct {
	PK    string  `dynamodbav:"pk"`
	SK    string  `dynamodbav:"sk"`
	Nome  string  `dynamodbav:"nome"`
	Preco float64 `dynamodbav:"preco"`
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openTestClient(t *testing.T) *Client {
	t.Helper()
	endpoint := envOr("KYRUX_TEST_DYNAMODB_ENDPOINT", "http://127.0.0.1:8010")
	c, err := New(context.Background(), "us-east-1", endpoint, "dummy", "dummy")
	if err != nil {
		t.Skipf("dynamodb indisponível em %s: %v", endpoint, err)
	}
	return c
}

func freshTable(t *testing.T, c *Client) *Table[produtoItem] {
	t.Helper()
	ctx := context.Background()
	name := "kyrux_test_produtos"

	// Remove a tabela de tentativas anteriores (ignora erro se não existir).
	c.raw.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(name)})

	_, err := c.raw.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(name),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	waiter := dynamodb.NewTableExistsWaiter(c.raw)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(name)}, 10e9); err != nil {
		t.Fatalf("esperar tabela ficar ativa: %v", err)
	}

	return TableOf[produtoItem](c, name)
}

func TestDynamoDBPutGet(t *testing.T) {
	c := openTestClient(t)
	table := freshTable(t, c)
	ctx := context.Background()

	if err := table.Put(ctx, &produtoItem{PK: "produto#1", SK: "meta", Nome: "Caneca", Preco: 29.9}); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, ok, err := table.Get(ctx, map[string]any{"pk": "produto#1", "sk": "meta"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatal("esperava encontrar o item")
	}
	if got.Nome != "Caneca" || got.Preco != 29.9 {
		t.Errorf("item incorreto: %+v", got)
	}
}

func TestDynamoDBGetInexistente(t *testing.T) {
	c := openTestClient(t)
	table := freshTable(t, c)

	_, ok, err := table.Get(context.Background(), map[string]any{"pk": "nao-existe", "sk": "meta"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ok {
		t.Error("esperava ok=false para item inexistente")
	}
}

func TestDynamoDBQueryPorPartitionKey(t *testing.T) {
	c := openTestClient(t)
	table := freshTable(t, c)
	ctx := context.Background()

	items := []*produtoItem{
		{PK: "produto#1", SK: "meta", Nome: "Caneca", Preco: 29.9},
		{PK: "produto#1", SK: "estoque", Nome: "Caneca", Preco: 29.9},
		{PK: "produto#2", SK: "meta", Nome: "Mochila", Preco: 199.9},
	}
	for _, it := range items {
		if err := table.Put(ctx, it); err != nil {
			t.Fatalf("put: %v", err)
		}
	}

	results, err := table.Query(ctx, "pk = :pk", map[string]any{":pk": "produto#1"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("esperava 2 itens para produto#1, recebeu %d: %+v", len(results), results)
	}
}

func TestDynamoDBScan(t *testing.T) {
	c := openTestClient(t)
	table := freshTable(t, c)
	ctx := context.Background()

	for i, nome := range []string{"A", "B", "C"} {
		if err := table.Put(ctx, &produtoItem{PK: "p", SK: nome, Nome: nome, Preco: float64(i)}); err != nil {
			t.Fatalf("put: %v", err)
		}
	}

	all, err := table.Scan(ctx)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("esperava 3 itens, recebeu %d", len(all))
	}
}

func TestDynamoDBDelete(t *testing.T) {
	c := openTestClient(t)
	table := freshTable(t, c)
	ctx := context.Background()

	if err := table.Put(ctx, &produtoItem{PK: "temp", SK: "x", Nome: "Temp"}); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := table.Delete(ctx, map[string]any{"pk": "temp", "sk": "x"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, ok, err := table.Get(ctx, map[string]any{"pk": "temp", "sk": "x"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ok {
		t.Error("item deveria ter sido removido")
	}
}

func TestNewDynamoDBFalhaComEndpointInvalido(t *testing.T) {
	if _, err := New(context.Background(), "us-east-1", "http://127.0.0.1:1", "dummy", "dummy"); err == nil {
		t.Error("esperava erro ao conectar em endpoint inválido")
	}
}
