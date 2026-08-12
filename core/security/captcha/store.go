package captcha

import (
	"log"
	"strings"

	"kyrux/core/router"
	"kyrux/core/security/session"
)

const sessionKey = "kyrux_captcha_code"

// Store liga New/PNG à sessão do visitante — a parte que, sem isso, cada
// app teria que escrever na mão (gerar, guardar, servir a imagem, conferir
// a resposta). Um Store não guarda nada em si: toda leitura/escrita passa
// pelo *session.Store informado em NewStore, então múltiplos apps no mesmo
// processo podem ter cada um o seu Store sem conflito — a chave de sessão
// é a mesma, mas a sessão em si já é por visitante.
type Store struct {
	sessions *session.Store
}

// NewStore cria um Store de captcha sobre o *session.Store do framework
// (normalmente fw.Sessions).
func NewStore(sessions *session.Store) *Store {
	return &Store{sessions: sessions}
}

// ImageHandler devolve um router.HandlerFunc pronto pra registrar como rota
// GET — a cada requisição gera um código novo, substitui o anterior na
// sessão do visitante (criando a sessão se ainda não existir) e serve a
// imagem PNG (sem cache: Cache-Control: no-store, pra um "gerar outro" no
// front nunca reusar uma imagem antiga do navegador).
func (s *Store) ImageHandler() router.HandlerFunc {
	return func(ctx *router.Context) {
		code, err := New()
		if err != nil {
			log.Printf("captcha: erro ao gerar código: %v", err)
			ctx.Error(500)
			return
		}
		s.setCode(ctx, code)

		png, err := PNG(code)
		if err != nil {
			log.Printf("captcha: erro ao renderizar imagem: %v", err)
			ctx.Error(500)
			return
		}

		h := ctx.Writer.Header()
		h.Set("Content-Type", "image/png")
		h.Set("Cache-Control", "no-store")
		ctx.Writer.Write(png)
	}
}

// Verify confere answer contra o código guardado na sessão pela última
// resposta de ImageHandler, e consome o código (uso único) independente do
// resultado — uma nova imagem é sempre necessária pra uma nova tentativa,
// evitando reenvio automatizado da mesma resposta.
func (s *Store) Verify(ctx *router.Context, answer string) bool {
	sess, ok := session.FromRequest(ctx.Request, s.sessions)
	if !ok {
		return false
	}
	codeVal, ok := sess.Get(sessionKey)
	if !ok {
		return false
	}
	sess.Delete(sessionKey)
	code, _ := codeVal.(string)
	return code != "" && strings.TrimSpace(answer) == code
}

func (s *Store) setCode(ctx *router.Context, code string) {
	sess, ok := session.FromRequest(ctx.Request, s.sessions)
	if !ok {
		var err error
		sess, err = s.sessions.New()
		if err != nil {
			log.Printf("captcha: não foi possível criar sessão: %v", err)
			return
		}
		session.SetCookie(ctx.Writer, sess.ID, ctx.Request.TLS != nil)
	}
	sess.Set(sessionKey, code)
}
