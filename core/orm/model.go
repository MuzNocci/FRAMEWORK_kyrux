package orm

import (
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Field descreve um campo do struct mapeado a uma coluna SQL.
type Field struct {
	Name      string
	Column    string
	IsPK      bool
	IsHash    bool // kyrux:"hash"    — auto-hash Argon2id na escrita
	IsEncrypt bool // kyrux:"encrypt" — auto-cifra AES-256-GCM na escrita, decifra na leitura
	GoIndex   int
	Size      int    // kyrux:"size:N" — usado por migrations
	Default   string // kyrux:"default:value" — valor padrão SQL se campo for vazio
}

// ModelMeta contém os metadados pré-computados de um model.
type ModelMeta struct {
	Table   string
	Fields  []Field
	PKField *Field
}

var metaCache sync.Map // map[reflect.Type]*ModelMeta

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
			case strings.HasPrefix(part, "column:"):
				f.Column = strings.TrimPrefix(part, "column:")
			case strings.HasPrefix(part, "size:"):
				f.Size, _ = strconv.Atoi(strings.TrimPrefix(part, "size:"))
			case strings.HasPrefix(part, "default:"):
				f.Default = strings.TrimPrefix(part, "default:")
			}
		}
		meta.Fields = append(meta.Fields, f)
		if f.IsPK && meta.PKField == nil {
			cp := f
			meta.PKField = &cp
		}
	}
	return meta
}

// toSnake converte CamelCase para snake_case.
func toSnake(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
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
