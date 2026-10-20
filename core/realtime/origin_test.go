package realtime

import (
	"net/http/httptest"
	"testing"
)

func TestOriginMesmaOrigemSemLista(t *testing.T) {
	h := &Hub{}
	r := httptest.NewRequest("GET", "http://app.exemplo.com/ws", nil)
	r.Host = "app.exemplo.com"
	r.Header.Set("Origin", "https://app.exemplo.com")
	if !h.originAllowed(r) {
		t.Fatal("mesma origem deve ser aceita")
	}
	r.Header.Set("Origin", "https://atacante.exemplo")
	if h.originAllowed(r) {
		t.Fatal("sem lista, outra origem deve ser recusada (CSWSH)")
	}
	h.allowedOrigins = []string{"*"}
	if h.originAllowed(r) {
		t.Fatal(`"*" não pode liberar qualquer origem`)
	}
}
