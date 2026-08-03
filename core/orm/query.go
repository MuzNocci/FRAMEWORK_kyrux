package orm

import (
	"database/sql"
	"fmt"
	"kyrux/core/database"
	"kyrux/core/security/crypton"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// reIdent valida nomes de coluna e expressões ORDER BY contra SQL injection.
// Aceita: coluna, tabela.coluna, coluna ASC, coluna DESC.
var reIdent = regexp.MustCompile(`(?i)^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?(\s+(ASC|DESC))?$`)

func validIdent(s string) bool { return reIdent.MatchString(strings.TrimSpace(s)) }

// reJoinOn valida a cláusula ON de um JOIN: "tabela.coluna = tabela.coluna".
var reJoinOn = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*\.[a-zA-Z_][a-zA-Z0-9_]*\s*=\s*[a-zA-Z_][a-zA-Z0-9_]*\.[a-zA-Z_][a-zA-Z0-9_]*$`)

func validJoinOn(s string) bool { return reJoinOn.MatchString(strings.TrimSpace(s)) }

// sqlExec é satisfeito por *database.DB e *database.Tx — permite que o mesmo
// builder execute dentro e fora de transações.
type sqlExec interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// stmtPreparer é implementado por *database.DB (cache LRU de statements).
// Transações executam direto — o *sql.Tx já tem afinidade de conexão.
type stmtPreparer interface {
	PrepareCached(query string) (*sql.Stmt, error)
}

// whereClause é uma condição do WHERE com o conectivo que a liga à anterior.
type whereClause struct {
	cond string
	or   bool
}

// Query é um builder fluente de consultas SQL para o tipo T.
// Construa com orm.From[T](connName), orm.FromDB[T](db) ou orm.FromTx[T](tx)
// e encadeie os métodos antes de executar.
type Query[T any] struct {
	exec     sqlExec
	driver   string
	schema   string
	meta     *ModelMeta
	cols     []string
	joins    []string
	where    []whereClause
	args     []any
	orderBy  []string
	rankArgs []any // argumentos extras da expressão de ranking no ORDER BY — ver Search
	distinct bool
	limit    int
	offset   int
	err      error
}

// Select define as colunas a retornar (ex: "id", "first_name", "email").
// Sem chamada a Select, usa SELECT *.
// Atenção: colunas não selecionadas ficam com zero value no struct resultante.
func (q *Query[T]) Select(cols ...string) *Query[T] {
	for _, c := range cols {
		if !validIdent(c) {
			q.err = fmt.Errorf("orm: select: identificador inválido: %q", c)
			return q
		}
	}
	q.cols = cols
	return q
}

// Distinct adiciona DISTINCT ao SELECT.
func (q *Query[T]) Distinct() *Query[T] {
	q.distinct = true
	return q
}

// addJoin valida e registra uma cláusula de JOIN.
func (q *Query[T]) addJoin(kind, table, on string) *Query[T] {
	if !validIdent(table) {
		q.err = fmt.Errorf("orm: join: tabela inválida: %q", table)
		return q
	}
	if !validJoinOn(on) {
		q.err = fmt.Errorf("orm: join: cláusula ON inválida (esperado \"tabela.col = tabela.col\"): %q", on)
		return q
	}
	q.joins = append(q.joins, kind+" JOIN "+table+" ON "+on)
	return q
}

// Join adiciona um INNER JOIN — use para FILTRAR pela tabela relacionada
// (equivalente a atravessar relações no filter do Django). Com JOIN, o
// SELECT usa "tabela_base.*", então o resultado continua sendo []T sem
// conflito de colunas; qualifique as colunas do Where com o nome da tabela:
//
//	posts, _ := orm.From[Post](db).
//	    Join("users", "users.id = posts.user_id").
//	    Where("users.is_active = ?", true).
//	    All()
//
// Para CARREGAR os registros relacionados, use orm.Prefetch.
func (q *Query[T]) Join(table, on string) *Query[T] { return q.addJoin("INNER", table, on) }

// LeftJoin adiciona um LEFT JOIN — mesmas regras de Join.
func (q *Query[T]) LeftJoin(table, on string) *Query[T] { return q.addJoin("LEFT", table, on) }

// Where adiciona uma condição AND à cláusula WHERE.
// Use ? como placeholder; para PostgreSQL são reescritos para $N automaticamente.
//
//	q.Where("active = ?", true).Where("age > ?", 18)
func (q *Query[T]) Where(cond string, args ...any) *Query[T] {
	q.where = append(q.where, whereClause{cond: cond})
	q.args = append(q.args, args...)
	return q
}

// OrWhere adiciona uma condição OR à cláusula WHERE. Cada condição é
// parentesizada individualmente; a precedência entre AND e OR segue o SQL
// (AND liga mais forte).
//
//	q.Where("tipo = ?", "a").OrWhere("tipo = ?", "b") // (tipo = ?) OR (tipo = ?)
func (q *Query[T]) OrWhere(cond string, args ...any) *Query[T] {
	q.where = append(q.where, whereClause{cond: cond, or: true})
	q.args = append(q.args, args...)
	return q
}

// WhereIn adiciona "col IN (...)" com expansão automática de placeholders.
// Aceita valores variádicos ou um único slice:
//
//	q.WhereIn("id", 1, 2, 3)
//	q.WhereIn("id", ids) // ids pode ser []int64, []string, etc.
//
// Lista vazia corresponde a nenhum resultado (comportamento do Django __in=[]).
func (q *Query[T]) WhereIn(col string, vals ...any) *Query[T] {
	if !validIdent(col) {
		q.err = fmt.Errorf("orm: wherein: identificador inválido: %q", col)
		return q
	}
	if len(vals) == 1 {
		if rv := reflect.ValueOf(vals[0]); rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			expanded := make([]any, rv.Len())
			for i := range expanded {
				expanded[i] = rv.Index(i).Interface()
			}
			vals = expanded
		}
	}
	if len(vals) == 0 {
		q.where = append(q.where, whereClause{cond: "1 = 0"})
		return q
	}
	var sb strings.Builder
	sb.WriteString(col)
	sb.WriteString(" IN (")
	for i := range vals {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteByte('?')
	}
	sb.WriteByte(')')
	q.where = append(q.where, whereClause{cond: sb.String()})
	q.args = append(q.args, vals...)
	return q
}

// Search filtra por uma condição de busca full-text (linguagem natural)
// sobre uma coluna marcada com kyrux:"fts" no model, e por padrão ordena o
// resultado por relevância (mais relevante primeiro). Para desempatar por
// outro critério, encadeie OrderBy depois — ele só ACRESCENTA colunas ao
// final, a relevância continua sendo o critério primário:
//
//	orm.FromDB[Post](db).Search("conteudo", "kyrux orm").All()               // só por relevância
//	orm.FromDB[Post](db).Search("conteudo", "kyrux orm").OrderBy("id").All() // relevância, depois id
//
// Suportado nativamente em três drivers, cada um com seu próprio mecanismo
// (o índice é criado pelo makemigrations quando o campo tem kyrux:"fts"):
//   - postgres/pgx: to_tsvector/to_tsquery + índice GIN, ts_rank para ordenar.
//   - mysql: índice FULLTEXT + MATCH...AGAINST (linguagem natural).
//   - sqlite: FTS5 — tabela virtual "<tabela>_<coluna>_fts" (JOIN automático
//     pelo rowid), ordenada pela coluna oculta "rank" do FTS5.
//
// Em qualquer outro driver (sqlserver, oracle) retorna erro — não existe
// fallback silencioso para LIKE, que não é busca full-text de verdade.
func (q *Query[T]) Search(col, term string) *Query[T] {
	if !validIdent(col) {
		q.err = fmt.Errorf("orm: search: identificador inválido: %q", col)
		return q
	}
	f, ok := q.meta.ColToField[col]
	if !ok || !f.FTS {
		q.err = fmt.Errorf("orm: search: coluna %q não está marcada com kyrux:\"fts\" no model %s", col, q.meta.Table)
		return q
	}

	switch q.driver {
	case "postgres", "pgx":
		vec := fmt.Sprintf("to_tsvector('portuguese', %s)", col)
		q.where = append(q.where, whereClause{cond: vec + " @@ plainto_tsquery('portuguese', ?)"})
		q.args = append(q.args, term)
		q.orderBy = []string{fmt.Sprintf("ts_rank(%s, plainto_tsquery('portuguese', ?)) DESC", vec)}
		q.rankArgs = []any{term}
	case "mysql":
		match := fmt.Sprintf("MATCH(%s) AGAINST(? IN NATURAL LANGUAGE MODE)", col)
		q.where = append(q.where, whereClause{cond: match})
		q.args = append(q.args, term)
		q.orderBy = []string{match + " DESC"}
		q.rankArgs = []any{term}
	case "sqlite", "sqlite3":
		pkCol := "id"
		if q.meta.PKField != nil {
			pkCol = q.meta.PKField.Column
		}
		shadow := q.meta.Table + "_" + col + "_fts"
		q.addJoin("INNER", shadow, shadow+".rowid = "+q.meta.Table+"."+pkCol)
		if q.err != nil {
			return q
		}
		q.where = append(q.where, whereClause{cond: shadow + " MATCH ?"})
		q.args = append(q.args, term)
		q.orderBy = []string{shadow + ".rank"}
	default:
		q.err = fmt.Errorf("orm: search: full-text search não é suportado no driver %q (suportado: postgres, pgx, mysql, sqlite)", q.driver)
	}
	return q
}

// OrderBy define a ordenação — aceita múltiplas colunas, como no Django:
//
//	q.OrderBy("criado_em DESC", "id ASC")
func (q *Query[T]) OrderBy(cols ...string) *Query[T] {
	for _, c := range cols {
		if !validIdent(c) {
			q.err = fmt.Errorf("orm: orderby: identificador inválido: %q", c)
			return q
		}
	}
	q.orderBy = append(q.orderBy, cols...)
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

// ── execução ──────────────────────────────────────────────────────────────────

// queryRows executa via cache de statements quando disponível (conexão),
// ou direto (transação).
func (q *Query[T]) queryRows(sqlStr string, args []any) (*sql.Rows, error) {
	if p, ok := q.exec.(stmtPreparer); ok {
		if stmt, err := p.PrepareCached(sqlStr); err == nil {
			return stmt.Query(args...)
		}
	}
	return q.exec.Query(sqlStr, args...)
}

// scanRow executa a query e escaneia a única linha esperada em dest.
func (q *Query[T]) scanRow(sqlStr string, args []any, dest ...any) error {
	if p, ok := q.exec.(stmtPreparer); ok {
		if stmt, err := p.PrepareCached(sqlStr); err == nil {
			return stmt.QueryRow(args...).Scan(dest...)
		}
	}
	return q.exec.QueryRow(sqlStr, args...).Scan(dest...)
}

// All executa a query e retorna todas as linhas encontradas.
func (q *Query[T]) All() ([]T, error) {
	if q.err != nil {
		return nil, q.err
	}
	sqlStr, args := q.buildSelect(0)
	rows, err := q.queryRows(sqlStr, args)
	if err != nil {
		return nil, fmt.Errorf("orm: all: %w", err)
	}
	defer rows.Close()
	return scanRows[T](rows, q.meta)
}

// Each itera o resultado em streaming, uma linha por vez — memória O(1),
// indicado para result sets grandes (equivalente ao iterator() do Django).
// fn devolvendo erro interrompe a iteração e propaga o erro.
func (q *Query[T]) Each(fn func(*T) error) error {
	if q.err != nil {
		return q.err
	}
	sqlStr, args := q.buildSelect(0)
	rows, err := q.queryRows(sqlStr, args)
	if err != nil {
		return fmt.Errorf("orm: each: %w", err)
	}
	defer rows.Close()
	return eachRow(rows, q.meta, fn)
}

// First retorna a primeira linha encontrada.
// Retorna sql.ErrNoRows se nenhuma linha corresponder.
func (q *Query[T]) First() (*T, error) {
	if q.err != nil {
		return nil, q.err
	}
	sqlStr, args := q.buildSelect(1)
	rows, err := q.queryRows(sqlStr, args)
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

// Last retorna a última linha segundo a ordenação atual (invertida).
// Sem OrderBy, usa a chave primária em ordem decrescente.
func (q *Query[T]) Last() (*T, error) {
	rq := *q
	rq.orderBy = reverseOrder(q.orderBy, q.meta)
	return rq.First()
}

// reverseOrder inverte a direção de cada termo do ORDER BY.
func reverseOrder(terms []string, meta *ModelMeta) []string {
	if len(terms) == 0 {
		if meta.PKField != nil {
			return []string{meta.PKField.Column + " DESC"}
		}
		return nil
	}
	out := make([]string, len(terms))
	for i, t := range terms {
		t = strings.TrimSpace(t)
		u := strings.ToUpper(t)
		switch {
		case strings.HasSuffix(u, " DESC"):
			out[i] = strings.TrimSpace(t[:len(t)-5]) + " ASC"
		case strings.HasSuffix(u, " ASC"):
			out[i] = strings.TrimSpace(t[:len(t)-4]) + " DESC"
		default:
			out[i] = t + " DESC"
		}
	}
	return out
}

// Exists reporta se ao menos uma linha corresponde ao filtro atual —
// mais barato que Count() (SELECT 1 ... LIMIT 1).
func (q *Query[T]) Exists() (bool, error) {
	if q.err != nil {
		return false, q.err
	}
	var sb strings.Builder
	sb.WriteString("SELECT 1")
	q.writeFrom(&sb)
	q.writeWhere(&sb)
	sb.WriteString(" LIMIT 1")
	var one int
	err := q.scanRow(rewritePlaceholders(q.driver, sb.String()), q.args, &one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("orm: exists: %w", err)
	}
	return true, nil
}

// Count retorna o número de linhas que correspondem ao filtro atual.
func (q *Query[T]) Count() (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	var sb strings.Builder
	sb.WriteString("SELECT COUNT(*)")
	q.writeFrom(&sb)
	q.writeWhere(&sb)
	var n int64
	if err := q.scanRow(rewritePlaceholders(q.driver, sb.String()), q.args, &n); err != nil {
		return 0, fmt.Errorf("orm: count: %w", err)
	}
	return n, nil
}

// aggregate executa uma função de agregação SQL sobre a coluna.
func (q *Query[T]) aggregate(fn, col string) (float64, error) {
	if q.err != nil {
		return 0, q.err
	}
	if !validIdent(col) {
		return 0, fmt.Errorf("orm: %s: identificador inválido: %q", strings.ToLower(fn), col)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "SELECT %s(%s)", fn, col)
	q.writeFrom(&sb)
	q.writeWhere(&sb)
	var v sql.NullFloat64
	if err := q.scanRow(rewritePlaceholders(q.driver, sb.String()), q.args, &v); err != nil {
		return 0, fmt.Errorf("orm: %s: %w", strings.ToLower(fn), err)
	}
	return v.Float64, nil
}

// Sum/Avg/Min/Max agregam uma coluna numérica respeitando o filtro atual.
// Sem linhas correspondentes (NULL), retornam 0.
func (q *Query[T]) Sum(col string) (float64, error) { return q.aggregate("SUM", col) }
func (q *Query[T]) Avg(col string) (float64, error) { return q.aggregate("AVG", col) }
func (q *Query[T]) Min(col string) (float64, error) { return q.aggregate("MIN", col) }
func (q *Query[T]) Max(col string) (float64, error) { return q.aggregate("MAX", col) }

// Update atualiza as colunas de values para as linhas que correspondem ao WHERE.
// Exige ao menos uma cláusula WHERE para evitar atualizações acidentais globais.
// Campos com tag autonow ausentes de values recebem CURRENT_TIMESTAMP.
func (q *Query[T]) Update(values map[string]any) error {
	if q.err != nil {
		return q.err
	}
	if len(q.where) == 0 {
		return fmt.Errorf("orm: update sem WHERE não é permitido")
	}
	if len(q.joins) > 0 {
		return fmt.Errorf("orm: update com JOIN não é suportado — filtre por subquery no Where")
	}

	colMeta := q.meta.ColToField
	for col := range values {
		if _, ok := colMeta[col]; !ok {
			return fmt.Errorf("orm: update: coluna desconhecida: %q", col)
		}
	}

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	setClauses := make([]string, 0, len(keys)+1)
	args := make([]any, 0, len(keys)+len(q.args))
	for _, col := range keys {
		setClauses = append(setClauses, col+" = ?")
		val := values[col]
		// Fail-closed: erro no hash/encrypt aborta o UPDATE — jamais
		// gravar plaintext numa coluna marcada como sensível.
		if f, ok := colMeta[col]; ok {
			if f.IsHash {
				if s, ok2 := val.(string); ok2 && !strings.HasPrefix(s, "$argon2id$") {
					hashed, err := crypton.HashPassword(s)
					if err != nil {
						return fmt.Errorf("orm: update: hash campo %s: %w", col, err)
					}
					val = hashed
				}
			} else if f.IsEncrypt {
				if s, ok2 := val.(string); ok2 {
					enc, err := crypton.Encrypt(s)
					if err != nil {
						return fmt.Errorf("orm: update: encrypt campo %s: %w", col, err)
					}
					val = enc
				}
			}
		}
		args = append(args, val)
	}

	// autonow: campos como updated_at são atualizados automaticamente
	// quando não vierem explícitos em values.
	for _, f := range q.meta.Fields {
		if f.IsAutoNow {
			if _, present := values[f.Column]; !present {
				setClauses = append(setClauses, f.Column+" = CURRENT_TIMESTAMP")
			}
		}
	}

	args = append(args, q.args...)

	var sb strings.Builder
	sb.WriteString("UPDATE ")
	sb.WriteString(qualifiedTable(q.schema, q.meta.Table))
	sb.WriteString(" SET ")
	sb.WriteString(strings.Join(setClauses, ", "))
	q.writeWhere(&sb)
	if _, err := q.exec.Exec(rewritePlaceholders(q.driver, sb.String()), args...); err != nil {
		return fmt.Errorf("orm: update: %w", err)
	}
	return nil
}

// Delete remove as linhas que correspondem ao WHERE.
// Exige ao menos uma cláusula WHERE para evitar deleções acidentais globais.
func (q *Query[T]) Delete() error {
	if q.err != nil {
		return q.err
	}
	if len(q.where) == 0 {
		return fmt.Errorf("orm: delete sem WHERE não é permitido")
	}
	if len(q.joins) > 0 {
		return fmt.Errorf("orm: delete com JOIN não é suportado — filtre por subquery no Where")
	}
	var sb strings.Builder
	sb.WriteString("DELETE FROM ")
	sb.WriteString(qualifiedTable(q.schema, q.meta.Table))
	q.writeWhere(&sb)
	if _, err := q.exec.Exec(rewritePlaceholders(q.driver, sb.String()), q.args...); err != nil {
		return fmt.Errorf("orm: delete: %w", err)
	}
	return nil
}

// GetOrCreate devolve a primeira linha do filtro atual; se não existir,
// insere defaults e o devolve com created=true.
//
// Como o Where é SQL livre, os campos do filtro NÃO são copiados para
// defaults — preencha defaults com todos os valores do novo registro,
// incluindo os usados no filtro (equivalente ao get_or_create do Django).
func (q *Query[T]) GetOrCreate(defaults *T) (obj *T, created bool, err error) {
	obj, err = q.First()
	if err == nil {
		return obj, false, nil
	}
	if err != sql.ErrNoRows {
		return nil, false, err
	}
	if err := createInto(q.exec, q.driver, q.schema, defaults); err != nil {
		return nil, false, err
	}
	return defaults, true, nil
}

// UpdateOrCreate atualiza as linhas do filtro com values; se nenhuma existir,
// insere defaults (created=true). Equivalente ao update_or_create do Django.
func (q *Query[T]) UpdateOrCreate(values map[string]any, defaults *T) (created bool, err error) {
	exists, err := q.Exists()
	if err != nil {
		return false, err
	}
	if exists {
		return false, q.Update(values)
	}
	return true, createInto(q.exec, q.driver, q.schema, defaults)
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
	if q.err != nil {
		return Page[T]{}, q.err
	}
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
	rows, err := q.queryRows(sqlStr, args)
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

// PaginateNoCount pagina sem executar COUNT(*) — indicado para tabelas
// grandes e feeds infinitos, onde o count exato não importa e custa caro.
// Busca pageSize+1 linhas para inferir HasNext; Total e TotalPages ficam
// em -1 e 0 (desconhecidos). Para paginação com total exato, use Paginate.
func (q *Query[T]) PaginateNoCount(page, pageSize int) (Page[T], error) {
	if q.err != nil {
		return Page[T]{}, q.err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize
	sqlStr, args := q.buildSelectPage(pageSize+1, offset)
	rows, err := q.queryRows(sqlStr, args)
	if err != nil {
		return Page[T]{}, fmt.Errorf("orm: paginate: %w", err)
	}
	defer rows.Close()

	items, err := scanRows[T](rows, q.meta)
	if err != nil {
		return Page[T]{}, err
	}

	hasNext := len(items) > pageSize
	if hasNext {
		items = items[:pageSize]
	}

	return Page[T]{
		Items:      items,
		Total:      -1,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: 0,
		HasNext:    hasNext,
		HasPrev:    page > 1,
	}, nil
}

// ── construção de SQL ─────────────────────────────────────────────────────────

// writeWhere escreve a cláusula WHERE com cada condição parentesizada.
func (q *Query[T]) writeWhere(sb *strings.Builder) {
	if len(q.where) == 0 {
		return
	}
	sb.WriteString(" WHERE ")
	for i, w := range q.where {
		if i > 0 {
			if w.or {
				sb.WriteString(" OR ")
			} else {
				sb.WriteString(" AND ")
			}
		}
		sb.WriteByte('(')
		sb.WriteString(w.cond)
		sb.WriteByte(')')
	}
}

// writeFrom escreve "FROM tabela" seguido das cláusulas de JOIN.
func (q *Query[T]) writeFrom(sb *strings.Builder) {
	sb.WriteString(" FROM ")
	sb.WriteString(qualifiedTable(q.schema, q.meta.Table))
	for _, j := range q.joins {
		sb.WriteByte(' ')
		sb.WriteString(j)
	}
}

func (q *Query[T]) writeSelectHead(sb *strings.Builder) {
	sb.WriteString("SELECT ")
	if q.distinct {
		sb.WriteString("DISTINCT ")
	}
	if len(q.cols) > 0 {
		for i, c := range q.cols {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(c)
		}
	} else if len(q.joins) > 0 {
		// Com JOIN, seleciona só as colunas da tabela base — o scan em T
		// não colide com colunas homônimas das tabelas juntadas.
		sb.WriteString(q.meta.Table)
		sb.WriteString(".*")
	} else {
		sb.WriteByte('*')
	}
	q.writeFrom(sb)
}

func (q *Query[T]) writeOrderBy(sb *strings.Builder) {
	if len(q.orderBy) > 0 {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(strings.Join(q.orderBy, ", "))
	}
}

// buildSelect monta o SQL SELECT respeitando os filtros, ordem e limite.
// O parâmetro forceLimit substitui q.limit quando > 0 (usado por First).
func (q *Query[T]) buildSelect(forceLimit int) (string, []any) {
	var sb strings.Builder
	q.writeSelectHead(&sb)
	q.writeWhere(&sb)
	q.writeOrderBy(&sb)

	// LIMIT/OFFSET como placeholders: o SQL fica idêntico entre páginas,
	// então o cache de prepared statements é reaproveitado de verdade.
	lim := q.limit
	if forceLimit > 0 {
		lim = forceLimit
	}
	args := q.args
	if len(q.rankArgs) > 0 || lim > 0 || q.offset > 0 {
		args = append(make([]any, 0, len(q.args)+len(q.rankArgs)+2), q.args...)
		args = append(args, q.rankArgs...)
	}
	if lim > 0 {
		sb.WriteString(" LIMIT ?")
		args = append(args, lim)
	}
	if q.offset > 0 {
		sb.WriteString(" OFFSET ?")
		args = append(args, q.offset)
	}
	return rewritePlaceholders(q.driver, sb.String()), args
}

// buildSelectPage monta o SELECT com LIMIT/OFFSET fixos, ignorando q.limit e q.offset.
func (q *Query[T]) buildSelectPage(pageSize, offset int) (string, []any) {
	var sb strings.Builder
	q.writeSelectHead(&sb)
	q.writeWhere(&sb)
	q.writeOrderBy(&sb)
	sb.WriteString(" LIMIT ? OFFSET ?")
	args := append(make([]any, 0, len(q.args)+len(q.rankArgs)+2), q.args...)
	args = append(args, q.rankArgs...)
	args = append(args, pageSize, offset)
	return rewritePlaceholders(q.driver, sb.String()), args
}

// compile-time: *database.DB e *database.Tx satisfazem sqlExec.
var (
	_ sqlExec      = (*database.DB)(nil)
	_ sqlExec      = (*database.Tx)(nil)
	_ stmtPreparer = (*database.DB)(nil)
)
