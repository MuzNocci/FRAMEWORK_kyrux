package orm

import (
	"database/sql"
	"fmt"
	"kyrux/core/security/crypton"
	"reflect"
)

// scanPlan é o mapeamento coluna→campo computado UMA vez por result set:
// índice do campo Go por coluna (-1 = descarta) e posições cifradas.
// Sem isso, cada linha pagaria linhas×colunas lookups de map.
type scanPlan struct {
	cols     []string
	fieldIdx []int
	encCols  []int
}

func buildScanPlan(cols []string, meta *ModelMeta) scanPlan {
	plan := scanPlan{cols: cols, fieldIdx: make([]int, len(cols))}
	for i, col := range cols {
		plan.fieldIdx[i] = -1
		if f, ok := meta.ColToField[col]; ok {
			plan.fieldIdx[i] = f.GoIndex
			if f.IsEncrypt {
				plan.encCols = append(plan.encCols, i)
			}
		}
	}
	return plan
}

// scanOne escaneia a linha atual de rows em um novo T seguindo o plano.
func scanOne[T any](rows *sql.Rows, plan scanPlan, dests []any, discard *any) (T, error) {
	var zero T
	v := reflect.ValueOf(&zero).Elem()

	for i := range plan.cols {
		if gi := plan.fieldIdx[i]; gi >= 0 {
			fv := v.Field(gi)
			if fv.CanAddr() {
				dests[i] = fv.Addr().Interface()
				continue
			}
		}
		// Coluna sem campo correspondente: descarta silenciosamente.
		dests[i] = discard
	}

	if err := rows.Scan(dests...); err != nil {
		return zero, fmt.Errorf("orm: scan: %w", err)
	}

	// Decifra campos marcados com kyrux:"encrypt". Fail-closed: chave errada
	// ou dado corrompido abortam o scan — nunca devolvem o struct com
	// ciphertext bruto no campo, o que passaria despercebido para o
	// chamador como se fosse o valor real.
	for _, i := range plan.encCols {
		fv := v.Field(plan.fieldIdx[i])
		if fv.Kind() == reflect.String && fv.CanSet() {
			dec, err := crypton.Decrypt(fv.String())
			if err != nil {
				return zero, fmt.Errorf("orm: decrypt coluna %s: %w", plan.cols[i], err)
			}
			fv.SetString(dec)
		}
	}
	return zero, nil
}

// scanRows converte sql.Rows em []T mapeando colunas para campos do struct via ModelMeta.
func scanRows[T any](rows *sql.Rows, meta *ModelMeta) ([]T, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("orm: columns: %w", err)
	}
	plan := buildScanPlan(cols, meta)
	dests := make([]any, len(cols))
	var discard any
	var results []T

	for rows.Next() {
		item, err := scanOne[T](rows, plan, dests, &discard)
		if err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: rows: %w", err)
	}
	return results, nil
}

// eachRow itera rows chamando fn por linha — streaming, memória O(1).
func eachRow[T any](rows *sql.Rows, meta *ModelMeta, fn func(*T) error) error {
	cols, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("orm: columns: %w", err)
	}
	plan := buildScanPlan(cols, meta)
	dests := make([]any, len(cols))
	var discard any

	for rows.Next() {
		item, err := scanOne[T](rows, plan, dests, &discard)
		if err != nil {
			return err
		}
		if err := fn(&item); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("orm: rows: %w", err)
	}
	return nil
}
