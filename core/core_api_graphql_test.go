package core_test

// Prova de que apigraphql se integra com o Core através do mecanismo
// genérico core.UseModule (sem core.API.GraphQL — este adapter é heavy
// o bastante para não ser importado por kyrux/core, ver apigraphql.go).

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/graphql-go/graphql"

	"kyrux/core"
	"kyrux/core/adapters/apigraphql"
)

func TestCoreAPIGraphQLViaUseModule(t *testing.T) {
	addr := envOr("KYRUX_TEST_GRAPHQL_ADDR", "127.0.0.1:18091")

	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"versao": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return "kyrux-core", nil
				},
			},
		},
	})
	schema, err := graphql.NewSchema(graphql.SchemaConfig{Query: queryType})
	if err != nil {
		t.Fatalf("NewSchema: %v", err)
	}

	c := core.New()
	mod := apigraphql.New(addr, schema)
	got, err := core.UseModule[*graphql.Schema](c, mod, "api.graphql.principal")
	if err != nil {
		t.Fatalf("UseModule: %v", err)
	}
	if got.QueryType().Name() != "Query" {
		t.Fatalf("schema devolvido não confere")
	}

	if err := c.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer c.Shutdown()

	var resp *http.Response
	for i := 0; i < 20; i++ {
		resp, err = http.Post("http://"+addr, "application/json", strings.NewReader(`{"query":"{ versao }"}`))
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("POST /: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Data struct {
			Versao string `json:"versao"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Versao != "kyrux-core" {
		t.Errorf("esperava %q, recebeu %q", "kyrux-core", body.Data.Versao)
	}

	if len(c.Modules()) != 1 {
		t.Errorf("esperava 1 módulo ativo, recebeu %d", len(c.Modules()))
	}
}
