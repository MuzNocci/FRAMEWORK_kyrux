package admin

import (
	"fmt"

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

// fetchFKOptions carrega as linhas existentes de table para popular o
// <select> de um campo kyrux:"fk:table". labelCol vazio usa o próprio id
// como rótulo (funcional, porém menos legível — configure kyrux:"fklabel:coluna"
// pra um rótulo melhor, ex: "nome" ou "titulo").
//
// table/labelCol vêm sempre de tags kyrux escritas pelo desenvolvedor no
// código-fonte, nunca de entrada de usuário — interpolar direto na query
// segue o mesmo nível de confiança já usado em todo o resto do pacote orm
// para nomes de tabela/coluna.
func fetchFKOptions(db *database.DB, table, labelCol string) ([]fkOption, error) {
	labelExpr := "id"
	if labelCol != "" {
		labelExpr = labelCol
	}
	query := fmt.Sprintf("SELECT id, %s FROM %s ORDER BY %s LIMIT %d", labelExpr, table, labelExpr, fkOptionsLimit)
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
