package router

import (
	"regexp"
	"strings"
)

// reKyruxParam captura a sintaxe <nome:tipo> usada pelo desenvolvedor.
var reKyruxParam = regexp.MustCompile(`<([a-zA-Z_][a-zA-Z0-9_]*):([a-zA-Z]+)>`)

// reGoParam captura {nome} e {nome...} do ServeMux para extração de nomes.
var reGoParam = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)(?:\.\.\.)?}`)

// convertPattern converte a sintaxe Kyrux <nome:tipo> para a sintaxe do
// Go ServeMux {nome}. Tipos suportados:
//
//	str, string, int, float, slug, uuid  →  {nome}   (segmento sem barra)
//	path                                 →  {nome...} (segmentos com barra)
func convertPattern(pattern string) string {
	return reKyruxParam.ReplaceAllStringFunc(pattern, func(match string) string {
		sub := reKyruxParam.FindStringSubmatch(match)
		name, typ := sub[1], sub[2]
		if typ == "path" {
			return "{" + name + "...}"
		}
		return "{" + name + "}"
	})
}

// extractParamNames retorna os nomes dos parâmetros de path presentes no padrão compilado.
func extractParamNames(pattern string) []string {
	matches := reGoParam.FindAllStringSubmatch(pattern, -1)
	if len(matches) == 0 {
		return nil
	}
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = m[1]
	}
	return names
}

// displayPath converte o padrão interno {nome} de volta para a notação <nome>
// usada na debug page (remove também o anchor {$}).
func displayPath(path string) string {
	path = strings.TrimSuffix(path, "{$}")
	path = reGoParam.ReplaceAllStringFunc(path, func(match string) string {
		sub := reGoParam.FindStringSubmatch(match)
		name := sub[1]
		if strings.Contains(match, "...") {
			return "<" + name + ":path>"
		}
		return "<" + name + ">"
	})
	return path
}

// semHost tira o domínio de um caminho com host ("site.com/rota/" → "/rota/").
// Sem "/" depois do domínio, não sobra caminho e devolve "".
func semHost(path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	if i := strings.Index(path, "/"); i >= 0 {
		return path[i:]
	}
	return ""
}

// slashAlternate retorna a variante complementar do padrão (com/sem barra final),
// permitindo que /rota e /rota/ sejam tratados pelo mesmo handler.
// Retorna "" quando não há alternativa válida (root "/", wildcards "...").
// Padrões com domínio ("GET site.com/rota/") seguem a mesma regra, olhando
// só o caminho: a raiz de um domínio não tem variante sem barra.
func slashAlternate(pattern string) string {
	if strings.Contains(pattern, "...}") {
		return ""
	}
	if strings.HasSuffix(pattern, "/{$}") {
		alt := strings.TrimSuffix(pattern, "/{$}")
		// Garante que sobra um path válido após remover "/{$}".
		// Ex: "GET /{$}" → alt="GET " → path="" → inválido, retorna "".
		// Ex: "GET site.com/{$}" → alt="GET site.com" → sem caminho, retorna "".
		_, path, _ := strings.Cut(alt, " ")
		if semHost(path) == "" {
			return ""
		}
		return alt
	}
	// Não gera alternativa para a raiz "/" (já coberta pelo caso acima).
	_, path, _ := strings.Cut(pattern, " ")
	if semHost(path) == "/" {
		return ""
	}
	return pattern + "/{$}"
}
