package realtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"kyrux/core/security/session"

	"github.com/coder/websocket"
)

const (
	wsWriteTimeout = 10 * time.Second
	// wsPingInterval mantém a conexão viva e detecta clientes mortos:
	// o Ping aguarda o Pong — sem resposta, a conexão é encerrada.
	wsPingInterval = 30 * time.Second
)

func clientID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("realtime: falha ao gerar client ID: %v", err))
	}
	return hex.EncodeToString(b)
}

type Client struct {
	id   string
	key  string // ID da sessão (valor do cookie) — vazio para visitantes sem sessão
	conn *websocket.Conn
	hub  *Hub
	send chan []byte
}

// sessionKey extrai o valor do cookie de sessão sem validá-lo no Store —
// serve apenas como chave de roteamento para os envios *For do Hub.
func sessionKey(r *http.Request) string {
	if c, err := r.Cookie(session.CookieName()); err == nil {
		return c.Value
	}
	return ""
}

// readPump consome frames do cliente até a conexão cair.
// Pongs e frames de controle são tratados internamente pela lib durante o Read.
func (c *Client) readPump() {
	defer c.hub.Unregister(c.id)
	ctx := context.Background()
	for {
		if _, _, err := c.conn.Read(ctx); err != nil {
			return
		}
	}
}

// writePump serializa as escritas na conexão e envia pings periódicos.
func (c *Client) writePump() {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			if err := c.write(msg); err != nil {
				c.hub.Unregister(c.id)
				return
			}
		case <-ticker.C:
			if err := c.ping(); err != nil {
				c.hub.Unregister(c.id)
				return
			}
		}
	}
}

func (c *Client) write(msg []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), wsWriteTimeout)
	defer cancel()
	return c.conn.Write(ctx, websocket.MessageText, msg)
}

func (c *Client) ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), wsWriteTimeout)
	defer cancel()
	return c.conn.Ping(ctx)
}

func (c *Client) close() {
	close(c.send)
	c.conn.Close(websocket.StatusNormalClosure, "")
}
