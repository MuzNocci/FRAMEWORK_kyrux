package orm

import (
	"testing"

	"kyrux/core/security/crypton"

	_ "modernc.org/sqlite"
)

type scannerEncTeste struct {
	ID     int64  `kyrux:"pk"`
	Secret string `kyrux:"encrypt"`
}

// TestScanDecryptFailClosed garante que uma falha de decrypt (chave errada
// ou dado corrompido) aborta o scan com erro, em vez de devolver o struct
// com o ciphertext bruto no campo — que passaria despercebido para o
// chamador como se fosse o valor real.
func TestScanDecryptFailClosed(t *testing.T) {
	db := openMemDB(t)
	if err := EnsureSQLiteTable[scannerEncTeste](db); err != nil {
		t.Fatal(err)
	}

	crypton.SetEncryptionKey("chave-correta-de-teste")
	if err := Create(db, &scannerEncTeste{Secret: "segredo"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Troca a chave — decrypt da linha recém-criada passa a falhar.
	crypton.SetEncryptionKey("chave-diferente-de-teste")
	t.Cleanup(func() { crypton.SetEncryptionKey("") })

	_, err := FromDB[scannerEncTeste](db).First()
	if err == nil {
		t.Fatal("esperava erro: decrypt com chave errada deveria falhar de forma fail-closed, não devolver o ciphertext em silêncio")
	}
}

// TestScanDecryptOK garante que o caminho feliz (chave correta) continua
// decifrando normalmente — a mudança para fail-closed não deveria afetar isso.
func TestScanDecryptOK(t *testing.T) {
	db := openMemDB(t)
	if err := EnsureSQLiteTable[scannerEncTeste](db); err != nil {
		t.Fatal(err)
	}

	crypton.SetEncryptionKey("chave-correta-de-teste")
	t.Cleanup(func() { crypton.SetEncryptionKey("") })

	if err := Create(db, &scannerEncTeste{Secret: "segredo"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := FromDB[scannerEncTeste](db).First()
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if got.Secret != "segredo" {
		t.Errorf("esperava decrypt = %q, recebeu %q", "segredo", got.Secret)
	}
}
