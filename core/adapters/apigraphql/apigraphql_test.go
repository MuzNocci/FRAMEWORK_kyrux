package apigraphql

// Teste de ponta a ponta real: monta um schema GraphQL de verdade, sobe o
// servidor via Init/Configure/Start (chamados diretamente, simulando o que
// core.UseModule faz) e executa uma query HTTP real contra ele.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/graphql-go/graphql"
)

func buildTestSchema(t *testing.T) graphql.Schema {
	t.Helper()

	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"saudacao": &graphql.Field{
				Type: graphql.String,
				Args: graphql.FieldConfigArgument{
					"nome": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					nome, _ := p.Args["nome"].(string)
					return "olá, " + nome, nil
				},
			},
		},
	})

	schema, err := graphql.NewSchema(graphql.SchemaConfig{Query: queryType})
	if err != nil {
		t.Fatalf("NewSchema: %v", err)
	}
	return schema
}

func TestAdapterGraphQLQueryReal(t *testing.T) {
	addr := "127.0.0.1:18090"
	schema := buildTestSchema(t)

	a := New(addr, schema)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := a.Configure(ctx); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Shutdown(ctx)

	query := `{"query":"{ saudacao(nome: \"kyrux\") }"}`
	var resp *http.Response
	var err error
	for i := 0; i < 20; i++ {
		resp, err = http.Post("http://"+addr, "application/json", strings.NewReader(query))
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("POST /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d", resp.StatusCode)
	}

	var body struct {
		Data struct {
			Saudacao string `json:"saudacao"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Errors) != 0 {
		t.Fatalf("query devolveu erros: %+v", body.Errors)
	}
	if body.Data.Saudacao != "olá, kyrux" {
		t.Errorf("esperava %q, recebeu %q", "olá, kyrux", body.Data.Saudacao)
	}
}

func TestAdapterGraphQLValueDevolveSchema(t *testing.T) {
	schema := buildTestSchema(t)
	a := New("127.0.0.1:0", schema)
	if a.Value() == nil {
		t.Fatal("Value() não deveria ser nil")
	}
	if a.Value().QueryType().Name() != "Query" {
		t.Errorf("schema devolvido não bate com o schema fornecido")
	}
}

func TestAdapterGraphQLEnderecoVazioFalhaEmInit(t *testing.T) {
	a := New("", buildTestSchema(t))
	if err := a.Init(context.Background()); err == nil {
		t.Error("esperava erro de Init com endereço vazio")
	}
}
