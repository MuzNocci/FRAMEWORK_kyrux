package realtime

import (
	"encoding/json"
	"kyrux/core/events"
	"log"
	"net/http"
	"net/url"
	"strings"
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

// Replace/Append/Prepend tratam html como HTML confiável (renderizado pelo servidor).
// Para conteúdo do usuário, use as variantes Text abaixo.
func (h *Hub) Replace(target, html string) { h.sendDOM(target, html, "replace") }
func (h *Hub) Append(target, html string)  { h.sendDOM(target, html, "append") }
func (h *Hub) Prepend(target, html string) { h.sendDOM(target, html, "prepend") }
func (h *Hub) Remove(target string)        { h.sendDOM(target, "", "remove") }

// ReplaceText/AppendText/PrependText são seguros para conteúdo do usuário:
// o cliente usa textContent em vez de innerHTML, impedindo XSS.
func (h *Hub) ReplaceText(target, text string) { h.sendDOM(target, text, "replace-text") }
func (h *Hub) AppendText(target, text string)  { h.sendDOM(target, text, "append-text") }
func (h *Hub) PrependText(target, text string) { h.sendDOM(target, text, "prepend-text") }

type Hub struct {
	mu             sync.RWMutex
	clients        map[string]*Client
	bus            *events.Bus
	allowedOrigins []string
}

func NewHub(bus *events.Bus) *Hub {
	return &Hub{
		clients: make(map[string]*Client),
		bus:     bus,
	}
}

// SetAllowedOrigins define os hosts permitidos na validação de Origin do WebSocket.
// Deve ser chamado no bootstrap com os mesmos hosts de ALLOWED_HOSTS.
func (h *Hub) SetAllowedOrigins(hosts []string) {
	h.allowedOrigins = hosts
}

func (h *Hub) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // cliente não-browser (curl, testes)
	}
	if len(h.allowedOrigins) == 0 {
		return true // sem lista configurada, permite tudo
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	for _, allowed := range h.allowedOrigins {
		if strings.EqualFold(host, allowed) {
			return true
		}
	}
	return false
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
	if !h.originAllowed(r) {
		http.Error(w, "websocket: origin não permitido", http.StatusForbidden)
		return
	}
	c, err := newClient(w, r, h)
	if err != nil {
		http.Error(w, "websocket upgrade failed", http.StatusBadRequest)
		return
	}
	h.Register(c)
	go c.writePump()
	c.readPump()
}
