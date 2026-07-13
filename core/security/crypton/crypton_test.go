package crypton

import (
	"strings"
	"testing"
)

// TestEncryptFailClosedSemChave garante que sem chave configurada a
// criptografia FALHA em vez de derivar uma chave de string vazia (que seria
// um segredo público — SHA-256 de "").
func TestEncryptFailClosedSemChave(t *testing.T) {
	SetEncryptionKey("") // desativa
	if HasEncryptionKey() {
		t.Fatal("chave vazia não deveria contar como configurada")
	}
	if _, err := Encrypt("segredo"); err == nil {
		t.Error("Encrypt sem chave deveria retornar erro (fail-closed)")
	}
	if _, err := Decrypt("$enc$abc"); err == nil {
		t.Error("Decrypt sem chave deveria retornar erro")
	}
}

func TestEncryptRoundTrip(t *testing.T) {
	SetEncryptionKey("uma-chave-forte-de-teste-32bytes!!")
	defer SetEncryptionKey("")

	plain := "CPF: 123.456.789-00"
	enc, err := Encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, encPrefix) {
		t.Errorf("cifra deveria ter prefixo %q", encPrefix)
	}
	if enc == plain || strings.Contains(enc, "123.456") {
		t.Error("texto claro não deveria aparecer na cifra")
	}
	dec, err := Decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != plain {
		t.Errorf("round-trip falhou: %q != %q", dec, plain)
	}
}

func TestEncryptIdempotente(t *testing.T) {
	SetEncryptionKey("uma-chave-forte-de-teste-32bytes!!")
	defer SetEncryptionKey("")
	enc, _ := Encrypt("x")
	again, err := Encrypt(enc) // já cifrado
	if err != nil || again != enc {
		t.Errorf("Encrypt deveria ser idempotente sobre valor já cifrado")
	}
}
