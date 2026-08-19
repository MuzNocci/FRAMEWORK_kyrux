package orm

import "crypto/rand"

// nanoIDAlphabet segue o alfabeto padrão do NanoID (A-Za-z0-9-_) — URL-safe,
// sem precisar de encoding extra pra aparecer numa rota. Exatos 64 símbolos
// (potência de 2): mapear um byte aleatório com &63 dá distribuição uniforme
// sem viés, sem precisar de rejection sampling.
const nanoIDAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-_"

// defaultNanoIDSize é o tamanho padrão do NanoID (21 caracteres) quando o
// campo não define size:N — mesmo padrão da biblioteca de referência do
// NanoID, dá colisão desprezível pro volume de qualquer tabela deste projeto.
const defaultNanoIDSize = 21

// generateNanoID gera um ID aleatório único do tamanho pedido (ou
// defaultNanoIDSize se size <= 0), sem dependência de terceiros — mesma
// filosofia do gerador de UUID em core/security/auth.
func generateNanoID(size int) string {
	if size <= 0 {
		size = defaultNanoIDSize
	}
	buf := make([]byte, size)
	rand.Read(buf)
	out := make([]byte, size)
	for i, b := range buf {
		out[i] = nanoIDAlphabet[b&63]
	}
	return string(out)
}
