package admin

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

// maxUploadMemory é o limite mantido em memória pelo parser multipart antes
// de derramar para arquivo temporário — mesmo teto documentado para
// MaxBodySize por padrão (USE.md).
const maxUploadMemory = 32 << 20 // 32MB

// allowedImageTypes é checado contra o Content-Type detectado a partir dos
// bytes reais do arquivo (não a extensão nem o header enviado pelo
// navegador, que são só metadados não confiáveis).
var allowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

var reUnsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// sanitizeFilename remove qualquer componente de caminho (impede path
// traversal via nome de arquivo, ex: "../../etc/passwd") e caracteres fora
// de um alfabeto seguro.
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = reUnsafeFilenameChars.ReplaceAllString(name, "_")
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "arquivo"
	}
	return name
}

// saveUploadedImage grava o arquivo de fh em medias/<app>/<table>/ com um
// nome único (evita colisão e nomes previsíveis) e devolve o caminho
// público (/medias/<app>/<table>/<nome>) pronto para gravar no campo do
// model. Valida o conteúdo real do arquivo contra allowedImageTypes —
// rejeita qualquer coisa que não seja uma imagem, mesmo que a extensão
// minta.
func saveUploadedImage(app, table string, fh *multipart.FileHeader) (string, error) {
	src, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("não foi possível abrir o arquivo enviado")
	}
	defer src.Close()

	head := make([]byte, 512)
	n, _ := io.ReadFull(src, head)
	contentType := http.DetectContentType(head[:n])
	if !allowedImageTypes[contentType] {
		return "", fmt.Errorf("arquivo não é uma imagem suportada (jpeg, png, gif ou webp)")
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("não foi possível ler o arquivo enviado")
	}

	dir := filepath.Join("medias", app, table)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("não foi possível preparar o diretório de mídia")
	}

	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		return "", fmt.Errorf("falha ao gerar nome de arquivo")
	}
	finalName := hex.EncodeToString(idBytes) + "_" + sanitizeFilename(fh.Filename)
	dstPath := filepath.Join(dir, finalName)

	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("não foi possível salvar o arquivo")
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(dstPath)
		return "", fmt.Errorf("falha ao gravar o arquivo")
	}

	return "/medias/" + app + "/" + table + "/" + finalName, nil
}

// parseAdminForm popula r.PostForm (campos) e r.MultipartForm (arquivos).
// Sempre tenta multipart — é um superset seguro: ParseMultipartForm chama
// ParseForm internamente antes de tentar o parse multipart, então formulários
// sem nenhum campo de arquivo (Content-Type comum) continuam funcionando
// normalmente, só retornando ErrNotMultipart (ignorado aqui).
func parseAdminForm(r *http.Request) error {
	err := r.ParseMultipartForm(maxUploadMemory)
	if err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return err
	}
	return nil
}

// uploadFieldValue lê o arquivo enviado para o campo f (kyrux:"image") a
// partir do multipart form de r. ok=false quando nenhum arquivo foi
// selecionado (comum no edit — mantém o valor atual, não é erro).
func uploadFieldValue(r *http.Request, app, table string, f adminField) (path string, ok bool, err error) {
	file, header, err := r.FormFile(f.Column)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("campo %q: arquivo inválido", f.Label)
	}
	file.Close()

	path, err = saveUploadedImage(app, table, header)
	if err != nil {
		return "", false, fmt.Errorf("campo %q: %w", f.Label, err)
	}
	return path, true, nil
}
