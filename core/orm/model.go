package orm

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Field descreve um campo do struct mapeado a uma coluna SQL.
type Field struct {
	Name         string
	Column       string
	IsPK         bool
	IsHash       bool // kyrux:"hash"    — auto-hash Argon2id na escrita
	IsEncrypt    bool // kyrux:"encrypt" — auto-cifra AES-256-GCM na escrita, decifra na leitura
	IsAutoNow    bool // kyrux:"autonow"     — CURRENT_TIMESTAMP automático em todo Update (ex: updated_at)
	IsAutoNowAdd bool // kyrux:"autonow_add" — CURRENT_TIMESTAMP só no INSERT, nunca tocado depois (ex: created_at)
	IsNanoID     bool // kyrux:"nanoid"  — string aleatória única gerada no INSERT quando o campo está vazio; tamanho vem de size:N (padrão 21)
	Unique       bool // kyrux:"unique"  — usado por migrations e pelo DDL automático do fallback SQLite
	FTS          bool // kyrux:"fts"     — busca full-text nativa (Query.Search); índice gerado por migrations
	IsImage      bool // kyrux:"image"   — admin exibe input de upload; arquivo vai para medias/<app>/<tabela>/
	GoIndex      int
	Size         int    // kyrux:"size:N" — usado por migrations
	Default      string // kyrux:"default:value" — valor padrão SQL se campo for vazio
	FK           string // kyrux:"fk:tabela" — REFERENCES tabela(id) na migration
	// FKLabel: coluna (ou colunas, separadas por "+") usada como rótulo no
	// <select> do admin — padrão: mostra o id. Cada parte é "coluna" (do
	// próprio model referenciado) ou "tabela.coluna" (segue o FK do model
	// referenciado até essa tabela — precisa haver um campo com
	// kyrux:"fk:tabela" nele). Ex: kyrux:"fklabel:clientes.nome+titulo"
	// no FK pra "projetos" mostra "<nome do cliente> — <título>".
	FKLabel string
}

// ModelMeta contém os metadados pré-computados de um model.
type ModelMeta struct {
	Table      string
	Fields     []Field
	PKField    *Field
	ColToField map[string]Field // índice coluna→Field pré-computado; evita rebuild por query
}

var metaCache sync.Map // map[reflect.Type]*ModelMeta

// MetaOf retorna os metadados computados (cacheados) do model T — nomes de
// coluna, tags kyrux, chave primária. Usado por ferramentas que precisam
// introspectar models em runtime (ex: o admin).
func MetaOf[T any]() *ModelMeta { return metaOf[T]() }

// metaOf retorna o ModelMeta cacheado para o tipo T.
func metaOf[T any]() *ModelMeta {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		panic("orm: T deve ser um struct concreto")
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return cachedMeta(t)
}

// cachedMeta constrói e cacheia o ModelMeta para um reflect.Type.
// Usado tanto por metaOf (generics) quanto por Create (any).
func cachedMeta(t reflect.Type) *ModelMeta {
	if v, ok := metaCache.Load(t); ok {
		return v.(*ModelMeta)
	}
	meta := buildMeta(t)
	metaCache.Store(t, meta)
	return meta
}

func buildMeta(t reflect.Type) *ModelMeta {
	meta := &ModelMeta{Table: pluralSnake(t.Name())}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		f := Field{
			Name:    sf.Name,
			Column:  toSnake(sf.Name),
			GoIndex: i,
		}
		for _, part := range strings.Split(sf.Tag.Get("kyrux"), ",") {
			part = strings.TrimSpace(part)
			switch {
			case part == "pk":
				f.IsPK = true
			case part == "hash":
				f.IsHash = true
			case part == "encrypt":
				f.IsEncrypt = true
			case part == "autonow":
				f.IsAutoNow = true
			case part == "autonow_add":
				f.IsAutoNowAdd = true
			case part == "nanoid":
				f.IsNanoID = true
			case part == "unique":
				f.Unique = true
			case part == "fts":
				f.FTS = true
			case part == "image":
				f.IsImage = true
			case strings.HasPrefix(part, "column:"):
				f.Column = strings.TrimPrefix(part, "column:")
			case strings.HasPrefix(part, "size:"):
				f.Size, _ = strconv.Atoi(strings.TrimPrefix(part, "size:"))
			case strings.HasPrefix(part, "default:"):
				f.Default = strings.TrimPrefix(part, "default:")
			case strings.HasPrefix(part, "fk:"):
				f.FK = strings.TrimPrefix(part, "fk:")
			case strings.HasPrefix(part, "fklabel:"):
				f.FKLabel = strings.TrimPrefix(part, "fklabel:")
			}
		}
		// autonow/autonow_add implicam preenchimento no INSERT quando o
		// campo está zerado — a diferença entre os dois é só no Update
		// (ver Query.Update em query.go): autonow_add nunca é tocado de
		// novo depois do INSERT.
		if (f.IsAutoNow || f.IsAutoNowAdd) && f.Default == "" {
			f.Default = "CURRENT_TIMESTAMP"
		}

		// Metadata das tags kyrux entra direto na construção de SQL
		// estrutural (SELECT/INSERT/UPDATE, DDL do autoddl/migrations) sem
		// passar por placeholder — valida aqui, uma vez por tipo, em vez de
		// confiar que nenhuma tag futura virá de fonte não confiável.
		if !validIdent(f.Column) {
			panic(fmt.Sprintf("orm: model %s: coluna inválida no campo %s (tag kyrux %q ou nome do campo): %q", t.Name(), f.Name, sf.Tag.Get("kyrux"), f.Column))
		}
		if f.FK != "" && !validIdent(f.FK) {
			panic(fmt.Sprintf("orm: model %s: kyrux:\"fk:...\" inválido no campo %s: %q", t.Name(), f.Name, f.FK))
		}
		for _, part := range strings.Split(f.FKLabel, "+") {
			if part != "" && !validIdent(part) {
				panic(fmt.Sprintf("orm: model %s: kyrux:\"fklabel:...\" inválido no campo %s: %q", t.Name(), f.Name, f.FKLabel))
			}
		}
		if f.Default != "" && !validDefault(f.Default) {
			panic(fmt.Sprintf("orm: model %s: kyrux:\"default:...\" inválido no campo %s: %q", t.Name(), f.Name, f.Default))
		}

		meta.Fields = append(meta.Fields, f)
		if f.IsPK && meta.PKField == nil {
			cp := f
			meta.PKField = &cp
		}
	}
	meta.ColToField = make(map[string]Field, len(meta.Fields))
	for _, f := range meta.Fields {
		meta.ColToField[f.Column] = f
	}
	return meta
}

// toSnake converte CamelCase para snake_case com tratamento de acrônimos:
// ID → id, UserID → user_id, HTTPServer → http_server.
func toSnake(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 4)
	rs := []rune(s)
	for i, r := range rs {
		if unicode.IsUpper(r) {
			// Underscore antes de maiúscula que inicia palavra nova:
			// após minúscula, ou maiúscula seguida de minúscula (fim de acrônimo).
			if i > 0 && (!unicode.IsUpper(rs[i-1]) || (i+1 < len(rs) && unicode.IsLower(rs[i+1]))) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// pluralSnake converte um nome de tipo em nome de tabela snake_case plural.
//
//	User        → users
//	Category    → categories
//	Address     → addresses
//	UserProfile → user_profiles
func pluralSnake(typeName string) string {
	s := toSnake(typeName)
	switch {
	case strings.HasSuffix(s, "s") ||
		strings.HasSuffix(s, "x") ||
		strings.HasSuffix(s, "z") ||
		strings.HasSuffix(s, "sh") ||
		strings.HasSuffix(s, "ch"):
		return s + "es"
	case strings.HasSuffix(s, "y") && len(s) > 1 && !isVowel(rune(s[len(s)-2])):
		return s[:len(s)-1] + "ies"
	default:
		return s + "s"
	}
}

func isVowel(r rune) bool {
	return strings.ContainsRune("aeiou", r)
}
