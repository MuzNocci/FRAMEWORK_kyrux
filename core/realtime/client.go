package realtime

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/websocket"
)

const wsReadTimeout = 60 * time.Second

func clientID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("realtime: falha ao gerar client ID: %v", err))
	}
	return hex.EncodeToString(b)
}

type Client struct {
	id   string
	conn *websocket.Conn
	hub  *Hub
	send chan []byte
}

func newClient(w http.ResponseWriter, r *http.Request, hub *Hub) (*Client, error) {
	var c *Client
	var err error

	handler := websocket.Handler(func(conn *websocket.Conn) {
		c = &Client{
			id:   clientID(),
			conn: conn,
			hub:  hub,
			send: make(chan []byte, 256),
		}
	})

	server := &websocket.Server{Handler: handler}
	server.ServeHTTP(w, r)

	if c == nil {
		err = fmt.Errorf("websocket upgrade failed")
	}
	return c, err
}

func (c *Client) readPump() {
	defer c.hub.Unregister(c.id)
	var msg []byte
	for {
		c.conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		if err := websocket.Message.Receive(c.conn, &msg); err != nil {
			break
		}
	}
}

func (c *Client) writePump() {
	for msg := range c.send {
		if err := websocket.Message.Send(c.conn, string(msg)); err != nil {
			break
		}
	}
}

func (c *Client) close() {
	close(c.send)
	c.conn.Close()
}
