package orm

import "kyrux/core/database"

// Prefetch carrega os registros R relacionados a um conjunto de chaves em
// UMA query e os agrupa pelo valor da FK — é o prefetch_related do Django,
// explícito e sem N+1 invisível:
//
//	posts, _ := orm.From[Post](db).All()
//	ids := make([]int64, len(posts))
//	for i, p := range posts { ids[i] = p.ID }
//
//	// 1 query: SELECT * FROM comentarios WHERE post_id IN (...)
//	porPost, _ := orm.Prefetch[Comentario](db, "post_id", ids,
//	    func(c *Comentario) int64 { return c.PostID })
//
//	for _, p := range posts {
//	    comentarios := porPost[p.ID] // []Comentario do post
//	}
//
// keyOf extrai de cada R o valor usado no agrupamento (a coluna col).
// Chaves duplicadas em keys são deduplicadas antes da query.
func Prefetch[R any, K comparable](db *database.DB, col string, keys []K, keyOf func(*R) K) (map[K][]R, error) {
	if len(keys) == 0 {
		return map[K][]R{}, nil
	}
	uniq := dedupe(keys)
	rels, err := FromDB[R](db).WhereIn(col, uniq).All()
	if err != nil {
		return nil, err
	}
	return groupByKey(rels, keyOf), nil
}

// PrefetchTx é a variante de Prefetch dentro de uma transação.
func PrefetchTx[R any, K comparable](tx *database.Tx, col string, keys []K, keyOf func(*R) K) (map[K][]R, error) {
	if len(keys) == 0 {
		return map[K][]R{}, nil
	}
	uniq := dedupe(keys)
	rels, err := FromTx[R](tx).WhereIn(col, uniq).All()
	if err != nil {
		return nil, err
	}
	return groupByKey(rels, keyOf), nil
}

// dedupe remove duplicatas preservando a ordem de primeira ocorrência.
func dedupe[K comparable](keys []K) []any {
	seen := make(map[K]struct{}, len(keys))
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	return out
}

// groupByKey agrupa items pelo valor devolvido por keyOf.
func groupByKey[R any, K comparable](items []R, keyOf func(*R) K) map[K][]R {
	grouped := make(map[K][]R, len(items))
	for i := range items {
		k := keyOf(&items[i])
		grouped[k] = append(grouped[k], items[i])
	}
	return grouped
}
