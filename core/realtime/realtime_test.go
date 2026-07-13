package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kyrux/core/events"
	"kyrux/core/security/session"

	"github.com/coder/websocket"
)

func dial(t *testing.T, url, sessID string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts := &websocket.DialOptions{}
	if sessID != "" {
		opts.HTTPHeader = http.Header{"Cookie": []string{session.CookieName() + "=" + sessID}}
	}
	c, _, err := websocket.Dial(ctx, url, opts)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

func waitClients(t *testing.T, hub *Hub, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		hub.mu.RLock()
		got := len(hub.clients)
		hub.mu.RUnlock()
		if got == n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("esperava %d clientes registrados no hub", n)
}

func readUpdate(t *testing.T, c *websocket.Conn) domUpdate {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m domUpdate
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

// TestScopedSend garante que as variantes *For alcançam apenas a sessão alvo
// e que o broadcast global continua chegando a todos.
func TestScopedSend(t *testing.T) {
	hub := NewHub(events.NewBus())
	srv := httptest.NewServer(hub)
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	alice := dial(t, wsURL, "sess-alice")
	defer alice.Close(websocket.StatusNormalClosure, "")
	bob := dial(t, wsURL, "")
	defer bob.Close(websocket.StatusNormalClosure, "")

	waitClients(t, hub, 2)

	hub.ReplaceFor("sess-alice", "saldo", "<b>10</b>")
	hub.Replace("banner", "promo")

	// Alice recebe as duas mensagens, na ordem de envio.
	if m := readUpdate(t, alice); m.Target != "saldo" {
		t.Errorf("alice: esperava 'saldo', recebeu %q", m.Target)
	}
	if m := readUpdate(t, alice); m.Target != "banner" {
		t.Errorf("alice: esperava 'banner', recebeu %q", m.Target)
	}

	// Bob (sem sessão) recebe apenas o broadcast — a primeira mensagem
	// dele deve ser o banner, nunca o saldo da Alice.
	if m := readUpdate(t, bob); m.Target != "banner" {
		t.Errorf("bob: esperava apenas 'banner', recebeu %q", m.Target)
	}
}

// TestForSemSessaoEhNoOp garante que sessionID vazio não degrada para broadcast.
func TestForSemSessaoEhNoOp(t *testing.T) {
	hub := NewHub(events.NewBus())
	srv := httptest.NewServer(hub)
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	bob := dial(t, wsURL, "")
	defer bob.Close(websocket.StatusNormalClosure, "")
	waitClients(t, hub, 1)

	hub.ReplaceFor("", "privado", "vazou") // deve ser descartado
	hub.Replace("ok", "fim")               // marcador de fim

	if m := readUpdate(t, bob); m.Target != "ok" {
		t.Errorf("ReplaceFor com sessionID vazio vazou como broadcast: recebeu %q", m.Target)
	}
}
