package s3

// Teste de integração real contra um MinIO de verdade (container Docker,
// compatível com a API S3). Pulado (t.Skip) se o servidor não estiver
// acessível.

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	endpoint := envOr("KYRUX_TEST_S3_ENDPOINT", "http://127.0.0.1:19000")
	a := New("teste", "us-east-1", endpoint, "minioadmin", "minioadmin")
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := a.Configure(ctx); err != nil {
		t.Skipf("s3/minio indisponível em %s: %v", endpoint, err)
	}
	return a
}

func freshBucket(t *testing.T, client *Client) *Bucket {
	t.Helper()
	ctx := context.Background()
	name := "kyrux-teste-bucket"

	raw := client.Raw()
	_, err := raw.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: strPtr(name)})
	if err != nil {
		var already *types.BucketAlreadyOwnedByYou
		var ownedByMe *types.BucketAlreadyExists
		if !errors.As(err, &already) && !errors.As(err, &ownedByMe) {
			t.Fatalf("CreateBucket: %v", err)
		}
	}
	return client.Bucket(name)
}

func strPtr(s string) *string { return &s }

func TestS3PutGetDeleteList(t *testing.T) {
	a := openTestAdapter(t)
	client := a.Value()
	bucket := freshBucket(t, client)
	ctx := context.Background()

	if err := bucket.Put(ctx, "pasta/arquivo.txt", strings.NewReader("conteúdo de teste do kyrux"), "text/plain"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	exists, err := bucket.Exists(ctx, "pasta/arquivo.txt")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("esperava Exists=true depois de Put")
	}

	rc, err := bucket.Get(ctx, "pasta/arquivo.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "conteúdo de teste do kyrux" {
		t.Errorf("esperava %q, recebeu %q", "conteúdo de teste do kyrux", string(data))
	}

	keys, err := bucket.List(ctx, "pasta/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, k := range keys {
		if k == "pasta/arquivo.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("esperava encontrar a chave na listagem, recebeu %v", keys)
	}

	if err := bucket.Delete(ctx, "pasta/arquivo.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	exists, err = bucket.Exists(ctx, "pasta/arquivo.txt")
	if err != nil {
		t.Fatalf("Exists (pós-delete): %v", err)
	}
	if exists {
		t.Error("esperava Exists=false depois de Delete")
	}
}

func TestS3RegiaoVaziaFalhaEmInit(t *testing.T) {
	a := New("teste", "", "http://127.0.0.1:19000", "x", "y")
	if err := a.Init(context.Background()); err == nil {
		t.Error("esperava erro de Init sem região")
	}
}

func TestS3EndpointInvalidoFalhaEmConfigure(t *testing.T) {
	a := New("teste", "us-east-1", "http://127.0.0.1:1", "x", "y")
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Configure(ctx); err == nil {
		t.Error("esperava erro de Configure com endpoint inacessível")
	}
}
