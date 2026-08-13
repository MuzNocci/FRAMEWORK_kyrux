package admin

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"kyrux/core/database"
	"kyrux/core/orm"
)

// adminRow é a representação genérica (sem o tipo T) de uma linha do model,
// pronta para o template: valores já formatados para exibição.
type adminRow struct {
	PK     string
	Values map[string]string // Column → valor formatado (hash sempre mascarado)
}

// Assinaturas type-erased do CRUD — cada registeredModel guarda uma closure
// fechada sobre seu T (ver registerCRUD). db é resolvido pelo caller
// (handlers.go), nunca pelas closures — mantém o pacote sem acoplamento a
// como a conexão é obtida (Manager, DB direto, etc.).
type (
	listFunc        func(db *database.DB, page, pageSize int, search, sortCol string, sortDesc bool) (rows []adminRow, hasNext bool, err error)
	getFunc         func(db *database.DB, pk string) (adminRow, error)
	createFunc      func(db *database.DB, r *http.Request) error
	updateFunc      func(db *database.DB, pk string, r *http.Request) error
	deleteFunc      func(db *database.DB, pk string) error
	ensureTableFunc func(db *database.DB) error
)

// registerCRUD fecha as operações de CRUD sobre o tipo T e as guarda em rm.
// É o único lugar do pacote que ainda "sabe" o tipo concreto — a partir daqui
// tudo é table-driven via rm.Fields.
func registerCRUD[T any](rm *registeredModel) {
	rm.list = func(db *database.DB, page, pageSize int, search, sortCol string, sortDesc bool) ([]adminRow, bool, error) {
		q := orm.FromDB[T](db)
		if search != "" && len(rm.searchCols) > 0 {
			q = applySearch(q, rm.searchCols, search)
		}
		if sortCol != "" {
			dir := "ASC"
			if sortDesc {
				dir = "DESC"
			}
			q = q.OrderBy(sortCol + " " + dir)
		} else {
			q = q.OrderBy(rm.pkColumn + " DESC")
		}
		// PaginateNoCount (não Paginate): a listagem do admin não precisa do
		// total exato de linhas pra funcionar — evitar o SELECT COUNT(*) tira
		// uma ida ao banco inteira de cada carregamento da página. Medido:
		// ~1.300-2.200 req/s (com COUNT) → ver benchmark após esta mudança.
		p, err := q.PaginateNoCount(page, pageSize)
		if err != nil {
			return nil, false, err
		}
		rows := make([]adminRow, len(p.Items))
		for i := range p.Items {
			rows[i] = rowFromStruct(reflect.ValueOf(&p.Items[i]).Elem(), rm.Fields, rm.pkColumn)
		}
		return rows, p.HasNext, nil
	}

	rm.get = func(db *database.DB, pk string) (adminRow, error) {
		pkArg, err := parsePKArg(pk, rm.pkKind)
		if err != nil {
			return adminRow{}, err
		}
		item, err := orm.FromDB[T](db).Where(rm.pkColumn+" = ?", pkArg).First()
		if err != nil {
			return adminRow{}, err
		}
		// rowFromStructForm (não rowFromStruct): esta linha alimenta o
		// formulário de edição — precisa do formato de INPUT (datetime-local
		// com "T"), não do formato de exibição da listagem.
		return rowFromStructForm(reflect.ValueOf(item).Elem(), rm.Fields, rm.pkColumn), nil
	}

	rm.create = func(db *database.DB, r *http.Request) error {
		var obj T
		v := reflect.ValueOf(&obj).Elem()
		form := r.PostForm
		for _, f := range rm.Fields {
			if f.IsPK || f.IsAutoNow {
				continue
			}
			if f.IsImage {
				path, ok, err := uploadFieldValue(r, rm.App, rm.Table, f)
				if err != nil {
					return err
				}
				if ok {
					v.Field(f.GoIndex).SetString(path)
				}
				continue
			}
			raw := form.Get(f.Column)
			if f.IsHash && raw == "" {
				return fmt.Errorf("campo %q é obrigatório", f.Label)
			}
			if err := setFieldFromString(v.Field(f.GoIndex), raw, f); err != nil {
				return fmt.Errorf("campo %q: %w", f.Label, err)
			}
		}
		return orm.Create(db, &obj)
	}

	rm.update = func(db *database.DB, pk string, r *http.Request) error {
		pkArg, err := parsePKArg(pk, rm.pkKind)
		if err != nil {
			return err
		}
		var scratch T
		v := reflect.ValueOf(&scratch).Elem()
		form := r.PostForm
		values := make(map[string]any, len(rm.Fields))
		for _, f := range rm.Fields {
			if f.IsPK || f.IsAutoNow {
				continue
			}
			if f.IsImage {
				path, ok, err := uploadFieldValue(r, rm.App, rm.Table, f)
				if err != nil {
					return err
				}
				if !ok {
					continue // sem arquivo novo no edit = mantém o caminho atual
				}
				v.Field(f.GoIndex).SetString(path)
				values[f.Column] = path
				continue
			}
			raw := form.Get(f.Column)
			if f.IsHash && raw == "" {
				continue // em branco no edit = mantém a senha/hash atual
			}
			if err := setFieldFromString(v.Field(f.GoIndex), raw, f); err != nil {
				return fmt.Errorf("campo %q: %w", f.Label, err)
			}
			values[f.Column] = v.Field(f.GoIndex).Interface()
		}
		if len(values) == 0 {
			return nil
		}
		return orm.FromDB[T](db).Where(rm.pkColumn+" = ?", pkArg).Update(values)
	}

	rm.delete = func(db *database.DB, pk string) error {
		pkArg, err := parsePKArg(pk, rm.pkKind)
		if err != nil {
			return err
		}
		return orm.FromDB[T](db).Where(rm.pkColumn+" = ?", pkArg).Delete()
	}

	rm.ensureTable = func(db *database.DB) error {
		return orm.EnsureSQLiteTable[T](db)
	}
}

// applySearch adiciona um grupo OR (busca textual) sobre as colunas
// pesquisáveis, como uma única condição AND-composable.
func applySearch[T any](q *orm.Query[T], cols []string, search string) *orm.Query[T] {
	conds := make([]string, len(cols))
	args := make([]any, len(cols))
	like := "%" + search + "%"
	for i, c := range cols {
		conds[i] = c + " LIKE ?"
		args[i] = like
	}
	return q.Where("("+strings.Join(conds, " OR ")+")", args...)
}

// rowFromStruct converte um struct (linha do model) num adminRow genérico.
// Campos hash NUNCA recebem o valor real — nem mascarado a partir dele.
func rowFromStruct(v reflect.Value, fields []adminField, pkColumn string) adminRow {
	row := adminRow{Values: make(map[string]string, len(fields))}
	for _, f := range fields {
		fv := v.Field(f.GoIndex)
		val := formatDisplayValue(fv, f)
		row.Values[f.Column] = val
		if f.Column == pkColumn {
			row.PK = val
		}
	}
	return row
}

// rowFromStructForm é a variante de rowFromStruct usada para popular o
// formulário de edição — os valores usam o formato aceito pelos inputs HTML
// (ex: datetime-local com "T"), diferente do formato de exibição da listagem.
func rowFromStructForm(v reflect.Value, fields []adminField, pkColumn string) adminRow {
	row := adminRow{Values: make(map[string]string, len(fields))}
	for _, f := range fields {
		fv := v.Field(f.GoIndex)
		var val string
		if !f.IsHash {
			val = formatInputValue(fv, f)
		}
		row.Values[f.Column] = val
		if f.Column == pkColumn {
			row.PK = formatDisplayValue(fv, f)
		}
	}
	return row
}

// formatDisplayValue formata um valor para exibição (lista e leitura no form).
func formatDisplayValue(fv reflect.Value, f adminField) string {
	if f.IsHash {
		return "" // nunca exibido — nem mascarado a partir do valor real
	}
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			return ""
		}
		fv = fv.Elem()
	}
	switch fv.Kind() {
	case reflect.String:
		return fv.String()
	case reflect.Bool:
		return strconv.FormatBool(fv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(fv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(fv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(fv.Float(), 'f', -1, 64)
	case reflect.Struct:
		if fv.Type() == timeType {
			t := fv.Interface().(time.Time)
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02 15:04")
		}
	}
	return fmt.Sprintf("%v", fv.Interface())
}

// formatInputValue formata um valor para o atributo value="" de um input —
// como formatDisplayValue, exceto datetime (formato datetime-local) e bool
// (tratado à parte via Checked, não value).
func formatInputValue(fv reflect.Value, f adminField) string {
	if f.IsHash {
		return "" // nunca preenche o input de senha com o hash real
	}
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			return ""
		}
		fv = fv.Elem()
	}
	if fv.Kind() == reflect.Struct && fv.Type() == timeType {
		t := fv.Interface().(time.Time)
		if t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02T15:04")
	}
	return formatDisplayValue(fv, adminField{})
}

// setFieldFromString converte o valor de um campo de formulário (string) e o
// grava em fv, respeitando ponteiro (Optional) e o tipo Go de destino.
func setFieldFromString(fv reflect.Value, raw string, f adminField) error {
	if fv.Kind() == reflect.Ptr {
		if raw == "" {
			fv.Set(reflect.Zero(fv.Type()))
			return nil
		}
		elem := reflect.New(fv.Type().Elem())
		if err := setScalar(elem.Elem(), raw, f.Widget); err != nil {
			return err
		}
		fv.Set(elem)
		return nil
	}
	return setScalar(fv, raw, f.Widget)
}

func setScalar(fv reflect.Value, raw string, widget string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		fv.SetBool(raw == "on" || raw == "true" || raw == "1")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if raw == "" {
			fv.SetInt(0)
			return nil
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("número inválido")
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if raw == "" {
			fv.SetUint(0)
			return nil
		}
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("número inválido")
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		if raw == "" {
			fv.SetFloat(0)
			return nil
		}
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return fmt.Errorf("número inválido")
		}
		fv.SetFloat(n)
	case reflect.Struct:
		if fv.Type() == timeType {
			if raw == "" {
				fv.Set(reflect.ValueOf(time.Time{}))
				return nil
			}
			t, err := time.Parse("2006-01-02T15:04", raw)
			if err != nil {
				return fmt.Errorf("data/hora inválida")
			}
			fv.Set(reflect.ValueOf(t))
			return nil
		}
		return fmt.Errorf("tipo não suportado pelo admin")
	default:
		return fmt.Errorf("tipo não suportado pelo admin")
	}
	return nil
}

// parsePKArg converte o PK vindo da URL (sempre string) para o tipo Go real
// da coluna — necessário porque comparar uma coluna bigint com um argumento
// string falha em bancos como PostgreSQL (sem cast implícito).
func parsePKArg(pk string, kind reflect.Kind) (any, error) {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(pk, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("admin: PK inválida: %q", pk)
		}
		return n, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(pk, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("admin: PK inválida: %q", pk)
		}
		return n, nil
	case reflect.String:
		return pk, nil
	default:
		return nil, fmt.Errorf("admin: tipo de PK não suportado: %s", kind)
	}
}
