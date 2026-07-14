package admin

import (
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestParsePKArg(t *testing.T) {
	if v, err := parsePKArg("42", reflect.Int64); err != nil || v != int64(42) {
		t.Errorf("int64: got %v, %v", v, err)
	}
	if v, err := parsePKArg("7", reflect.Uint32); err != nil || v != uint64(7) {
		t.Errorf("uint32: got %v, %v", v, err)
	}
	if v, err := parsePKArg("abc", reflect.String); err != nil || v != "abc" {
		t.Errorf("string: got %v, %v", v, err)
	}
	if _, err := parsePKArg("nao-numero", reflect.Int64); err == nil {
		t.Error("esperava erro para PK int inválida")
	}
	if _, err := parsePKArg("1", reflect.Float64); err == nil {
		t.Error("esperava erro para kind não suportado")
	}
}

func TestSetFieldFromStringEscalares(t *testing.T) {
	type s struct {
		Str   string
		Bool  bool
		Int   int64
		Uint  uint32
		Float float64
		Time  time.Time
		Opt   *string
	}
	var obj s
	v := reflect.ValueOf(&obj).Elem()

	if err := setFieldFromString(v.FieldByName("Str"), "olá", adminField{Widget: "text"}); err != nil || obj.Str != "olá" {
		t.Errorf("string: %v, %q", err, obj.Str)
	}
	if err := setFieldFromString(v.FieldByName("Bool"), "on", adminField{Widget: "checkbox"}); err != nil || !obj.Bool {
		t.Errorf("bool: %v, %v", err, obj.Bool)
	}
	if err := setFieldFromString(v.FieldByName("Int"), "-5", adminField{Widget: "number"}); err != nil || obj.Int != -5 {
		t.Errorf("int: %v, %v", err, obj.Int)
	}
	if err := setFieldFromString(v.FieldByName("Uint"), "9", adminField{Widget: "number"}); err != nil || obj.Uint != 9 {
		t.Errorf("uint: %v, %v", err, obj.Uint)
	}
	if err := setFieldFromString(v.FieldByName("Float"), "3.5", adminField{Widget: "number-float"}); err != nil || obj.Float != 3.5 {
		t.Errorf("float: %v, %v", err, obj.Float)
	}
	if err := setFieldFromString(v.FieldByName("Time"), "2026-01-02T15:04", adminField{Widget: "datetime"}); err != nil {
		t.Errorf("time: %v", err)
	}
	if obj.Time.Year() != 2026 || obj.Time.Month() != 1 || obj.Time.Day() != 2 {
		t.Errorf("time parseado incorretamente: %v", obj.Time)
	}

	// Ponteiro: em branco vira nil; valor não-vazio aloca.
	if err := setFieldFromString(v.FieldByName("Opt"), "", adminField{Widget: "text"}); err != nil || obj.Opt != nil {
		t.Errorf("ponteiro vazio deveria ser nil: %v, %v", err, obj.Opt)
	}
	if err := setFieldFromString(v.FieldByName("Opt"), "x", adminField{Widget: "text"}); err != nil || obj.Opt == nil || *obj.Opt != "x" {
		t.Errorf("ponteiro preenchido incorreto: %v, %v", err, obj.Opt)
	}
}

func TestSetFieldFromStringNumeroInvalido(t *testing.T) {
	type s struct{ N int64 }
	var obj s
	v := reflect.ValueOf(&obj).Elem()
	if err := setFieldFromString(v.FieldByName("N"), "abc", adminField{}); err == nil {
		t.Error("esperava erro para número inválido")
	}
}

func TestFormatDisplayValueMascaraHash(t *testing.T) {
	type s struct{ Senha string }
	obj := s{Senha: "$argon2id$v=19$...hash-real..."}
	v := reflect.ValueOf(obj)
	got := formatDisplayValue(v.FieldByName("Senha"), adminField{IsHash: true})
	if got != "" {
		t.Errorf("hash deveria ser sempre vazio na exibição, recebeu %q", got)
	}
}

func TestFormatInputValueMascaraHash(t *testing.T) {
	type s struct{ Senha string }
	obj := s{Senha: "$argon2id$v=19$...hash-real..."}
	v := reflect.ValueOf(obj)
	got := formatInputValue(v.FieldByName("Senha"), adminField{IsHash: true})
	if got != "" {
		t.Errorf("hash nunca deve preencher o input, recebeu %q", got)
	}
}

func TestFormatInputValueDatetimeLocal(t *testing.T) {
	type s struct{ T time.Time }
	obj := s{T: time.Date(2026, 3, 5, 14, 30, 0, 0, time.UTC)}
	v := reflect.ValueOf(obj)
	got := formatInputValue(v.FieldByName("T"), adminField{})
	want := "2026-03-05T14:30"
	if got != want {
		t.Errorf("formatInputValue datetime: got %q, want %q", got, want)
	}
}

func TestFormatDisplayValueDatetimeEspaco(t *testing.T) {
	type s struct{ T time.Time }
	obj := s{T: time.Date(2026, 3, 5, 14, 30, 0, 0, time.UTC)}
	v := reflect.ValueOf(obj)
	got := formatDisplayValue(v.FieldByName("T"), adminField{})
	want := "2026-03-05 14:30"
	if got != want {
		t.Errorf("formatDisplayValue datetime: got %q, want %q", got, want)
	}
}

func TestFormatDisplayValuePonteiroNil(t *testing.T) {
	type s struct{ P *string }
	var obj s
	v := reflect.ValueOf(obj)
	if got := formatDisplayValue(v.FieldByName("P"), adminField{}); got != "" {
		t.Errorf("ponteiro nil deveria formatar como vazio, recebeu %q", got)
	}
}

// TestBuildURLOmiteVazios garante que parâmetros vazios não aparecem na URL
// (evita "?q=&sort=&dir=" poluindo os links de paginação/ordenação).
func TestBuildURLOmiteVazios(t *testing.T) {
	got := buildURL("/admin/produtos/", map[string]string{"q": "", "sort": "nome", "dir": ""})
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("sort") != "nome" {
		t.Errorf("sort ausente: %q", got)
	}
	if q.Has("q") || q.Has("dir") {
		t.Errorf("parâmetros vazios não deveriam aparecer: %q", got)
	}
}

func TestBuildURLSemParametrosNaoAdicionaInterrogacao(t *testing.T) {
	got := buildURL("/admin/produtos/", map[string]string{"q": "", "dir": ""})
	if got != "/admin/produtos/" {
		t.Errorf("sem params, URL não deveria ter '?': %q", got)
	}
}

func TestNormalizeBasePath(t *testing.T) {
	cases := map[string]string{
		"":          "/admin/",
		"admin":     "/admin/",
		"/admin":    "/admin/",
		"admin/":    "/admin/",
		"/painel/":  "/painel/",
		"/custom":   "/custom/",
	}
	for in, want := range cases {
		if got := normalizeBasePath(in); got != want {
			t.Errorf("normalizeBasePath(%q) = %q, want %q", in, got, want)
		}
	}
}
