package captcha

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestNewGeraCodigoDeDigitos(t *testing.T) {
	code, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(code) != CodeLength {
		t.Fatalf("esperava %d dígitos, veio %q (%d)", CodeLength, code, len(code))
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			t.Fatalf("código %q tem caractere não-dígito: %q", code, r)
		}
	}
}

func TestNewGeraCodigosDiferentes(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		code, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		seen[code] = true
	}
	// 20 códigos de 5 dígitos aleatórios repetirem tudo seria virtualmente
	// impossível (1 em 10^5 de chance por par) — se cair abaixo disso é
	// sinal de gerador quebrado, não azar.
	if len(seen) < 15 {
		t.Fatalf("esperava a maioria dos 20 códigos distintos, só %d são", len(seen))
	}
}

func TestPNGGeraImagemValidaDoTamanhoEsperado(t *testing.T) {
	code := "12345"
	data, err := PNG(code)
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("PNG devolveu bytes vazios")
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("imagem gerada não é um PNG válido: %v", err)
	}

	bounds := img.Bounds()
	wantW := marginX*2 + len(code)*glyphWidth*scale + (len(code)-1)*charGap
	wantH := marginY*2 + glyphHeight*scale
	if bounds.Dx() != wantW || bounds.Dy() != wantH {
		t.Fatalf("dimensões da imagem = %dx%d, esperava %dx%d", bounds.Dx(), bounds.Dy(), wantW, wantH)
	}
}

func TestPNGNaoRepeteAImagemEntreChamadas(t *testing.T) {
	a, err := PNG("55555")
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	b, err := PNG("55555")
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	// Mesmo código, mas o ruído (jitter, linhas, pontos) é aleatório a
	// cada chamada — duas imagens do mesmo código não deveriam sair
	// byte-a-byte idênticas (prova que o ruído está de fato variando).
	if bytes.Equal(a, b) {
		t.Fatal("duas renderizações do mesmo código saíram idênticas — ruído não está variando")
	}
}

func TestPNGCodeVazioFalha(t *testing.T) {
	if _, err := PNG(""); err == nil {
		t.Fatal("esperava erro para code vazio")
	}
}

func TestPNGIgnoraCaracteresForaDe0a9SemErro(t *testing.T) {
	data, err := PNG("12a45")
	if err != nil {
		t.Fatalf("PNG não deveria falhar com caractere fora da fonte: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("imagem com caractere ignorado ainda deveria ser um PNG válido: %v", err)
	}
}

func TestCodeLengthConsistenteComNew(t *testing.T) {
	code, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if strings.TrimSpace(code) != code {
		t.Fatalf("código não deveria ter espaços: %q", code)
	}
}
