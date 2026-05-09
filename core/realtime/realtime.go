package realtime

import (
	"encoding/json"
	"kyrux/core/events"
	"log"
	"net/http"
	"sync"
)

type domUpdate struct {
	Type   string `json:"type"`
	Target string `json:"target"`
	HTML   string `json:"html"`
	Action string `json:"action"`
}

func (h *Hub) sendDOM(target, html, action string) {
	data, err := json.Marshal(domUpdate{
		Type: "kyrux:dom", Target: target, HTML: html, Action: action,
	})
	if err != nil {
		log.Printf("realtime: marshal error: %v", err)
		return
	}
	h.mu.RLock()
	for _, c := range h.clients {
		select {
		case c.send <- data:
		default:
		}
	}
	h.mu.RUnlock()
}

func (h *Hub) Replace(target, html string) { h.sendDOM(target, html, "replace") }
func (h *Hub) Append(target, html string)  { h.sendDOM(target, html, "append") }
func (h *Hub) Prepend(target, html string) { h.sendDOM(target, html, "prepend") }
func (h *Hub) Remove(target string)        { h.sendDOM(target, "", "remove") }

type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client
	bus     *events.Bus
}

func NewHub(bus *events.Bus) *Hub {
	return &Hub{
		clients: make(map[string]*Client),
		bus:     bus,
	}
}

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	h.clients[c.id] = c
	h.mu.Unlock()
}

func (h *Hub) Unregister(id string) {
	h.mu.Lock()
	if c, ok := h.clients[id]; ok {
		c.close()
		delete(h.clients, id)
	}
	h.mu.Unlock()
}

func (h *Hub) Broadcast(event string, payload any) {
	h.bus.Publish(event, payload)
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := newClient(w, r, h)
	if err != nil {
		http.Error(w, "websocket upgrade failed", http.StatusBadRequest)
		return
	}
	h.Register(c)
	go c.writePump()
	c.readPump()
}
