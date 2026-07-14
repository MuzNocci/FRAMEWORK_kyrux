package admin

import (
	"testing"

	"kyrux/core/database"

	_ "modernc.org/sqlite"
)

func TestEnsureAllTablesCriaTabelasDosModelsRegistrados(t *testing.T) {
	resetRegistry()
	Register[testProduto]("produtos", "Produtos")

	db, err := database.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	defer db.Close()

	if err := EnsureAllTables(db); err != nil {
		t.Fatalf("EnsureAllTables: %v", err)
	}

	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='test_produtos'").Scan(&name)
	if err != nil {
		t.Fatalf("tabela test_produtos não foi criada: %v", err)
	}
}

func TestEnsureAllTablesSemModelsNaoFalha(t *testing.T) {
	resetRegistry()
	db, err := database.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	defer db.Close()

	if err := EnsureAllTables(db); err != nil {
		t.Errorf("sem models registrados, EnsureAllTables não deveria falhar: %v", err)
	}
}
