package auth

import (
	"strings"
	"testing"
	"time"

	"kyrux/core/database"
	"kyrux/core/orm"
	"kyrux/core/security/crypton"

	_ "modernc.org/sqlite"
)

func newUsersDB(t *testing.T) *database.DB {
	t.Helper()
	crypton.SetPepper("teste-pepper-nao-usar-em-producao")
	db, err := database.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := orm.EnsureSQLiteTable[User](db); err != nil {
		t.Fatalf("criar tabela users: %v", err)
	}
	return db
}

// TestPasswordCampoHashFailClosed é o teste de regressão do bug real: um
// Update genérico via ORM (sem passar por SetPassword) NUNCA pode gravar a
// senha em texto claro — Password precisa estar marcado kyrux:"hash" para
// que o Update fail-closed do ORM entre em ação automaticamente.
func TestPasswordCampoHashFailClosed(t *testing.T) {
	db := newUsersDB(t)
	u := &User{Username: "alice", IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := orm.Create(db, u); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Update "cru" — como um dev inexperiente (ou um script de reset) faria,
	// sem chamar SetPassword antes.
	if err := orm.FromDB[User](db).Where("id = ?", u.ID).Update(map[string]any{
		"password": "senha-em-texto-claro",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := orm.FromDB[User](db).Where("id = ?", u.ID).First()
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !strings.HasPrefix(got.Password, "$argon2id$") {
		t.Fatalf("Password deveria estar hasheado (prefixo $argon2id$), recebeu: %q", got.Password)
	}
	if !got.CheckPassword("senha-em-texto-claro") {
		t.Error("CheckPassword deveria validar a senha original após o hash automático")
	}
}

// TestSetPasswordNaoEhReHasheado garante que o valor já hasheado por
// SetPassword não seja hasheado de novo pelo ORM (o prefixo $argon2id$ evita
// isso tanto em Create quanto em Update).
func TestSetPasswordNaoEhReHasheado(t *testing.T) {
	db := newUsersDB(t)
	u := &User{Username: "bob", IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := u.SetPassword("minhasenha123"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := orm.Create(db, u); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := orm.FromDB[User](db).Where("id = ?", u.ID).First()
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !got.CheckPassword("minhasenha123") {
		t.Error("CheckPassword deveria validar a senha definida via SetPassword")
	}

	// Update reenviando o MESMO hash (ex: um form de edição que não altera a
	// senha, mas ainda assim reenvia o valor atual) não deve gerar um
	// segundo hash em cima do primeiro.
	if err := orm.FromDB[User](db).Where("id = ?", u.ID).Update(map[string]any{
		"password": got.Password,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	again, err := orm.FromDB[User](db).Where("id = ?", u.ID).First()
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if again.Password != got.Password {
		t.Error("reenviar o hash já existente não deveria alterá-lo (double-hash)")
	}
	if !again.CheckPassword("minhasenha123") {
		t.Error("senha deveria continuar válida após o Update com o mesmo hash")
	}
}
