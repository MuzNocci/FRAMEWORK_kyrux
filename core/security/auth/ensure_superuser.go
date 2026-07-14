package auth

import (
	"crypto/rand"
	"fmt"
	"kyrux/core/database"
	"kyrux/core/orm"
	"time"
)

// EnsureSuperuser cria o superusuário inicial a partir de ADMIN_SUPERUSER_USERNAME/
// ADMIN_SUPERUSER_PASSWORD (ver bootstrap) se ainda não existir ninguém com esse
// login — nunca atualiza a senha de uma conta já existente, mesmo que o valor no
// .env mude entre reinícios (evita reset silencioso de senha a cada boot em dev
// com hot reload). loginValue é aplicado ao campo marcado com kyrux:"login"
// (Username ou Email, conforme o model User).
//
// Retorna (true, nil) quando cria a conta, (false, nil) quando já existia.
func EnsureSuperuser(db *database.DB, loginValue, password string) (bool, error) {
	if loginValue == "" || password == "" {
		return false, nil
	}
	if len(password) < 8 {
		return false, fmt.Errorf("senha deve ter ao menos 8 caracteres")
	}

	n, err := orm.FromDB[User](db).Where(loginColumn+" = ?", loginValue).Count()
	if err != nil {
		return false, fmt.Errorf("verificar superusuário existente: %w", err)
	}
	if n > 0 {
		return false, nil
	}

	user := &User{
		UUID:      generateUUID(),
		IsAdmin:   true,
		IsStaff:   true,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if loginFieldName == "Email" {
		user.Email = &loginValue
	} else {
		user.Username = loginValue
	}
	if err := user.SetPassword(password); err != nil {
		return false, fmt.Errorf("hash da senha: %w", err)
	}

	if err := orm.Create(db, user); err != nil {
		return false, fmt.Errorf("criar superusuário: %w", err)
	}
	return true, nil
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
