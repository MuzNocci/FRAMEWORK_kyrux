package orm

import (
	"database/sql"
	"fmt"
	"kyrux/core/security/crypton"
	"reflect"
)

// scanRows converte sql.Rows em []T mapeando colunas para campos do struct via ModelMeta.
func scanRows[T any](rows *sql.Rows, meta *ModelMeta) ([]T, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("orm: columns: %w", err)
	}

	colToField := meta.ColToField
	dests := make([]any, len(cols))
	var results []T

	for rows.Next() {
		var zero T
		v := reflect.ValueOf(&zero).Elem()

		for i, col := range cols {
			if f, ok := colToField[col]; ok {
				fv := v.Field(f.GoIndex)
				if fv.CanAddr() {
					dests[i] = fv.Addr().Interface()
					continue
				}
			}
			// Coluna sem campo correspondente: descarta silenciosamente.
			dests[i] = new(any)
		}

		if err := rows.Scan(dests...); err != nil {
			return nil, fmt.Errorf("orm: scan: %w", err)
		}

		// Decifra campos marcados com kyrux:"encrypt".
		for _, col := range cols {
			if f, ok := colToField[col]; ok && f.IsEncrypt {
				fv := v.Field(f.GoIndex)
				if fv.Kind() == reflect.String && fv.CanSet() {
					if dec, err := crypton.Decrypt(fv.String()); err == nil {
						fv.SetString(dec)
					}
				}
			}
		}

		results = append(results, zero)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: rows: %w", err)
	}
	return results, nil
}
