package captcha

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
)

const (
	scale   = 6 // cada pixel da fonte vira um bloco scale x scale na imagem final
	charGap = 4 // espaço horizontal entre caracteres
	marginX = 12
	marginY = 14
)

// PNG renderiza code (só dígitos — ver New) como uma imagem distorcida:
// ruído de fundo (linhas e pontos aleatórios), cor e deslocamento vertical
// variando por caractere, e uma leve inclinação — o suficiente pra
// dificultar OCR automatizado simples sem exigir nenhuma lib de terceiros
// (só image/image/png da stdlib). Dígitos fora de 0-9 em code são ignorados
// (não desenha nada pra eles, mas não retorna erro).
func PNG(code string) ([]byte, error) {
	if code == "" {
		return nil, fmt.Errorf("captcha: code vazio")
	}

	charW := glyphWidth * scale
	charH := glyphHeight * scale
	width := marginX*2 + len(code)*charW + (len(code)-1)*charGap
	height := marginY*2 + charH

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	fillBackground(img, width, height)

	for range 4 {
		drawNoiseLine(img, width, height)
	}
	for range width * height / 18 {
		x, y := rand.IntN(width), rand.IntN(height)
		img.Set(x, y, noiseDotColor())
	}

	palette := []color.RGBA{
		{40, 50, 90, 255},
		{80, 30, 60, 255},
		{20, 70, 50, 255},
		{60, 40, 20, 255},
	}

	x := marginX
	for _, ch := range []byte(code) {
		rows, ok := glyphs[ch]
		if ok {
			yJitter := rand.IntN(9) - 4 // -4..+4 px
			shear := rand.IntN(3) - 1   // -1..+1, inclinação leve por linha
			col := palette[rand.IntN(len(palette))]
			drawGlyph(img, rows, x, marginY+yJitter, shear, col)
		}
		x += charW + charGap
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("captcha: codificar PNG: %w", err)
	}
	return buf.Bytes(), nil
}

func fillBackground(img *image.RGBA, width, height int) {
	bg := color.RGBA{245, 245, 248, 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, bg)
		}
	}
}

func drawGlyph(img *image.RGBA, rows [glyphHeight]string, x0, y0, shear int, col color.RGBA) {
	for row := 0; row < glyphHeight; row++ {
		shiftedX := x0 + shear*row/2
		for c := 0; c < glyphWidth; c++ {
			if rows[row][c] != '1' {
				continue
			}
			px := shiftedX + c*scale
			py := y0 + row*scale
			for dy := 0; dy < scale-1; dy++ {
				for dx := 0; dx < scale-1; dx++ {
					img.Set(px+dx, py+dy, col)
				}
			}
		}
	}
}

// drawNoiseLine desenha uma linha reta com leve variação, de um ponto
// aleatório da borda esquerda a um ponto aleatório da borda direita.
func drawNoiseLine(img *image.RGBA, width, height int) {
	col := color.RGBA{
		uint8(150 + rand.IntN(60)),
		uint8(150 + rand.IntN(60)),
		uint8(150 + rand.IntN(60)),
		255,
	}
	y0, y1 := rand.IntN(height), rand.IntN(height)
	for x := 0; x < width; x++ {
		t := float64(x) / float64(width)
		y := int(float64(y0)*(1-t) + float64(y1)*t)
		img.Set(x, y, col)
		img.Set(x, y+1, col)
	}
}

func noiseDotColor() color.RGBA {
	v := uint8(170 + rand.IntN(70))
	return color.RGBA{v, v, v, 255}
}
