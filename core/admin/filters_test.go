package admin

import (
	"reflect"
	"testing"
	"time"

	"kyrux/core/database"
	"kyrux/core/orm"

	_ "modernc.org/sqlite"
)

// ── FilterFields / resolveFilterFields ──────────────────────────────────────

type filterTestModel struct {
	ID           int64 `kyrux:"pk"`
	Ativo        bool
	Nome         string `kyrux:"size:100"`
	Preco        float64
	FornecedorID int64 `kyrux:"column:fornecedor_id,fk:fornecedores"`
}

func TestFilterFieldsAceitaWidgetsSuportados(t *testing.T) {
	resetRegistry()
	Register[filterTestModel]("filter-test", "Filter Test",
		FilterFields("Ativo", "Preco", "FornecedorID"))
	rm, _ := modelBySlug("filter-test")
	if len(rm.filterFields) != 3 {
		t.Fatalf("esperava 3 campos filtráveis, recebeu %d: %+v", len(rm.filterFields), rm.filterFields)
	}
}

func TestFilterFieldsRejeitaCampoInexistente(t *testing.T) {
	resetRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para campo inexistente em FilterFields")
		}
	}()
	Register[filterTestModel]("filter-test", "Filter Test", FilterFields("NaoExiste"))
}

func TestFilterFieldsRejeitaWidgetNaoFiltravel(t *testing.T) {
	resetRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para campo text (Nome) em FilterFields")
		}
	}()
	Register[filterTestModel]("filter-test", "Filter Test", FilterFields("Nome"))
}

func TestFilterFieldsVazioNaoGeraFiltros(t *testing.T) {
	resetRegistry()
	Register[filterTestModel]("filter-test", "Filter Test")
	rm, _ := modelBySlug("filter-test")
	if len(rm.filterFields) != 0 {
		t.Errorf("sem FilterFields, esperava zero filtros, recebeu %d", len(rm.filterFields))
	}
}

// ── convertFilterScalar ──────────────────────────────────────────────────

func TestConvertFilterScalar(t *testing.T) {
	if v, err := convertFilterScalar("true", reflect.Bool); err != nil || v != true {
		t.Errorf("bool true: %v, %v", v, err)
	}
	if v, err := convertFilterScalar("false", reflect.Bool); err != nil || v != false {
		t.Errorf("bool false: %v, %v", v, err)
	}
	if v, err := convertFilterScalar("42", reflect.Int64); err != nil || v != int64(42) {
		t.Errorf("int64: %v, %v", v, err)
	}
	if v, err := convertFilterScalar("7", reflect.Uint32); err != nil || v != uint64(7) {
		t.Errorf("uint32: %v, %v", v, err)
	}
	if v, err := convertFilterScalar("3.5", reflect.Float64); err != nil || v != 3.5 {
		t.Errorf("float64: %v, %v", v, err)
	}
	if _, err := convertFilterScalar("abc", reflect.Int64); err == nil {
		t.Error("esperava erro para int inválido")
	}
	if _, err := convertFilterScalar("abc", reflect.Float64); err == nil {
		t.Error("esperava erro para float inválido")
	}
	if v, err := convertFilterScalar("qualquer", reflect.String); err != nil || v != "qualquer" {
		t.Errorf("string (fallback): %v, %v", v, err)
	}
}

// ── filterConds ──────────────────────────────────────────────────────────

func TestFilterCondsExatoEFaixa(t *testing.T) {
	fields := []adminField{
		{Column: "ativo", Widget: "checkbox", Kind: reflect.Bool},
		{Column: "preco", Widget: "number-float", Kind: reflect.Float64},
	}
	views := []filterView{
		{Column: "ativo", Value: "true"},
		{Column: "preco", IsRange: true, MinValue: "10", MaxValue: "100"},
	}
	conds := filterConds(fields, views)
	if len(conds) != 3 {
		t.Fatalf("esperava 3 condições, recebeu %d: %+v", len(conds), conds)
	}
	byKey := make(map[string]filterCond, len(conds))
	for _, c := range conds {
		byKey[c.Col+c.Op] = c
	}
	if c, ok := byKey["ativo="]; !ok || c.Val != true {
		t.Errorf("condição de igualdade incorreta: %+v", c)
	}
	if c, ok := byKey["preco>="]; !ok || c.Val != 10.0 {
		t.Errorf("condição >= incorreta: %+v", c)
	}
	if c, ok := byKey["preco<="]; !ok || c.Val != 100.0 {
		t.Errorf("condição <= incorreta: %+v", c)
	}
}

func TestFilterCondsIgnoraValorInvalidoSilenciosamente(t *testing.T) {
	fields := []adminField{{Column: "preco", Widget: "number-float", Kind: reflect.Float64}}
	views := []filterView{{Column: "preco", IsRange: true, MinValue: "nao-numero"}}
	conds := filterConds(fields, views)
	if len(conds) != 0 {
		t.Errorf("valor inválido deveria ser ignorado silenciosamente, recebeu %+v", conds)
	}
}

func TestFilterCondsDataUsaLimiteSuperiorExclusivo(t *testing.T) {
	fields := []adminField{{Column: "criado_em", Widget: "datetime"}}
	views := []filterView{{Column: "criado_em", IsRange: true, IsDate: true, MaxValue: "2026-01-15"}}
	conds := filterConds(fields, views)
	if len(conds) != 1 || conds[0].Op != "<" {
		t.Fatalf("esperava uma condição '<' (limite exclusivo), recebeu %+v", conds)
	}
	tv, ok := conds[0].Val.(time.Time)
	if !ok {
		t.Fatalf("valor deveria ser time.Time, recebeu %T", conds[0].Val)
	}
	if tv.Year() != 2026 || tv.Month() != 1 || tv.Day() != 16 {
		t.Errorf("limite superior deveria ser o dia seguinte (2026-01-16), recebeu %v", tv)
	}
}

func TestFilterCondsDataMinInclusivo(t *testing.T) {
	fields := []adminField{{Column: "criado_em", Widget: "datetime"}}
	views := []filterView{{Column: "criado_em", IsRange: true, IsDate: true, MinValue: "2026-01-15"}}
	conds := filterConds(fields, views)
	if len(conds) != 1 || conds[0].Op != ">=" {
		t.Fatalf("esperava uma condição '>=', recebeu %+v", conds)
	}
}

// ── filterURLParams ──────────────────────────────────────────────────────

func TestFilterURLParams(t *testing.T) {
	views := []filterView{
		{Column: "ativo", Value: "true"},
		{Column: "preco", IsRange: true, MinValue: "10", MaxValue: ""},
		{Column: "nada", Value: ""},
	}
	params := filterURLParams(views)
	if params["f_ativo"] != "true" {
		t.Errorf("f_ativo ausente ou incorreto: %v", params)
	}
	if params["f_preco_min"] != "10" {
		t.Errorf("f_preco_min ausente ou incorreto: %v", params)
	}
	if _, ok := params["f_preco_max"]; ok {
		t.Errorf("f_preco_max vazio não deveria aparecer: %v", params)
	}
	if _, ok := params["f_nada"]; ok {
		t.Errorf("filtro sem valor não deveria aparecer: %v", params)
	}
}

func TestFilterURLParamsVazioSemFiltros(t *testing.T) {
	if params := filterURLParams(nil); params != nil {
		t.Errorf("sem filtros, esperava nil, recebeu %v", params)
	}
}

// ── integração: rm.list aplicando filterConds via SQLite real ──────────────

type listFilterModel struct {
	ID    int64  `kyrux:"pk"`
	Nome  string `kyrux:"size:100"`
	Ativo bool
	Preco float64
}

func TestListAplicaFiltros(t *testing.T) {
	resetRegistry()
	Register[listFilterModel]("list-filter-test", "List Filter Test",
		FilterFields("Ativo", "Preco"))
	rm, _ := modelBySlug("list-filter-test")

	db, err := database.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	defer db.Close()
	if err := EnsureAllTables(db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}

	orm.Create(db, &listFilterModel{Nome: "A", Ativo: true, Preco: 10})
	orm.Create(db, &listFilterModel{Nome: "B", Ativo: false, Preco: 200})
	orm.Create(db, &listFilterModel{Nome: "C", Ativo: true, Preco: 300})

	rows, _, err := rm.list(db, 1, 20, "", "", false, []filterCond{{Col: "ativo", Op: "=", Val: true}})
	if err != nil {
		t.Fatalf("list com filtro bool: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("esperava 2 linhas (ativo=true), recebeu %d", len(rows))
	}

	rows, _, err = rm.list(db, 1, 20, "", "", false, []filterCond{{Col: "preco", Op: ">=", Val: 100.0}})
	if err != nil {
		t.Fatalf("list com filtro de faixa: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("esperava 2 linhas (preco >= 100), recebeu %d", len(rows))
	}

	rows, _, err = rm.list(db, 1, 20,
		"", "", false,
		[]filterCond{{Col: "ativo", Op: "=", Val: true}, {Col: "preco", Op: "<=", Val: 100.0}})
	if err != nil {
		t.Fatalf("list com filtros combinados: %v", err)
	}
	if len(rows) != 1 || rows[0].Values["nome"] != "A" {
		t.Errorf("esperava só 'A' (ativo=true AND preco<=100), recebeu %+v", rows)
	}
}
