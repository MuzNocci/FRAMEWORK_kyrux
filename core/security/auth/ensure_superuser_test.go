package auth

import (
	"testing"

	"kyrux/core/orm"
)

func TestEnsureSuperuserCriaApenasUmaVez(t *testing.T) {
	db := newUsersDB(t)

	created, err := EnsureSuperuser(db, "admin", "senha-provisoria")
	if err != nil {
		t.Fatalf("EnsureSuperuser: %v", err)
	}
	if !created {
		t.Fatal("esperava created=true na primeira chamada")
	}

	u, err := orm.FromDB[User](db).Where("username = ?", "admin").First()
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !u.IsAdmin || !u.IsStaff || !u.IsActive {
		t.Errorf("superusuário deveria ter IsAdmin=IsStaff=IsActive=true, recebeu %+v", u)
	}
	if !u.CheckPassword("senha-provisoria") {
		t.Error("senha do superusuário criado não confere")
	}

	// Segunda chamada com senha DIFERENTE não deve recriar nem sobrescrever
	// a senha da conta já existente (evita reset silencioso a cada boot).
	created, err = EnsureSuperuser(db, "admin", "outra-senha-qualquer")
	if err != nil {
		t.Fatalf("EnsureSuperuser (segunda chamada): %v", err)
	}
	if created {
		t.Fatal("esperava created=false quando o login já existe")
	}

	n, err := orm.FromDB[User](db).Where("username = ?", "admin").Count()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("esperava exatamente 1 usuário 'admin', encontrou %d", n)
	}

	again, err := orm.FromDB[User](db).Where("username = ?", "admin").First()
	if err != nil {
		t.Fatalf("first (segunda checagem): %v", err)
	}
	if !again.CheckPassword("senha-provisoria") {
		t.Error("senha original deveria continuar válida — EnsureSuperuser não deve resetar senha de conta existente")
	}
}

func TestEnsureSuperuserSenhaCurtaFalha(t *testing.T) {
	db := newUsersDB(t)

	if _, err := EnsureSuperuser(db, "admin", "curta"); err == nil {
		t.Fatal("esperava erro para senha com menos de 8 caracteres")
	}

	n, err := orm.FromDB[User](db).Count()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatal("nenhum usuário deveria ter sido criado com senha inválida")
	}
}

func TestEnsureSuperuserVazioNaoFazNada(t *testing.T) {
	db := newUsersDB(t)

	created, err := EnsureSuperuser(db, "", "")
	if err != nil {
		t.Fatalf("EnsureSuperuser com valores vazios não deveria retornar erro: %v", err)
	}
	if created {
		t.Fatal("não deveria criar nada com login/senha vazios")
	}
}
