package orm

import (
	"fmt"
	"kyrux/core/database"
	"reflect"
	"strings"
	"time"
)

var ddlTimeType = reflect.TypeOf(time.Time{})

// EnsureSQLiteTable cria a tabela de T no banco SQLite db se ela ainda não
// existir (CREATE TABLE IF NOT EXISTS + índices únicos) — nunca altera uma
// tabela já existente (sem ALTER, sem detecção de mudanças). Único uso
// pretendido: o SQLite de fallback criado pelo bootstrap quando o admin é
// habilitado sem nenhum banco configurado. Para bancos reais, use
// makemigrations/migrate — migrations explícitas e revisáveis continuam
// sendo o caminho oficial de evolução de schema.
//
// Faz panic se db não for SQLite: esta função gera DDL específico do
// SQLite (INTEGER PRIMARY KEY, tipos com afinidade), incompatível com
// outros bancos.
func EnsureSQLiteTable[T any](db *database.DB) error {
	if !isSQLite(db.Driver) {
		panic(fmt.Sprintf("orm: EnsureSQLiteTable chamado com driver %q — só é válido para sqlite", db.Driver))
	}
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil || t.Kind() != reflect.Struct {
		return fmt.Errorf("orm: EnsureSQLiteTable: T deve ser um struct concreto")
	}
	meta := cachedMeta(t)

	createStmt, indexStmts := buildSQLiteCreateTable(meta, t)
	if _, err := db.Exec(createStmt); err != nil {
		return fmt.Errorf("orm: criar tabela %s: %w", meta.Table, err)
	}
	for _, stmt := range indexStmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("orm: criar índice em %s: %w", meta.Table, err)
		}
	}
	return nil
}

func isSQLite(driver string) bool {
	return driver == "sqlite" || driver == "sqlite3"
}

// buildSQLiteCreateTable monta o CREATE TABLE IF NOT EXISTS e os CREATE
// UNIQUE INDEX necessários para o model, em sintaxe SQLite.
func buildSQLiteCreateTable(meta *ModelMeta, t reflect.Type) (createStmt string, indexStmts []string) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE TABLE IF NOT EXISTS %s (\n", meta.Table)

	lines := make([]string, 0, len(meta.Fields))
	for _, f := range meta.Fields {
		sf := t.Field(f.GoIndex)
		line := "  " + f.Column + " " + sqliteColumnType(sf.Type, f)
		if !f.IsPK {
			if sf.Type.Kind() != reflect.Ptr {
				line += " NOT NULL"
			}
			if def := sqliteDefault(f.Default); def != "" {
				line += " DEFAULT " + def
			}
			if f.FK != "" {
				line += " REFERENCES " + f.FK + "(id)"
			}
		}
		lines = append(lines, line)

		if f.Unique && !f.IsPK {
			indexStmts = append(indexStmts, fmt.Sprintf(
				"CREATE UNIQUE INDEX IF NOT EXISTS %s_%s_idx ON %s (%s)",
				meta.Table, f.Column, meta.Table, f.Column,
			))
		}
	}
	sb.WriteString(strings.Join(lines, ",\n"))
	sb.WriteString("\n)")
	return sb.String(), indexStmts
}

// sqliteColumnType escolhe o tipo SQLite a partir do tipo Go do campo.
// "INTEGER PRIMARY KEY" é tratamento especial do SQLite: torna a coluna um
// alias do rowid interno e a faz auto-incrementar sozinha.
func sqliteColumnType(t reflect.Type, f Field) string {
	if f.IsPK {
		return "INTEGER PRIMARY KEY"
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return "INTEGER"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "INTEGER"
	case reflect.Float32, reflect.Float64:
		return "REAL"
	case reflect.Struct:
		if t == ddlTimeType {
			return "DATETIME"
		}
	}
	if f.Size > 0 {
		return fmt.Sprintf("VARCHAR(%d)", f.Size)
	}
	return "TEXT"
}

// sqliteDefault traduz defaults comuns de outros bancos para SQLite —
// NOW() (convenção Postgres, vista nos exemplos da documentação) não existe
// no SQLite e quebraria o CREATE TABLE; CURRENT_TIMESTAMP é o equivalente.
func sqliteDefault(def string) string {
	if strings.EqualFold(def, "NOW()") {
		return "CURRENT_TIMESTAMP"
	}
	return def
}
