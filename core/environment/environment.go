package environment

import (
	"os"
	"strings"
)

var loaded = map[string]string{}
var rawLines []string

func Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	rawLines = make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if idx := strings.Index(value, " #"); idx != -1 {
			value = value[:idx]
		}
		value = strings.TrimSpace(value)

		rawLines = append(rawLines, key+"="+value)

		// Respeita vars já definidas no ambiente (ex: CI, testes, Docker).
		// Para chaves repetidas, a primeira ocorrência prevalece no mapa flat.
		if _, already := loaded[key]; already {
			continue
		}
		if existing := os.Getenv(key); existing != "" {
			loaded[key] = existing
		} else {
			loaded[key] = value
			os.Setenv(key, value)
		}
	}
	return nil
}

func Get(key string) string {
	if v, ok := loaded[key]; ok {
		return v
	}
	return os.Getenv(key)
}

func GetOr(key, fallback string) string {
	if v := Get(key); v != "" {
		return v
	}
	return fallback
}

// GetBlocks agrupa as linhas do .env em blocos, usando signalKey como marcador
// de início de cada bloco. Cada vez que signalKey aparece, um novo bloco começa.
//
// Exemplo com signalKey="DB_NAME":
//
//	DB_NAME=principal   ← início do bloco 1
//	DB_DRIVER=postgres
//	DB_DSN=...
//	DB_NAME=analytics   ← início do bloco 2
//	DB_DRIVER=postgres
//	DB_DSN=...
//
// Retorna []map[string]string, um map por bloco.
func GetBlocks(signalKey string) []map[string]string {
	var blocks []map[string]string
	var current map[string]string

	for _, line := range rawLines {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if key == signalKey {
			current = map[string]string{key: value}
			blocks = append(blocks, current)
		} else if current != nil {
			current[key] = value
		}
	}

	return blocks
}
