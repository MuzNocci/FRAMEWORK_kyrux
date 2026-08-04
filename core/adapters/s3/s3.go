// Package s3 é o adapter que expõe um client de armazenamento de objetos
// (github.com/aws/aws-sdk-go-v2/service/s3 — o SDK oficial da AWS) como um
// Module do Core (kyrux/core). Funciona tanto com o S3 real da AWS quanto
// com qualquer serviço compatível (MinIO, etc.) — mesmo padrão de endpoint
// configurável já usado por core/nosql/dynamodb.
//
// Ao contrário de restapi/sqlpostgres, este pacote NÃO é importado por
// kyrux/core — o SDK da AWS é uma dependência pesada de verdade que a
// maioria dos projetos Kyrux nunca vai usar. Importe este pacote você
// mesmo, na sua aplicação, só se for usar storage de objetos — mesma
// filosofia dos clients NoSQL (core/nosql/*) e dos outros adapters.
//
// Ativação: como este adapter recebe parâmetros de construção, ele não
// passa pelo registro por nome (core/registry) — construa com New e ative
// com core.UseModule:
//
//	client, err := core.UseModule[*s3.Client](c, s3adapter.New("principal", "us-east-1", "http://localhost:9000", "minioadmin", "minioadmin"), "storage.s3.principal")
//	bucket := client.Bucket("meu-bucket")
//	err = bucket.Put(ctx, "foto.jpg", arquivo, "image/jpeg")
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithy "github.com/aws/smithy-go"
)

// Client é uma conexão com o serviço de armazenamento de objetos (AWS S3
// real, ou um serviço compatível como MinIO).
type Client struct {
	raw *s3.Client
}

// Bucket abre um bucket nomeado para operações de objeto.
func (c *Client) Bucket(name string) *Bucket {
	return &Bucket{client: c.raw, name: name}
}

// Raw devolve o *s3.Client nativo do SDK oficial — escape hatch para
// operações que este wrapper não cobre (multipart upload, presigned URLs,
// políticas de bucket, etc.).
func (c *Client) Raw() *s3.Client { return c.raw }

// Bucket representa um bucket específico — todas as operações (Put/Get/
// Delete/List/Exists) são relativas a ele.
type Bucket struct {
	client *s3.Client
	name   string
}

// Put grava (cria ou substitui) um objeto inteiro a partir de body.
func (b *Bucket) Put(ctx context.Context, key string, body io.Reader, contentType string) error {
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(b.name),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("s3: put %s/%s: %w", b.name, key, err)
	}
	return nil
}

// Get baixa um objeto — o chamador é responsável por fechar o
// io.ReadCloser devolvido.
func (b *Bucket) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.name),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3: get %s/%s: %w", b.name, key, err)
	}
	return out.Body, nil
}

// Delete remove um objeto.
func (b *Bucket) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.name),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3: delete %s/%s: %w", b.name, key, err)
	}
	return nil
}

// Exists verifica se um objeto existe, sem baixar o conteúdo (HeadObject).
func (b *Bucket) Exists(ctx context.Context, key string) (bool, error) {
	_, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.name),
		Key:    aws.String(key),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NotFound" {
			return false, nil
		}
		return false, fmt.Errorf("s3: exists %s/%s: %w", b.name, key, err)
	}
	return true, nil
}

// List lista as chaves de objetos com o prefixo dado (todas as páginas).
func (b *Bucket) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.name),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3: list %s (prefix %q): %w", b.name, prefix, err)
		}
		for _, obj := range page.Contents {
			keys = append(keys, aws.ToString(obj.Key))
		}
	}
	return keys, nil
}

// Adapter implementa registry.Module para uma conexão de storage nomeada.
type Adapter struct {
	name      string
	region    string
	endpoint  string
	accessKey string
	secretKey string
	client    *Client
}

// New cria (mas ainda não conecta — isso só acontece em Configure) um
// adapter de storage. name identifica esta conexão entre outras do mesmo
// tipo. region é obrigatória mesmo apontando para um endpoint não-AWS (o
// SDK exige alguma região configurada). endpoint vazio usa o S3 real da
// AWS, com credenciais resolvidas pela cadeia padrão do SDK (accessKey/
// secretKey são ignorados nesse caso); um endpoint não-vazio (ex: MinIO)
// exige accessKey/secretKey explícitos.
func New(name, region, endpoint, accessKey, secretKey string) *Adapter {
	return &Adapter{name: name, region: region, endpoint: endpoint, accessKey: accessKey, secretKey: secretKey}
}

func (a *Adapter) Name() string { return "storage.s3." + a.name }

func (a *Adapter) Init(ctx context.Context) error {
	if a.region == "" {
		return fmt.Errorf("s3: região vazia para a conexão %q", a.name)
	}
	return nil
}

// Configure resolve a config do SDK e testa a conectividade (ListBuckets)
// antes de devolver um client que pareceria pronto mas não é.
func (a *Adapter) Configure(ctx context.Context) error {
	optFns := []func(*config.LoadOptions) error{config.WithRegion(a.region)}
	if a.endpoint != "" {
		optFns = append(optFns, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(a.accessKey, a.secretKey, ""),
		))
	}
	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return fmt.Errorf("s3: carregar config (%s): %w", a.name, err)
	}

	var s3Opts []func(*s3.Options)
	if a.endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(a.endpoint)
			// path-style (endpoint/bucket/key) em vez de virtual-hosted
			// (bucket.endpoint/key) — necessário pra MinIO e a maioria dos
			// serviços S3-compatíveis fora da AWS real.
			o.UsePathStyle = true
		})
	}
	raw := s3.NewFromConfig(cfg, s3Opts...)

	if _, err := raw.ListBuckets(ctx, &s3.ListBucketsInput{}); err != nil {
		return fmt.Errorf("s3: conectar (%s): %w", a.name, err)
	}

	a.client = &Client{raw: raw}
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

// Shutdown não faz nada — o client HTTP subjacente não precisa de
// encerramento explícito.
func (a *Adapter) Shutdown(ctx context.Context) error { return nil }

// Value devolve o *Client já pronto.
func (a *Adapter) Value() *Client { return a.client }
