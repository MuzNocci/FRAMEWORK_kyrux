package realtime

import (
	"encoding/json"
	"kyrux/core/events"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/coder/websocket"
)

type domUpdate struct {
	Type   string `json:"type"`
	Target string `json:"target"`
	HTML   string `json:"html"`
	Action string `json:"action"`
}

// sendDOM envia a atualização aos clientes. key == "" → broadcast global;
// key preenchida → apenas conexões cuja sessão corresponde.
func (h *Hub) sendDOM(key, target, html, action string) {
	data, err := json.Marshal(domUpdate{
		Type: "kyrux:dom", Target: target, HTML: html, Action: action,
	})
	if err != nil {
		log.Printf("realtime: marshal error: %v", err)
		return
	}
	h.mu.RLock()
	for _, c := range h.clients {
		if key != "" && c.key != key {
			continue
		}
		select {
		case c.send <- data:
		default:
		}
	}
	h.mu.RUnlock()
}

// Replace/Append/Prepend/Remove são BROADCAST GLOBAL: todos os clientes
// conectados recebem a atualização. Nunca use com dados privados de um
// usuário — para isso existem as variantes *For abaixo.
// O html é tratado como HTML confiável (renderizado pelo servidor);
// para conteúdo vindo do usuário, use as variantes Text.
func (h *Hub) Replace(target, html string) { h.sendDOM("", target, html, "replace") }
func (h *Hub) Append(target, html string)  { h.sendDOM("", target, html, "append") }
func (h *Hub) Prepend(target, html string) { h.sendDOM("", target, html, "prepend") }
func (h *Hub) Remove(target string)        { h.sendDOM("", target, "", "remove") }

// ReplaceText/AppendText/PrependText são broadcast global, mas seguros para
// conteúdo do usuário: o cliente usa textContent em vez de innerHTML (sem XSS).
func (h *Hub) ReplaceText(target, text string) { h.sendDOM("", target, text, "replace-text") }
func (h *Hub) AppendText(target, text string)  { h.sendDOM("", target, text, "append-text") }
func (h *Hub) PrependText(target, text string) { h.sendDOM("", target, text, "prepend-text") }

// As variantes *For enviam apenas às conexões da sessão indicada — use-as
// para conteúdo por usuário (saldo, notificações, carrinho):
//
//	sess, _ := ctx.Get("session")           // colocado por RequireLogin
//	fw.Realtime.ReplaceFor(sess.(*session.Session).ID, "saldo", html)
//
// sessionID vazio é no-op: jamais degrada para broadcast por acidente.
func (h *Hub) sendDOMFor(sessionID, target, html, action string) {
	if sessionID == "" {
		return
	}
	h.sendDOM(sessionID, target, html, action)
}

func (h *Hub) ReplaceFor(sessionID, target, html string) {
	h.sendDOMFor(sessionID, target, html, "replace")
}
func (h *Hub) AppendFor(sessionID, target, html string) {
	h.sendDOMFor(sessionID, target, html, "append")
}
func (h *Hub) PrependFor(sessionID, target, html string) {
	h.sendDOMFor(sessionID, target, html, "prepend")
}
func (h *Hub) RemoveFor(sessionID, target string) { h.sendDOMFor(sessionID, target, "", "remove") }

// Variantes *TextFor: por sessão e seguras para conteúdo do usuário (textContent).
func (h *Hub) ReplaceTextFor(sessionID, target, text string) {
	h.sendDOMFor(sessionID, target, text, "replace-text")
}
func (h *Hub) AppendTextFor(sessionID, target, text string) {
	h.sendDOMFor(sessionID, target, text, "append-text")
}
func (h *Hub) PrependTextFor(sessionID, target, text string) {
	h.sendDOMFor(sessionID, target, text, "prepend-text")
}

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
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// A validação de Origin já foi feita por originAllowed acima,
		// contra a mesma lista de ALLOWED_HOSTS do resto do framework.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return // Accept já respondeu o erro ao cliente
	}
	c := &Client{
		id:   clientID(),
		key:  sessionKey(r),
		conn: conn,
		hub:  h,
		send: make(chan []byte, 256),
	}
	h.Register(c)
	go c.writePump()
	c.readPump()
}
