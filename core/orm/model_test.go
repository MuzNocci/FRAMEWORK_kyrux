package orm

import "testing"

// ── validação de metadata (buildMeta) ───────────────────────────────────────
//
// column:, fk:, fklabel: e default: das tags kyrux entram direto na
// construção de SQL estrutural (sem placeholder) — buildMeta deve panicar
// cedo (na primeira vez que o model é usado) em vez de deixar um valor
// inválido virar SQL malformado ou injetável mais adiante.

type modelBadColumn struct {
	ID   int64  `kyrux:"pk"`
	Nome string `kyrux:"column:nome; DROP TABLE x"`
}

func TestBuildMetaRejeitaColunaInvalida(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para coluna inválida")
		}
	}()
	_ = metaOf[modelBadColumn]()
}

type modelBadFK struct {
	ID     int64 `kyrux:"pk"`
	UserID int64 `kyrux:"fk:users; DROP TABLE x"`
}

func TestBuildMetaRejeitaFKInvalida(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para fk inválida")
		}
	}()
	_ = metaOf[modelBadFK]()
}

type modelBadFKLabel struct {
	ID     int64 `kyrux:"pk"`
	UserID int64 `kyrux:"fk:users,fklabel:nome; DROP TABLE x"`
}

func TestBuildMetaRejeitaFKLabelInvalido(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para fklabel inválido")
		}
	}()
	_ = metaOf[modelBadFKLabel]()
}

type modelBadDefault struct {
	ID   int64  `kyrux:"pk"`
	Nome string `kyrux:"default:'x'); DROP TABLE users; --"`
}

func TestBuildMetaRejeitaDefaultInvalido(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic para default inválido")
		}
	}()
	_ = metaOf[modelBadDefault]()
}

// modelGoodMeta cobre as formas válidas de default que a regex precisa aceitar.
type modelGoodMeta struct {
	ID     int64   `kyrux:"pk"`
	Ativo  bool    `kyrux:"default:true"`
	Criado string  `kyrux:"default:NOW()"`
	Preco  float64 `kyrux:"default:0"`
	Status string  `kyrux:"default:'pending'"`
	UserID int64   `kyrux:"fk:users,fklabel:nome"`
}

func TestBuildMetaAceitaFormasValidas(t *testing.T) {
	meta := metaOf[modelGoodMeta]()
	if len(meta.Fields) != 6 {
		t.Fatalf("esperava 6 campos, recebeu %d", len(meta.Fields))
	}
	f, ok := meta.ColToField["user_id"]
	if !ok || f.FK != "users" || f.FKLabel != "nome" {
		t.Errorf("fk/fklabel não foram preservados corretamente: %+v", f)
	}
}
