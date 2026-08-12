// Package captcha gera desafios visuais simples (código numérico
// distorcido, renderizado como PNG) pra formulários públicos — sem
// depender de nenhum serviço externo (Google reCAPTCHA, hCaptcha, etc).
//
// New e PNG são as primitivas de baixo nível: geram o código e desenham a
// imagem, sem opinar em como o código esperado é guardado. Store é o uso
// pronto pra maioria dos casos — liga as duas a uma sessão do framework
// (kyrux/core/security/session), com ImageHandler (rota GET, já serve a
// imagem) e Verify (confere e consome a resposta):
//
//	captchaStore := captcha.NewStore(fw.Sessions)
//	router.Path("GET", "/captcha/image", captchaStore.ImageHandler(), "captcha_image")
//	// no handler de POST do formulário:
//	if !captchaStore.Verify(ctx, ctx.Request.FormValue("captcha_answer")) { ... }
//
// Um app que precise guardar o código em outro lugar (ex: banco, pra um
// fluxo sem sessão de cookie) usa New/PNG direto, sem o Store.
package captcha

import (
	"crypto/rand"
	"strings"
)

// CodeLength é o tamanho do código gerado por New.
const CodeLength = 5

// New gera um código aleatório de CodeLength dígitos (0-9), criptograficamente
// seguro (crypto/rand — o código em si é o segredo do desafio, diferente do
// ruído puramente visual usado em PNG).
func New() (string, error) {
	digits := make([]byte, CodeLength)
	if _, err := rand.Read(digits); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, d := range digits {
		sb.WriteByte('0' + d%10)
	}
	return sb.String(), nil
}
