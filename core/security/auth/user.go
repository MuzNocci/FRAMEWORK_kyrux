package auth

import (
	"errors"
	"kyrux/core/security/crypton"
	"time"
)

var (
	ErrUserNotFound  = errors.New("user not found")
	ErrWrongPassword = errors.New("wrong password")
	ErrInactiveUser  = errors.New("user is inactive")
)

// User é o model padrão de usuário do sistema.
// Tabela: users (gerada automaticamente pelo ORM via pluralSnake).
//
// O campo Group usa column:user_group para evitar conflito com a palavra reservada SQL GROUP.
//
// A tag "login" marca qual campo é usado para autenticação (Login/GetUser).
// Só um campo pode ter "login". Não altere após o primeiro migrate — equivale a uma
// mudança de schema que exige nova migration.
type User struct {
	ID        int64     `kyrux:"column:id,pk"`
	UUID      string    `kyrux:"column:uuid,size:36"`
	FirstName string    `kyrux:"column:first_name,size:150"`
	LastName  string    `kyrux:"column:last_name,size:150"`
	Username  string    `kyrux:"column:username,size:150,unique,login"`
	Email     *string   `kyrux:"column:email,size:254,unique"`
	Password  string    `kyrux:"column:password,size:128"`
	Group     string    `kyrux:"column:user_group,size:100"`
	IsAdmin   bool      `kyrux:"column:is_admin"`
	IsStaff   bool      `kyrux:"column:is_staff"`
	IsActive  bool      `kyrux:"column:is_active,default:true"`
	CreatedAt time.Time `kyrux:"column:created_at"`
	UpdatedAt time.Time `kyrux:"column:updated_at"`
}

// SetPassword faz o hash de plain e armazena em u.Password.
func (u *User) SetPassword(plain string) error {
	hash, err := crypton.HashPassword(plain)
	if err != nil {
		return err
	}
	u.Password = hash
	return nil
}

// CheckPassword verifica se plain corresponde ao hash armazenado.
func (u *User) CheckPassword(plain string) bool {
	return crypton.CheckPassword(plain, u.Password)
}

// FullName retorna o nome completo do usuário.
func (u *User) FullName() string {
	if u.FirstName == "" {
		return u.LastName
	}
	if u.LastName == "" {
		return u.FirstName
	}
	return u.FirstName + " " + u.LastName
}
