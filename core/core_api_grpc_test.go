package core_test

// Prova de que apigrpc se integra com o Core através do mecanismo genérico
// core.UseModule (sem core.API.GRPC — este adapter é heavy o bastante para
// não ser importado por kyrux/core, ver apigrpc.go), e que um servidor gRPC
// coexiste com uma API REST no mesmo Core (dois protocolos diferentes,
// simultâneos, sem qualquer limitação de uso conjunto).

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"kyrux/core"
	"kyrux/core/adapters/apigrpc"
	pb "kyrux/core/internal/greetertest"
	"kyrux/core/router"
)

type coreGreeterServer struct {
	pb.UnimplementedGreeterServer
}

func (s *coreGreeterServer) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloReply, error) {
	return &pb.HelloReply{Message: "olá via core, " + req.GetName()}, nil
}

func TestCoreAPIGRPCERESTCoexistindo(t *testing.T) {
	grpcAddr := envOr("KYRUX_TEST_GRPC_ADDR", "127.0.0.1:19192")
	restAddr := envOr("KYRUX_TEST_REST_ADDR_GRPC_COEXIST", "127.0.0.1:18092")

	c := core.New()

	srv, err := core.UseModule[*grpc.Server](c, apigrpc.New(grpcAddr), "api.grpc.principal")
	if err != nil {
		t.Fatalf("UseModule (grpc): %v", err)
	}
	pb.RegisterGreeterServer(srv, &coreGreeterServer{})

	r, err := c.API.REST(restAddr)
	if err != nil {
		t.Fatalf("API.REST: %v", err)
	}
	r.Handle("GET /status", func(ctx *router.Context) {
		ctx.JSON(200, map[string]string{"status": "ok"})
	})

	if err := c.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer c.Shutdown()

	var conn *grpc.ClientConn
	for i := 0; i < 20; i++ {
		conn, err = grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
	if resp.GetMessage() != "olá via core, kyrux" {
		t.Errorf("esperava %q, recebeu %q", "olá via core, kyrux", resp.GetMessage())
	}

	if len(c.Modules()) != 2 {
		t.Errorf("esperava 2 módulos ativos (grpc + rest), recebeu %d", len(c.Modules()))
	}
}
