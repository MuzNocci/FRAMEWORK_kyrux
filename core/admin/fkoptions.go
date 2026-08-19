package admin

import (
	"fmt"
	"strings"

	"kyrux/core/database"
)

// fkOptionsLimit evita carregar uma tabela inteira num único <select> — acima
// disso o campo continua funcional (aceita o id atual), só não lista tudo.
const fkOptionsLimit = 1000

// fkOption é um par id/rótulo pronto para <option value="Value">Label</option>.
type fkOption struct {
	Value string
	Label string
}

// findLocalFKColumn procura, entre os models registrados no admin, aquele
// cuja tabela é table e devolve a coluna de um campo seu com
// kyrux:"fk:targetTable" — usado por fetchFKOptions pra resolver o JOIN
// implícito de um fklabel no formato "tabela.coluna" (ver buildFKLabelExpr).
func findLocalFKColumn(table, targetTable string) (string, bool) {
	for _, rm := range modelsOrdered() {
		if rm.Table != table {
			continue
		}
		for _, f := range rm.Fields {
			if f.IsFK && f.FKTable == targetTable {
				return f.Column, true
			}
		}
	}
	return "", false
}

// buildFKLabelExpr monta a expressão SQL do rótulo a partir de labelCol
// (kyrux:"fklabel:..."): partes separadas por "+", cada uma "coluna" (do
// próprio table) ou "tabela.coluna" (segue o FK de table até essa tabela,
// via LEFT JOIN resolvido por findLocalFKColumn). Múltiplas partes são
// concatenadas com " - " e o resultado sai em maiúsculas (UPPER), pra
// destacar o rótulo no <select> — ex: "ERINK MARCELO - CIPETRAN NORTE".
// Devolve a expressão SELECT e as cláusulas JOIN necessárias, ou erro se
// alguma "tabela.coluna" não corresponder a nenhum campo kyrux:"fk:tabela"
// em table.
func buildFKLabelExpr(table, labelCol string) (expr string, joins []string, err error) {
	if labelCol == "" {
		return table + ".id", nil, nil
	}
	var selectParts []string
	seen := map[string]string{} // joinTable -> alias já criado, evita JOIN duplicado
	for i, part := range strings.Split(labelCol, "+") {
		if dot := strings.IndexByte(part, '.'); dot >= 0 {
			joinTable, remoteCol := part[:dot], part[dot+1:]
			alias, ok := seen[joinTable]
			if !ok {
				localCol, ok := findLocalFKColumn(table, joinTable)
				if !ok {
					return "", nil, fmt.Errorf("fklabel %q: %q não tem campo kyrux:\"fk:%s\"", labelCol, table, joinTable)
				}
				alias = fmt.Sprintf("fkj%d", i)
				seen[joinTable] = alias
				joins = append(joins, fmt.Sprintf("LEFT JOIN %s %s ON %s.id = %s.%s", joinTable, alias, alias, table, localCol))
			}
			selectParts = append(selectParts, fmt.Sprintf("COALESCE(%s.%s, '')", alias, remoteCol))
		} else {
			selectParts = append(selectParts, fmt.Sprintf("COALESCE(%s.%s, '')", table, part))
		}
	}
	return "UPPER(" + strings.Join(selectParts, " || ' - ' || ") + ")", joins, nil
}

// fetchFKOptions carrega as linhas existentes de table para popular o
// <select> de um campo kyrux:"fk:table". labelCol vazio usa o próprio id
// como rótulo (funcional, porém menos legível — configure kyrux:"fklabel:..."
// pra um rótulo melhor, ex: "nome" ou "titulo", ou "clientes.nome+titulo"
// pra combinar uma coluna de uma tabela relacionada com uma coluna local).
//
// table/labelCol vêm sempre de tags kyrux escritas pelo desenvolvedor no
// código-fonte, nunca de entrada de usuário — interpolar direto na query
// segue o mesmo nível de confiança já usado em todo o resto do pacote orm
// para nomes de tabela/coluna.
func fetchFKOptions(db *database.DB, table, labelCol string) ([]fkOption, error) {
	labelExpr, joins, err := buildFKLabelExpr(table, labelCol)
	if err != nil {
		return nil, fmt.Errorf("admin: carregar opções de %q: %w", table, err)
	}
	query := fmt.Sprintf(
		"SELECT %s.id, %s AS label FROM %s %s ORDER BY label LIMIT %d",
		table, labelExpr, table, strings.Join(joins, " "), fkOptionsLimit,
	)
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("admin: carregar opções de %q: %w", table, err)
	}
	defer rows.Close()

	var opts []fkOption
	for rows.Next() {
		var idVal, labelVal any
		if err := rows.Scan(&idVal, &labelVal); err != nil {
			return nil, fmt.Errorf("admin: ler opções de %q: %w", table, err)
		}
		opts = append(opts, fkOption{Value: cellToString(idVal), Label: cellToString(labelVal)})
	}
	return opts, rows.Err()
}

// resolveFKLabelsForList substitui, em cada linha de rows, o valor bruto
// (id numérico) de todo campo kyrux:"fk:..." com fklabel configurado pelo
// rótulo correspondente — a mesma resolução usada no <select> do formulário,
// aplicada agora também às colunas da listagem. Um id sem opção
// correspondente (registro relacionado apagado, ou além do fkOptionsLimit)
// mantém o id bruto como fallback. Roda uma query extra por campo FK
// rotulado, uma vez por página da listagem — aceitável no admin (baixo
// tráfego), no mesmo espírito de fetchFKOptions no formulário.
func resolveFKLabelsForList(db *database.DB, fields []adminField, rows []adminRow) error {
	for _, f := range fields {
		if !f.IsFK || f.FKLabel == "" {
			continue
		}
		opts, err := fetchFKOptions(db, f.FKTable, f.FKLabel)
		if err != nil {
			return err
		}
		byID := make(map[string]string, len(opts))
		for _, o := range opts {
			byID[o.Value] = o.Label
		}
		for i := range rows {
			if label, ok := byID[rows[i].Values[f.Column]]; ok {
				rows[i].Values[f.Column] = label
			}
		}
	}
	return nil
}

// cellToString converte o valor bruto devolvido pelo driver SQL (int64,
// float64, []byte, string, nil, ...) para exibição — []byte precisa de
// conversão explícita, senão vira uma lista de bytes em vez de texto.
func cellToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}
