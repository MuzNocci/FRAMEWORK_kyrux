package database

import (
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // driver puro-Go (sem CGO) — único driver embutido pelo framework
)

// OpenSQLiteFallback abre (criando se necessário, incluindo a pasta) um banco
// SQLite local em path. Usado exclusivamente pelo bootstrap quando o admin é
// habilitado sem nenhum banco configurado — é a ÚNICA situação em que o
// Kyrux abre um banco por conta própria; para qualquer banco real, o driver
// continua sendo escolha e responsabilidade do desenvolvedor (import blank
// no main.go, DSN no .env).
func OpenSQLiteFallback(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return nil, err
		}
	}
	return open("sqlite", path)
}
