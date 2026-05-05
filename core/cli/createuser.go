package cli

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"kyrux/core/database"
	"kyrux/core/environment"
	"kyrux/core/orm"
	"kyrux/core/security/auth"
	"kyrux/core/security/crypton"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

// ── createsuperuser / createuser ──────────────────────────────────────────────

func runCreateSuperuser() error {
	return createUserCLI(true, true)
}

func runCreateUser() error {
	return createUserCLI(false, false)
}

func createUserCLI(isAdmin, isStaff bool) error {
	_ = environment.Load(".env")

	if environment.GetOr("DB_ENABLED", "false") != "true" {
		return fmt.Errorf("DB_ENABLED=false — banco de dados não configurado")
	}

	driver := environment.GetOr("DB_DRIVER", "postgres")
	dsn := environment.Get("DB_DSN")
	if dsn == "" {
		return fmt.Errorf("DB_DSN não configurado no .env")
	}

	db, err := database.Open(driver, dsn)
	if err != nil {
		return fmt.Errorf("conectar ao banco: %w", err)
	}
	defer db.Close()

	crypton.SetPepper(environment.Get("PASSWORD_PEPPER"))

	label := "Usuário"
	if isAdmin {
		label = "Superusuário"
	}

	fmt.Printf("\n── Criar %s %s\n\n", label, strings.Repeat("─", 40-len(label)))

	loginField := auth.LoginFieldName() // "Username" ou "Email"

	var username, email string

	if loginField == "Email" {
		// E-mail é o campo de login — obrigatório
		email, err = cuPrompt("E-mail")
		if err != nil {
			return err
		}
		if err := cuCheckUnique(db, "email", email); err != nil {
			return err
		}
		// Username é opcional
		username, err = cuPromptOptional("Username")
		if err != nil {
			return err
		}
		if username != "" {
			if err := cuCheckUnique(db, "username", username); err != nil {
				return err
			}
		}
	} else {
		// Username é o campo de login — obrigatório (padrão)
		username, err = cuPrompt("Username")
		if err != nil {
			return err
		}
		if err := cuCheckUnique(db, "username", username); err != nil {
			return err
		}
		// E-mail é opcional
		email, err = cuPromptOptional("E-mail")
		if err != nil {
			return err
		}
		if email != "" {
			if err := cuCheckUnique(db, "email", email); err != nil {
				return err
			}
		}
	}

	firstName, err := cuPromptOptional("Primeiro nome")
	if err != nil {
		return err
	}

	lastName, err := cuPromptOptional("Sobrenome")
	if err != nil {
		return err
	}

	// Staff só é perguntado para usuários comuns — superusuário já é staff
	if !isAdmin {
		isStaff, err = cuPromptBool("É staff?")
		if err != nil {
			return err
		}
	}

	password, err := cuPromptPassword()
	if err != nil {
		return err
	}

	// E-mail é *string: nil quando não informado (evita conflito no índice UNIQUE)
	var emailPtr *string
	if email != "" {
		emailPtr = &email
	}

	user := &auth.User{
		UUID:      cuGenerateUUID(),
		Username:  username,
		Email:     emailPtr,
		FirstName: firstName,
		LastName:  lastName,
		IsAdmin:   isAdmin,
		IsStaff:   isStaff,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := user.SetPassword(password); err != nil {
		return fmt.Errorf("hash da senha: %w", err)
	}

	if err := orm.Create(db, user); err != nil {
		return fmt.Errorf("criar usuário: %w", err)
	}

	loginValue := username
	if loginField == "Email" {
		loginValue = email
	}
	fmt.Printf("\n%s '%s' criado com sucesso (ID: %d).\n\n", label, loginValue, user.ID)
	return nil
}

// ── helpers de input ──────────────────────────────────────────────────────────

var cuReader = bufio.NewReader(os.Stdin)

func cuReadLine() (string, error) {
	line, err := cuReader.ReadString('\n')
	return strings.TrimSpace(line), err
}

func cuPrompt(label string) (string, error) {
	for {
		fmt.Printf("%s: ", label)
		val, err := cuReadLine()
		if err != nil {
			return "", err
		}
		if val != "" {
			return val, nil
		}
		fmt.Printf("  %s é obrigatório.\n", label)
	}
}

func cuPromptOptional(label string) (string, error) {
	fmt.Printf("%s (opcional): ", label)
	val, err := cuReadLine()
	return val, err
}

func cuPromptBool(label string) (bool, error) {
	fmt.Printf("%s [s/N]: ", label)
	val, err := cuReadLine()
	if err != nil {
		return false, err
	}
	return strings.EqualFold(val, "s"), nil
}

func cuPromptPassword() (string, error) {
	for {
		fmt.Print("Senha: ")
		p1, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("ler senha: %w", err)
		}

		fmt.Print("Senha (confirmação): ")
		p2, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("ler confirmação: %w", err)
		}

		if string(p1) != string(p2) {
			fmt.Println("  As senhas não conferem. Tente novamente.\n")
			continue
		}

		password := strings.TrimSpace(string(p1))
		if len(password) < 8 {
			fmt.Println("  A senha deve ter ao menos 8 caracteres.\n")
			continue
		}

		return password, nil
	}
}

// ── helpers de DB e UUID ──────────────────────────────────────────────────────

func cuCheckUnique(db *database.DB, col, val string) error {
	n, err := orm.From[auth.User](db).Where(col+" = ?", val).Count()
	if err != nil {
		return fmt.Errorf("verificar %s: %w", col, err)
	}
	if n > 0 {
		return fmt.Errorf("%s '%s' já está em uso", col, val)
	}
	return nil
}

func cuGenerateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
