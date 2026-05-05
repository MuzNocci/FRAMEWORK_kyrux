package orm

import (
	"fmt"
	"kyrux/core/database"
	"kyrux/core/security/crypton"
	"reflect"
	"strings"
)

// Create insere model no banco e preenche o campo PK com o ID gerado.
// Passe sempre um ponteiro para que o PK seja preenchido de volta:
//
//	user := User{Name: "Maria"}
//	err := orm.Create(db, &user)
//	fmt.Println(user.ID) // preenchido
func Create(db *database.DB, model any) error {
	t := reflect.TypeOf(model)
	v := reflect.ValueOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		v = v.Elem()
	}
	meta := cachedMeta(t)

	cols := make([]string, 0, len(meta.Fields))
	phs := make([]string, 0, len(meta.Fields))
	args := make([]any, 0, len(meta.Fields))

	for _, f := range meta.Fields {
		if f.IsPK {
			continue
		}
		cols = append(cols, f.Column)

		val := v.Field(f.GoIndex).Interface()

		// Se o valor for zero e houver default, usar o default do SQL
		if f.Default != "" && isZeroValue(val) {
			phs = append(phs, f.Default)
		} else {
			phs = append(phs, "?")
		}

		if f.IsHash {
			if s, ok := val.(string); ok && !strings.HasPrefix(s, "$argon2id$") {
				hashed, err := crypton.HashPassword(s)
				if err != nil {
					return fmt.Errorf("orm: hash campo %s: %w", f.Column, err)
				}
				val = hashed
			}
		} else if f.IsEncrypt {
			if s, ok := val.(string); ok {
				enc, err := crypton.Encrypt(s)
				if err != nil {
					return fmt.Errorf("orm: encrypt campo %s: %w", f.Column, err)
				}
				val = enc
			}
		}

		// Só adiciona arg se não for usando default SQL
		if !(f.Default != "" && isZeroValue(v.Field(f.GoIndex).Interface())) {
			args = append(args, val)
		}
	}

	table := qualifiedTable(db, meta.Table)
	sqlStr := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(cols, ", "),
		strings.Join(phs, ", "),
	)

	// PostgreSQL: RETURNING evita um round-trip extra para buscar o PK.
	if isPG(db.Driver) && meta.PKField != nil {
		sqlStr += " RETURNING " + meta.PKField.Column
		row := db.QueryRow(rewritePlaceholders(db.Driver, sqlStr), args...)
		pkVal := v.Field(meta.PKField.GoIndex)
		if pkVal.CanAddr() {
			if err := row.Scan(pkVal.Addr().Interface()); err != nil {
				return fmt.Errorf("orm: create: %w", err)
			}
			return nil
		}
		var discard any
		return row.Scan(&discard)
	}

	result, err := db.Exec(rewritePlaceholders(db.Driver, sqlStr), args...)
	if err != nil {
		return fmt.Errorf("orm: create: %w", err)
	}
	// MySQL / SQLite retornam LastInsertId.
	if meta.PKField != nil {
		if id, err := result.LastInsertId(); err == nil {
			pkVal := v.Field(meta.PKField.GoIndex)
			if pkVal.CanSet() {
				pkVal.SetInt(id)
			}
		}
	}
	return nil
}

// qualifiedTable prefixa o nome da tabela com o schema, se definido.
func qualifiedTable(db *database.DB, table string) string {
	if db.Schema != "" {
		return db.Schema + "." + table
	}
	return table
}

// isPG reporta se o driver é PostgreSQL (lib/pq ou pgx).
func isPG(driver string) bool {
	return driver == "postgres" || driver == "pgx"
}

// rewritePlaceholders converte ? para $N (PostgreSQL).
// Para outros drivers devolve a query sem alterações.
func rewritePlaceholders(driver, query string) string {
	if !isPG(driver) {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 1
	for _, c := range query {
		if c == '?' {
			fmt.Fprintf(&b, "$%d", n)
			n++
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// isZeroValue reporta se um valor é o zero value do seu tipo.
func isZeroValue(val any) bool {
	if val == nil {
		return true
	}
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Ptr {
		return rv.IsNil()
	}
	switch v := val.(type) {
	case string:
		return v == ""
	case int, int8, int16, int32, int64:
		return v == 0
	case uint, uint8, uint16, uint32, uint64:
		return v == 0
	case float32, float64:
		return v == 0
	case bool:
		return !v
	case []byte:
		return len(v) == 0
	default:
		return false
	}
}
