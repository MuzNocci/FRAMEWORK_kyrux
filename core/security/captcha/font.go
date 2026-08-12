package captcha

// glyphs é uma fonte bitmap 5x7 (largura x altura) só com os dígitos 0-9 —
// suficiente pro captcha (código só numérico, evita a ambiguidade visual
// que letras+dígitos misturados trariam, tipo 0/O ou 1/I/l). Desenhada na
// mão, sem golang.org/x/image nem nenhuma dependência de terceiros — mesma
// filosofia do resto do framework (ver core/adapters/smtp).
var glyphs = map[byte][glyphHeight]string{
	'0': {
		"01110",
		"10001",
		"10011",
		"10101",
		"11001",
		"10001",
		"01110",
	},
	'1': {
		"00100",
		"01100",
		"00100",
		"00100",
		"00100",
		"00100",
		"01110",
	},
	'2': {
		"01110",
		"10001",
		"00001",
		"00010",
		"00100",
		"01000",
		"11111",
	},
	'3': {
		"11111",
		"00010",
		"00100",
		"00010",
		"00001",
		"10001",
		"01110",
	},
	'4': {
		"00010",
		"00110",
		"01010",
		"10010",
		"11111",
		"00010",
		"00010",
	},
	'5': {
		"11111",
		"10000",
		"11110",
		"00001",
		"00001",
		"10001",
		"01110",
	},
	'6': {
		"00110",
		"01000",
		"10000",
		"11110",
		"10001",
		"10001",
		"01110",
	},
	'7': {
		"11111",
		"00001",
		"00010",
		"00100",
		"01000",
		"01000",
		"01000",
	},
	'8': {
		"01110",
		"10001",
		"10001",
		"01110",
		"10001",
		"10001",
		"01110",
	},
	'9': {
		"01110",
		"10001",
		"10001",
		"01111",
		"00001",
		"00010",
		"01100",
	},
}

const (
	glyphWidth  = 5
	glyphHeight = 7
)
