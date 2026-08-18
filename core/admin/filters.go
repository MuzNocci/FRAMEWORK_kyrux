package admin

import (
	"fmt"
	"log"
	"reflect"
	"strconv"
	"time"

	"kyrux/core/database"
	"kyrux/core/router"
)

// filterableWidgets são os widgets com um tipo de filtro coerente — os
// demais (text, textarea, password, file) não entram em FilterFields.
var filterableWidgets = map[string]bool{
	"checkbox":     true, // bool: filtro exato (Todos/Sim/Não)
	"select":       true, // FK: filtro exato pelas opções do <select>
	"number":       true, // filtro por faixa (mín/máx)
	"number-float": true,
	"datetime":     true, // filtro por faixa de datas
}

// resolveFilterFields converte os nomes de campo Go de FilterFields em
// adminFields completos, na ordem informada — panic em nome desconhecido ou
// widget sem filtro coerente (mesmo espírito de resolveColumns).
func resolveFilterFields(label string, fields []adminField, names []string) []adminField {
	if len(names) == 0 {
		return nil
	}
	byName := make(map[string]adminField, len(fields))
	for _, f := range fields {
		byName[f.GoName] = f
	}
	out := make([]adminField, 0, len(names))
	for _, n := range names {
		f, ok := byName[n]
		if !ok {
			panic(fmt.Sprintf("admin: model %q: campo %q não existe no model", label, n))
		}
		if !filterableWidgets[f.Widget] {
			panic(fmt.Sprintf("admin: model %q: campo %q (widget %q) não pode ser usado em FilterFields — suportado: bool, FK (select), number, number-float, datetime", label, n, f.Widget))
		}
		out = append(out, f)
	}
	return out
}

// filterCond é uma condição de filtro já resolvida (coluna + operador +
// valor no tipo Go real da coluna) — aplicada no WHERE por rm.list.
type filterCond struct {
	Col string
	Op  string // "=" | ">=" | "<=" | "<"
	Val any
}

// filterView é o view-model de um filtro para o template da listagem —
// carrega tanto a UI (Options, rótulos) quanto os valores atuais lidos da
// query string, servindo de origem única tanto para filterConds (o WHERE)
// quanto para filterURLParams (preservar os filtros em links de
// ordenação/paginação), evitando reler a query string em cada lugar.
type filterView struct {
	Column   string
	Label    string
	IsRange  bool // number/number-float/datetime: Min/Max em vez de Value/Options
	IsDate   bool // dentro de IsRange: <input type="date"> em vez de type="number"
	Options  []fkOption
	Value    string
	MinValue string
	MaxValue string
}

// buildFilterViews lê os parâmetros f_<coluna>[_min/_max] da query string
// atual e monta o view-model de cada filtro configurado — carrega as opções
// de campos FK (mesmo fetchFKOptions do formulário) e faz uma leitura da
// query string por request, não por chamador.
func buildFilterViews(ctx *router.Context, db *database.DB, filterFields []adminField) []filterView {
	if len(filterFields) == 0 {
		return nil
	}
	views := make([]filterView, 0, len(filterFields))
	for _, f := range filterFields {
		switch f.Widget {
		case "checkbox":
			views = append(views, filterView{
				Column: f.Column,
				Label:  f.Label,
				Value:  ctx.Query("f_" + f.Column),
				Options: []fkOption{
					{Value: "true", Label: "Sim"},
					{Value: "false", Label: "Não"},
				},
			})
		case "select":
			opts, err := fetchFKOptions(db, f.FKTable, f.FKLabel)
			if err != nil {
				log.Printf("admin: %v\n", err)
			}
			views = append(views, filterView{
				Column:  f.Column,
				Label:   f.Label,
				Value:   ctx.Query("f_" + f.Column),
				Options: opts,
			})
		case "number", "number-float":
			views = append(views, filterView{
				Column:   f.Column,
				Label:    f.Label,
				IsRange:  true,
				MinValue: ctx.Query("f_" + f.Column + "_min"),
				MaxValue: ctx.Query("f_" + f.Column + "_max"),
			})
		case "datetime":
			views = append(views, filterView{
				Column:   f.Column,
				Label:    f.Label,
				IsRange:  true,
				IsDate:   true,
				MinValue: ctx.Query("f_" + f.Column + "_min"),
				MaxValue: ctx.Query("f_" + f.Column + "_max"),
			})
		}
	}
	return views
}

// filterConds converte os valores brutos de filters (já lidos da URL por
// buildFilterViews) nas condições tipadas do WHERE. Um valor que não
// converte pro tipo da coluna (ex: URL adulterada manualmente) é
// silenciosamente ignorado — mesmo tratamento que sort=coluna_falsa já
// recebe: nunca um erro 500 por causa de um parâmetro de filtro inválido.
func filterConds(filterFields []adminField, filters []filterView) []filterCond {
	if len(filters) == 0 {
		return nil
	}
	byCol := make(map[string]adminField, len(filterFields))
	for _, f := range filterFields {
		byCol[f.Column] = f
	}
	var conds []filterCond
	for _, fv := range filters {
		f := byCol[fv.Column]
		if fv.IsDate {
			if fv.MinValue != "" {
				if t, err := time.Parse("2006-01-02", fv.MinValue); err == nil {
					conds = append(conds, filterCond{Col: fv.Column, Op: ">=", Val: t})
				}
			}
			if fv.MaxValue != "" {
				if t, err := time.Parse("2006-01-02", fv.MaxValue); err == nil {
					// limite superior exclusivo no dia seguinte: cobre o dia
					// inteiro sem depender da precisão de hora armazenada.
					conds = append(conds, filterCond{Col: fv.Column, Op: "<", Val: t.AddDate(0, 0, 1)})
				}
			}
			continue
		}
		if fv.IsRange {
			if fv.MinValue != "" {
				if val, err := convertFilterScalar(fv.MinValue, f.Kind); err == nil {
					conds = append(conds, filterCond{Col: fv.Column, Op: ">=", Val: val})
				}
			}
			if fv.MaxValue != "" {
				if val, err := convertFilterScalar(fv.MaxValue, f.Kind); err == nil {
					conds = append(conds, filterCond{Col: fv.Column, Op: "<=", Val: val})
				}
			}
			continue
		}
		if fv.Value != "" {
			if val, err := convertFilterScalar(fv.Value, f.Kind); err == nil {
				conds = append(conds, filterCond{Col: fv.Column, Op: "=", Val: val})
			}
		}
	}
	return conds
}

// filterURLParams devolve os parâmetros f_* atualmente ativos, para
// preservar os filtros aplicados nos links de ordenação/paginação da
// listagem (que, de outra forma, perderiam o filtro a cada clique).
func filterURLParams(filters []filterView) map[string]string {
	if len(filters) == 0 {
		return nil
	}
	params := make(map[string]string, len(filters)*2)
	for _, fv := range filters {
		if fv.IsRange {
			if fv.MinValue != "" {
				params["f_"+fv.Column+"_min"] = fv.MinValue
			}
			if fv.MaxValue != "" {
				params["f_"+fv.Column+"_max"] = fv.MaxValue
			}
			continue
		}
		if fv.Value != "" {
			params["f_"+fv.Column] = fv.Value
		}
	}
	return params
}

// convertFilterScalar converte o valor bruto (string, vindo da query
// string) para o tipo Go real da coluna — necessário pro placeholder da
// query bater com o tipo da coluna em bancos estritos como PostgreSQL
// (mesmo motivo de parsePKArg, generalizado para qualquer campo filtrável).
func convertFilterScalar(raw string, kind reflect.Kind) (any, error) {
	switch kind {
	case reflect.Bool:
		return raw == "true" || raw == "1", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("valor inválido: %q", raw)
		}
		return n, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("valor inválido: %q", raw)
		}
		return n, nil
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("valor inválido: %q", raw)
		}
		return n, nil
	default:
		return raw, nil
	}
}
