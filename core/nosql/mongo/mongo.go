// Package mongo é um client MongoDB idiomático para o Kyrux — não faz parte
// do ORM relacional (core/orm) porque MongoDB não tem esse modelo de dados:
// documentos BSON em coleções, sem tabelas/colunas/JOIN, filtrados por
// documentos (bson.M), não por WHERE em SQL. Tentar encaixar isso no
// Query[T] do ORM seria fingir uma portabilidade que não existe — este
// pacote assume o modelo de documentos de verdade, com generics só para
// dar ergonomia de "coleção tipada" equivalente à do ORM.
//
// Uso básico:
//
//	c, err := mongo.New("mongodb://localhost:27017", "meubanco")
//	posts := mongo.CollectionOf[Post](c, "posts")
//	err = posts.InsertOne(ctx, &Post{Titulo: "Olá"})
//	all, err := posts.Find(ctx, mongo.M{"publicado": true})
package mongo

import (
	"context"
	"fmt"
	"time"

	driver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// M representa um filtro/update BSON — mesma forma de bson.M (documento
// desordenado), reexportado para dispensar um segundo import em código que
// só usa a API deste pacote.
type M = map[string]any

// ErrNoDocuments é retornado por FindOne quando nada corresponde ao filtro —
// equivalente ao sql.ErrNoRows do ORM relacional.
var ErrNoDocuments = driver.ErrNoDocuments

// Client é uma conexão com um servidor MongoDB, já com o banco selecionado.
type Client struct {
	raw *driver.Client
	db  *driver.Database
}

// New conecta a um servidor MongoDB em uri e seleciona database. Faz Ping
// (timeout de 10s) antes de retornar — se o servidor não responder, retorna
// erro para o chamador decidir (o bootstrap loga e não sobe o client).
func New(uri, database string) (*Client, error) {
	raw, err := driver.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo: conectar em %s: %w", uri, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := raw.Ping(ctx, nil); err != nil {
		_ = raw.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo: ping em %s: %w", uri, err)
	}

	return &Client{raw: raw, db: raw.Database(database)}, nil
}

// Close encerra a conexão. Chamado automaticamente pelo bootstrap no shutdown.
func (c *Client) Close(ctx context.Context) error {
	return c.raw.Disconnect(ctx)
}

// Raw devolve o *mongo.Client nativo do driver oficial — escape hatch para
// qualquer operação (transações, change streams, agregações complexas) que
// este wrapper não cobre.
func (c *Client) Raw() *driver.Client { return c.raw }

// Database devolve o *mongo.Database nativo do driver oficial.
func (c *Client) Database() *driver.Database { return c.db }

// Coll é uma coleção MongoDB tipada — T é a struct Go que representa o
// documento (use tags `bson:"..."` para controlar nomes de campo, igual
// encoding/json).
type Coll[T any] struct {
	raw *driver.Collection
}

// CollectionOf abre a coleção name no banco de c, tipada para T.
func CollectionOf[T any](c *Client, name string) *Coll[T] {
	return &Coll[T]{raw: c.db.Collection(name)}
}

// Raw devolve a *mongo.Collection nativa — escape hatch para Aggregate,
// BulkWrite, Watch e outras operações que este wrapper não cobre.
func (c *Coll[T]) Raw() *driver.Collection { return c.raw }

// InsertOne insere um documento. O ID gerado (se ObjectID) fica em
// doc.ID quando o campo existir com bson:"_id,omitempty" — mesma convenção
// do driver oficial.
func (c *Coll[T]) InsertOne(ctx context.Context, doc *T) error {
	if _, err := c.raw.InsertOne(ctx, doc); err != nil {
		return fmt.Errorf("mongo: insertone: %w", err)
	}
	return nil
}

// InsertMany insere vários documentos numa única chamada.
func (c *Coll[T]) InsertMany(ctx context.Context, docs []*T) error {
	items := make([]any, len(docs))
	for i, d := range docs {
		items[i] = d
	}
	if _, err := c.raw.InsertMany(ctx, items); err != nil {
		return fmt.Errorf("mongo: insertmany: %w", err)
	}
	return nil
}

// Find retorna todos os documentos que correspondem a filter.
// filter vazio (mongo.M{}) retorna a coleção inteira.
func (c *Coll[T]) Find(ctx context.Context, filter M, opts ...options.Lister[options.FindOptions]) ([]T, error) {
	cur, err := c.raw.Find(ctx, filter, opts...)
	if err != nil {
		return nil, fmt.Errorf("mongo: find: %w", err)
	}
	defer cur.Close(ctx)

	results := make([]T, 0)
	if err := cur.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("mongo: find: decode: %w", err)
	}
	return results, nil
}

// FindOne retorna o primeiro documento que corresponde a filter.
// Retorna ErrNoDocuments se nada corresponder.
func (c *Coll[T]) FindOne(ctx context.Context, filter M) (*T, error) {
	var result T
	if err := c.raw.FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateOne atualiza o primeiro documento que corresponde a filter.
// update deve usar operadores do MongoDB (ex: mongo.M{"$set": mongo.M{"nome": "x"}})
// — diferente do UPDATE SQL, um update sem operador falha (não substitui o documento).
// Retorna o número de documentos modificados.
func (c *Coll[T]) UpdateOne(ctx context.Context, filter, update M) (int64, error) {
	res, err := c.raw.UpdateOne(ctx, filter, update)
	if err != nil {
		return 0, fmt.Errorf("mongo: updateone: %w", err)
	}
	return res.ModifiedCount, nil
}

// UpdateMany atualiza todos os documentos que correspondem a filter.
// Mesma regra de update do UpdateOne.
func (c *Coll[T]) UpdateMany(ctx context.Context, filter, update M) (int64, error) {
	res, err := c.raw.UpdateMany(ctx, filter, update)
	if err != nil {
		return 0, fmt.Errorf("mongo: updatemany: %w", err)
	}
	return res.ModifiedCount, nil
}

// DeleteOne remove o primeiro documento que corresponde a filter.
func (c *Coll[T]) DeleteOne(ctx context.Context, filter M) (int64, error) {
	res, err := c.raw.DeleteOne(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("mongo: deleteone: %w", err)
	}
	return res.DeletedCount, nil
}

// DeleteMany remove todos os documentos que correspondem a filter.
func (c *Coll[T]) DeleteMany(ctx context.Context, filter M) (int64, error) {
	res, err := c.raw.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("mongo: deletemany: %w", err)
	}
	return res.DeletedCount, nil
}

// Count retorna o número de documentos que correspondem a filter.
func (c *Coll[T]) Count(ctx context.Context, filter M) (int64, error) {
	n, err := c.raw.CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("mongo: count: %w", err)
	}
	return n, nil
}
