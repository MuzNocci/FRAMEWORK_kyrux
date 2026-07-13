package realtime

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kyrux/core/events"

	"github.com/coder/websocket"
)

// TestClientPingPong garante que o servidor responde pings do protocolo —
// é o que mantém a conexão viva em proxies/browsers.
func TestClientPingPong(t *testing.T) {
	hub := NewHub(events.NewBus())
	srv := httptest.NewServer(hub)
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	c := dial(t, wsURL, "")
	defer c.Close(websocket.StatusNormalClosure, "")
	waitClients(t, hub, 1)

	// Ping espera o pong, que é processado pelo read loop do cliente —
	// CloseRead mantém um leitor em background (como um browser faz).
	c.CloseRead(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("ping sem resposta do servidor: %v", err)
	}
}
