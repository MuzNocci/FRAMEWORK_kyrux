package orm

import (
	"database/sql"
	"errors"
	"fmt"
	"kyrux/core/database"
	"kyrux/core/security/crypton"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	"modernc.org/sqlite"
)

// bulkChunkSize limita linhas por INSERT no CreateAll — mantém o número de
// placeholders abaixo dos limites dos drivers (PostgreSQL: 65535).
const bulkChunkSize = 500

// Create insere model no banco e preenche o campo PK com o ID gerado.
// Passe sempre um ponteiro para que o PK seja preenchido de volta:
//
//	user := User{Name: "Maria"}
//	err := orm.Create(db, &user)
//	fmt.Println(user.ID) // preenchido
func Create(db *database.DB, model any) error {
	return createInto(db, db.Driver, db.Schema, model)
}

// CreateTx insere model dentro de uma transação (database.Tx) —
// use com fw.DB.Use().Transaction para escrita atômica.
func CreateTx(tx *database.Tx, model any) error {
	return createInto(tx, tx.Driver, tx.Schema, model)
}

// nonPKCols devolve as colunas de INSERT (todas exceto a PK).
func nonPKCols(meta *ModelMeta) []string {
	cols := make([]string, 0, len(meta.Fields))
	for _, f := range meta.Fields {
		if !f.IsPK {
			cols = append(cols, f.Column)
		}
	}
	return cols
}

// rowValues monta placeholders e args de UMA linha de INSERT, aplicando
// default SQL para zero values e hash/encrypt (fail-closed: erro aborta).
func rowValues(meta *ModelMeta, v reflect.Value) (phs []string, args []any, err error) {
	phs = make([]string, 0, len(meta.Fields))
	args = make([]any, 0, len(meta.Fields))
	for _, f := range meta.Fields {
		if f.IsPK {
			continue
		}
		val := v.Field(f.GoIndex).Interface()

		// Zero value com default definido (inclui autonow): usa o default SQL.
		if f.Default != "" && isZeroValue(val) {
			phs = append(phs, f.Default)
			continue
		}

		if f.IsHash {
			if s, ok := val.(string); ok && !strings.HasPrefix(s, "$argon2id$") {
				hashed, herr := crypton.HashPassword(s)
				if herr != nil {
					return nil, nil, fmt.Errorf("orm: hash campo %s: %w", f.Column, herr)
				}
				val = hashed
			}
		} else if f.IsEncrypt {
			if s, ok := val.(string); ok {
				enc, eerr := crypton.Encrypt(s)
				if eerr != nil {
					return nil, nil, fmt.Errorf("orm: encrypt campo %s: %w", f.Column, eerr)
				}
				val = enc
			}
		}

		phs = append(phs, "?")
		args = append(args, val)
	}
	return phs, args, nil
}

// createInto é o núcleo do INSERT — funciona sobre conexão ou transação.
func createInto(exec sqlExec, driver, schema string, model any) error {
	t := reflect.TypeOf(model)
	v := reflect.ValueOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		v = v.Elem()
	}
	meta := cachedMeta(t)

	cols := nonPKCols(meta)
	phs, args, err := rowValues(meta, v)
	if err != nil {
		return err
	}

	table := qualifiedTable(schema, meta.Table)
	sqlStr := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(cols, ", "),
		strings.Join(phs, ", "),
	)

	// PostgreSQL: RETURNING evita um round-trip extra para buscar o PK.
	if isPG(driver) && meta.PKField != nil {
		sqlStr += " RETURNING " + meta.PKField.Column
		row := queryRowCached(exec, rewritePlaceholders(driver, sqlStr), args)
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

	result, err := execCached(exec, rewritePlaceholders(driver, sqlStr), args)
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

// CreateAll insere todos os models em INSERTs multi-VALUES (chunks de 500) —
// um round-trip para N linhas em vez de N (equivalente ao bulk_create do Django).
// No PostgreSQL os PKs são preenchidos de volta (RETURNING); nos demais
// drivers os structs não recebem o ID gerado (limitação em INSERT múltiplo).
func CreateAll[T any](db *database.DB, models []*T) error {
	return createAllInto(db, db.Driver, db.Schema, models)
}

// CreateAllTx é a variante de CreateAll dentro de uma transação.
func CreateAllTx[T any](tx *database.Tx, models []*T) error {
	return createAllInto(tx, tx.Driver, tx.Schema, models)
}

func createAllInto[T any](exec sqlExec, driver, schema string, models []*T) error {
	if len(models) == 0 {
		return nil
	}
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil || t.Kind() != reflect.Struct {
		return fmt.Errorf("orm: createall: T deve ser um struct concreto")
	}
	meta := cachedMeta(t)
	cols := nonPKCols(meta)
	table := qualifiedTable(schema, meta.Table)

	for start := 0; start < len(models); start += bulkChunkSize {
		end := min(start+bulkChunkSize, len(models))
		if err := insertChunk(exec, driver, meta, table, cols, models[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func insertChunk[T any](exec sqlExec, driver string, meta *ModelMeta, table string, cols []string, chunk []*T) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "INSERT INTO %s (%s) VALUES ", table, strings.Join(cols, ", "))
	args := make([]any, 0, len(chunk)*len(cols))

	for i, m := range chunk {
		phs, rowArgs, err := rowValues(meta, reflect.ValueOf(m).Elem())
		if err != nil {
			return err
		}
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteByte('(')
		sb.WriteString(strings.Join(phs, ", "))
		sb.WriteByte(')')
		args = append(args, rowArgs...)
	}

	sqlStr := sb.String()
	if isPG(driver) && meta.PKField != nil {
		sqlStr += " RETURNING " + meta.PKField.Column
		rows, err := queryCached(exec, rewritePlaceholders(driver, sqlStr), args)
		if err != nil {
			return fmt.Errorf("orm: createall: %w", err)
		}
		defer rows.Close()
		for i := 0; rows.Next() && i < len(chunk); i++ {
			pkVal := reflect.ValueOf(chunk[i]).Elem().Field(meta.PKField.GoIndex)
			if pkVal.CanAddr() {
				if err := rows.Scan(pkVal.Addr().Interface()); err != nil {
					return fmt.Errorf("orm: createall: scan pk: %w", err)
				}
			} else {
				var discard any
				_ = rows.Scan(&discard)
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("orm: createall: %w", err)
		}
		return nil
	}

	if _, err := execCached(exec, rewritePlaceholders(driver, sqlStr), args); err != nil {
		return fmt.Errorf("orm: createall: %w", err)
	}
	return nil
}

// execCached, queryRowCached e queryCached são as versões de escrita de
// queryRows/scanRow (ver query.go): reaproveitam o cache de prepared
// statements da conexão (PrepareCached) quando disponível — o mesmo
// benefício que as leituras já tinham, agora também em
// Create/CreateAll/Update/Delete. Sobre uma transação (*database.Tx, que
// não implementa stmtPreparer), cai para o Exec/Query/QueryRow direto —
// idêntico ao comportamento anterior.
func execCached(exec sqlExec, sqlStr string, args []any) (sql.Result, error) {
	if p, ok := exec.(stmtPreparer); ok {
		if stmt, err := p.PrepareCached(sqlStr); err == nil {
			return stmt.Exec(args...)
		}
	}
	return exec.Exec(sqlStr, args...)
}

func queryRowCached(exec sqlExec, sqlStr string, args []any) *sql.Row {
	if p, ok := exec.(stmtPreparer); ok {
		if stmt, err := p.PrepareCached(sqlStr); err == nil {
			return stmt.QueryRow(args...)
		}
	}
	return exec.QueryRow(sqlStr, args...)
}

func queryCached(exec sqlExec, sqlStr string, args []any) (*sql.Rows, error) {
	if p, ok := exec.(stmtPreparer); ok {
		if stmt, err := p.PrepareCached(sqlStr); err == nil {
			return stmt.Query(args...)
		}
	}
	return exec.Query(sqlStr, args...)
}

// qualifiedTable prefixa o nome da tabela com o schema, se definido.
func qualifiedTable(schema, table string) string {
	if schema != "" {
		return schema + "." + table
	}
	return table
}

// isPG reporta se o driver é PostgreSQL (lib/pq ou pgx).
func isPG(driver string) bool {
	return driver == "postgres" || driver == "pgx"
}

// isUniqueViolation reporta se err veio de uma constraint UNIQUE (ou PK)
// violada — usado por GetOrCreate/UpdateOrCreate para distinguir "outro
// processo criou a linha entre o SELECT e o INSERT" (corrida esperada, trata
// como sucesso) de qualquer outro erro de banco (propaga). Checa o tipo de
// erro nativo de cada um dos três drivers embutidos no framework — sem
// fallback por string, que quebraria silenciosamente ao mudar a mensagem
// de erro do driver.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505" // unique_violation (SQLSTATE)
	}
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		return myErr.Number == 1062 // ER_DUP_ENTRY
	}
	var liteErr *sqlite.Error
	if errors.As(err, &liteErr) {
		return strings.Contains(liteErr.Error(), "UNIQUE constraint")
	}
	return false
}

// rewritePlaceholders converte ? para $N (PostgreSQL).
// Para outros drivers devolve a query sem alterações.
//
// Roda em toda query (leitura e escrita, sem cache) — por isso evita
// fmt.Fprintf (que carrega parsing de format string e boxing via
// reflection a cada chamada) em favor de strconv.Itoa direto.
func rewritePlaceholders(driver, query string) string {
	if !isPG(driver) {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 1
	for _, c := range query {
		if c == '?' {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
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
		return rv.IsZero()
	}
}
