package orm

import (
	"database/sql"
	"fmt"
	"kyrux/core/database"
	"kyrux/core/security/crypton"
	"sort"
	"strings"
)

// Query é um builder fluente de consultas SQL para o tipo T.
// Construa com orm.From[T](connName) ou orm.FromDB[T](db) e encadeie os métodos antes de executar.
type Query[T any] struct {
	db      *database.DB
	meta    *ModelMeta
	where   []string
	args    []any
	orderBy string
	limit   int
	offset  int
}

// Where adiciona uma condição AND à cláusula WHERE.
// Use ? como placeholder; para PostgreSQL são reescritos para $N automaticamente.
//
//	q.Where("active = ?", true).Where("age > ?", 18)
func (q *Query[T]) Where(cond string, args ...any) *Query[T] {
	q.where = append(q.where, cond)
	q.args = append(q.args, args...)
	return q
}

// OrderBy define a cláusula ORDER BY (ex: "created_at DESC").
func (q *Query[T]) OrderBy(col string) *Query[T] {
	q.orderBy = col
	return q
}

// Limit define o número máximo de linhas retornadas.
func (q *Query[T]) Limit(n int) *Query[T] {
	q.limit = n
	return q
}

// Offset define o número de linhas a pular — use junto com Limit para paginação.
func (q *Query[T]) Offset(n int) *Query[T] {
	q.offset = n
	return q
}

// All executa a query e retorna todas as linhas encontradas.
func (q *Query[T]) All() ([]T, error) {
	sqlStr, args := q.buildSelect(0)
	rows, err := func() (*sql.Rows, error) {
		if stmt, perr := q.db.PrepareCached(sqlStr); perr == nil {
			return stmt.Query(args...)
		}
		return q.db.Query(sqlStr, args...)
	}()
	if err != nil {
		return nil, fmt.Errorf("orm: all: %w", err)
	}
	defer rows.Close()
	return scanRows[T](rows, q.meta)
}

// First retorna a primeira linha encontrada.
// Retorna sql.ErrNoRows se nenhuma linha corresponder.
func (q *Query[T]) First() (*T, error) {
	sqlStr, args := q.buildSelect(1)
	rows, err := func() (*sql.Rows, error) {
		if stmt, perr := q.db.PrepareCached(sqlStr); perr == nil {
			return stmt.Query(args...)
		}
		return q.db.Query(sqlStr, args...)
	}()
	if err != nil {
		return nil, fmt.Errorf("orm: first: %w", err)
	}
	defer rows.Close()
	results, err := scanRows[T](rows, q.meta)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, sql.ErrNoRows
	}
	return &results[0], nil
}

// Count retorna o número de linhas que correspondem ao filtro atual.
func (q *Query[T]) Count() (int64, error) {
	var sb strings.Builder
	sb.WriteString("SELECT COUNT(*) FROM ")
	sb.WriteString(qualifiedTable(q.db, q.meta.Table))
	if len(q.where) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(q.where, " AND "))
	}
	sqlStr := rewritePlaceholders(q.db.Driver, sb.String())
	var n int64
	if stmt, perr := q.db.PrepareCached(sqlStr); perr == nil {
		if err := stmt.QueryRow(q.args...).Scan(&n); err != nil {
			return 0, fmt.Errorf("orm: count: %w", err)
		}
	} else {
		if err := q.db.QueryRow(sqlStr, q.args...).Scan(&n); err != nil {
			return 0, fmt.Errorf("orm: count: %w", err)
		}
	}
	return n, nil
}

// Update atualiza as colunas de values para as linhas que correspondem ao WHERE.
// Exige ao menos uma cláusula WHERE para evitar atualizações acidentais globais.
func (q *Query[T]) Update(values map[string]any) error {
	if len(q.where) == 0 {
		return fmt.Errorf("orm: update sem WHERE não é permitido")
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Índice coluna → Field para aplicar hash/encrypt.
	colMeta := make(map[string]Field, len(q.meta.Fields))
	for _, f := range q.meta.Fields {
		colMeta[f.Column] = f
	}

	setClauses := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)+len(q.args))
	for _, col := range keys {
		setClauses = append(setClauses, col+" = ?")
		val := values[col]
		if f, ok := colMeta[col]; ok {
			if f.IsHash {
				if s, ok2 := val.(string); ok2 && !strings.HasPrefix(s, "$argon2id$") {
					if hashed, err := crypton.HashPassword(s); err == nil {
						val = hashed
					}
				}
			} else if f.IsEncrypt {
				if s, ok2 := val.(string); ok2 {
					if enc, err := crypton.Encrypt(s); err == nil {
						val = enc
					}
				}
			}
		}
		args = append(args, val)
	}
	args = append(args, q.args...)

	sqlStr := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		qualifiedTable(q.db, q.meta.Table),
		strings.Join(setClauses, ", "),
		strings.Join(q.where, " AND "),
	)
	if _, err := q.db.Exec(rewritePlaceholders(q.db.Driver, sqlStr), args...); err != nil {
		return fmt.Errorf("orm: update: %w", err)
	}
	return nil
}

// Delete remove as linhas que correspondem ao WHERE.
// Exige ao menos uma cláusula WHERE para evitar deleções acidentais globais.
func (q *Query[T]) Delete() error {
	if len(q.where) == 0 {
		return fmt.Errorf("orm: delete sem WHERE não é permitido")
	}
	sqlStr := fmt.Sprintf("DELETE FROM %s WHERE %s",
		qualifiedTable(q.db, q.meta.Table),
		strings.Join(q.where, " AND "),
	)
	if _, err := q.db.Exec(rewritePlaceholders(q.db.Driver, sqlStr), q.args...); err != nil {
		return fmt.Errorf("orm: delete: %w", err)
	}
	return nil
}

// Page contém o resultado paginado de uma consulta.
type Page[T any] struct {
	Items      []T
	Total      int64
	Page       int
	PageSize   int
	TotalPages int
	HasNext    bool
	HasPrev    bool
}

// Paginate executa a consulta com paginação e retorna uma Page[T] com dados e metadados.
// page começa em 1; pageSize define o número de itens por página.
// Os filtros Where e OrderBy aplicados antes são respeitados.
func (q *Query[T]) Paginate(page, pageSize int) (Page[T], error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	total, err := q.Count()
	if err != nil {
		return Page[T]{}, fmt.Errorf("orm: paginate: %w", err)
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 {
		totalPages = 1
	}

	offset := (page - 1) * pageSize
	sqlStr, args := q.buildSelectPage(pageSize, offset)
	rows, err := func() (*sql.Rows, error) {
		if stmt, perr := q.db.PrepareCached(sqlStr); perr == nil {
			return stmt.Query(args...)
		}
		return q.db.Query(sqlStr, args...)
	}()
	if err != nil {
		return Page[T]{}, fmt.Errorf("orm: paginate: %w", err)
	}
	defer rows.Close()

	items, err := scanRows[T](rows, q.meta)
	if err != nil {
		return Page[T]{}, err
	}

	return Page[T]{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		HasNext:    page < totalPages,
		HasPrev:    page > 1,
	}, nil
}

// buildSelect monta o SQL SELECT respeitando os filtros, ordem e limite.
// O parâmetro forceLimit substitui q.limit quando > 0 (usado por First).
func (q *Query[T]) buildSelect(forceLimit int) (string, []any) {
	var sb strings.Builder
	sb.WriteString("SELECT * FROM ")
	sb.WriteString(qualifiedTable(q.db, q.meta.Table))

	if len(q.where) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(q.where, " AND "))
	}
	if q.orderBy != "" {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(q.orderBy)
	}
	lim := q.limit
	if forceLimit > 0 {
		lim = forceLimit
	}
	if lim > 0 {
		fmt.Fprintf(&sb, " LIMIT %d", lim)
	}
	if q.offset > 0 {
		fmt.Fprintf(&sb, " OFFSET %d", q.offset)
	}
	return rewritePlaceholders(q.db.Driver, sb.String()), q.args
}

// buildSelectPage monta o SELECT com LIMIT/OFFSET fixos, ignorando q.limit e q.offset.
func (q *Query[T]) buildSelectPage(pageSize, offset int) (string, []any) {
	var sb strings.Builder
	sb.WriteString("SELECT * FROM ")
	sb.WriteString(qualifiedTable(q.db, q.meta.Table))

	if len(q.where) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(q.where, " AND "))
	}
	if q.orderBy != "" {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(q.orderBy)
	}
	fmt.Fprintf(&sb, " LIMIT %d OFFSET %d", pageSize, offset)
	return rewritePlaceholders(q.db.Driver, sb.String()), q.args
}
