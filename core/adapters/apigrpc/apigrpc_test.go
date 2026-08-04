package apigrpc

// Teste de ponta a ponta real: sobe um servidor gRPC de verdade (Init/
// Configure/Start chamados diretamente, simulando o que core.UseModule
// faz), registra um serviço gerado por protoc (internal/greetertest) e faz
// uma chamada gRPC real de cliente contra ele.

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "kyrux/core/internal/greetertest"
)

type greeterServer struct {
	pb.UnimplementedGreeterServer
}

func (s *greeterServer) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloReply, error) {
	return &pb.HelloReply{Message: "olá, " + req.GetName()}, nil
}

func TestAdapterGRPCChamadaReal(t *testing.T) {
	addr := "127.0.0.1:19090"

	a := New(addr)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := a.Configure(ctx); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Registro do serviço tem que acontecer depois de Configure (o
	// *grpc.Server já existe) e antes de Start (antes de aceitar conexões).
	pb.RegisterGreeterServer(a.Value(), &greeterServer{})

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Shutdown(ctx)

	var conn *grpc.ClientConn
	var err error
	for i := 0; i < 20; i++ {
		conn, err = grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer conn.Close()

	client := pb.NewGreeterClient(conn)

	var resp *pb.HelloReply
	for i := 0; i < 20; i++ {
		resp, err = client.SayHello(context.Background(), &pb.HelloRequest{Name: "kyrux"})
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("SayHello: %v", err)
	}
	if resp.GetMessage() != "olá, kyrux" {
		t.Errorf("esperava %q, recebeu %q", "olá, kyrux", resp.GetMessage())
	}
}

func TestAdapterGRPCEnderecoVazioFalhaEmInit(t *testing.T) {
	a := New("")
	if err := a.Init(context.Background()); err == nil {
		t.Error("esperava erro de Init com endereço vazio")
	}
}

func TestAdapterGRPCEnderecoJaEmUsoFalhaEmStart(t *testing.T) {
	addr := "127.0.0.1:19091"

	a1 := New(addr)
	ctx := context.Background()
	if err := a1.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a1.Configure(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a1.Start(ctx); err != nil {
		t.Fatalf("primeiro Start: %v", err)
	}
	defer a1.Shutdown(ctx)

	a2 := New(addr)
	if err := a2.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a2.Configure(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a2.Start(ctx); err == nil {
		t.Error("esperava erro ao subir um segundo servidor gRPC na mesma porta já em uso")
	}
}
