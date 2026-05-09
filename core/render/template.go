package render

import (
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reExtends  = regexp.MustCompile(`(?m)\{%-?\s*extends\s+"([^"]+)"\s*-?%\}\n?`)
	reBlock    = regexp.MustCompile(`\{%-?\s*block\s+"([^"]+)"\s*-?%\}`)
	reEndBlock = regexp.MustCompile(`\{%-?\s*endblock(?:\s+"[^"]*")?\s*-?%\}`)
	reInclude  = regexp.MustCompile(`\{%-?\s*include\s+"([^"]+)"\s*-?%\}`)
	reTmplRef  = regexp.MustCompile(`\{\{-?\s*template\s+"([^"]+)"`)
)

type srcInfo struct {
	content string
	parent  string // "" if no extends
}

type compiledEntry struct {
	set      *template.Template
	execName string
}

// safeTmplPath valida que um caminho de template não tenta path traversal.
func safeTmplPath(p string) bool {
	return !strings.Contains(p, "..") && !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "\\")
}

// preprocess converte a sintaxe Django-like para sintaxe Go template.
func preprocess(raw string) (content, parent string, err error) {
	if m := reExtends.FindStringSubmatch(raw); m != nil {
		parent = m[1]
		if !safeTmplPath(parent) {
			return "", "", fmt.Errorf("render: caminho inválido em extends: %q", parent)
		}
		raw = reExtends.ReplaceAllString(raw, "")
		raw = reBlock.ReplaceAllStringFunc(raw, func(s string) string {
			return `{{define "` + reBlock.FindStringSubmatch(s)[1] + `"}}`
		})
	} else {
		raw = reBlock.ReplaceAllStringFunc(raw, func(s string) string {
			return `{{block "` + reBlock.FindStringSubmatch(s)[1] + `" .}}`
		})
	}
	raw = reEndBlock.ReplaceAllString(raw, `{{end}}`)
	var includeErr error
	raw = reInclude.ReplaceAllStringFunc(raw, func(s string) string {
		if includeErr != nil {
			return s
		}
		name := reInclude.FindStringSubmatch(s)[1]
		if !safeTmplPath(name) {
			includeErr = fmt.Errorf("render: caminho inválido em include: %q", name)
			return s
		}
		return `{{template "` + name + `" .}}`
	})
	if includeErr != nil {
		return "", "", includeErr
	}
	return strings.TrimSpace(raw), parent, nil
}

func tmplRefs(content string) []string {
	ms := reTmplRef.FindAllStringSubmatch(content, -1)
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m[1])
	}
	return out
}

func (e *Engine) loadSources() error {
	sources := map[string]srcInfo{}

	if _, err := os.Stat(e.dir); os.IsNotExist(err) {
		e.sources = sources
		e.compiled = map[string]*compiledEntry{}
		return nil
	}

	absBase, _ := filepath.Abs(e.dir)
	err := filepath.WalkDir(e.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".html" {
			return err
		}
		absPath, _ := filepath.Abs(path)
		if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
			return fmt.Errorf("render: path traversal detectado: %s", path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(e.dir, path)
		name := filepath.ToSlash(rel)
		content, parent, perr := preprocess(string(raw))
		if perr != nil {
			return perr
		}
		sources[name] = srcInfo{content: content, parent: parent}
		return nil
	})
	if err != nil {
		return err
	}

	e.sources = sources

	compiled := map[string]*compiledEntry{}
	for name := range sources {
		ce, err := e.compile(name)
		if err != nil {
			return fmt.Errorf("template '%s': %w", name, err)
		}
		compiled[name] = ce
	}
	e.compiled = compiled
	return nil
}

// compile monta o template set completo para renderizar name,
// unindo parent + child + todos os partials incluídos transitivamente.
func (e *Engine) compile(name string) (*compiledEntry, error) {
	info, ok := e.sources[name]
	if !ok {
		return nil, fmt.Errorf("não encontrado")
	}

	collected := map[string]string{}
	execName := name

	if info.parent != "" {
		execName = info.parent
		collected[name] = info.content
		for _, ref := range tmplRefs(info.content) {
			e.collectTransitive(ref, collected)
		}
		e.collectTransitive(info.parent, collected)
	} else {
		e.collectTransitive(name, collected)
	}

	// cria o set com execName como root (será o template executado)
	root := template.New(execName).Funcs(templateFuncs).Option("missingkey=error")
	if c, ok := collected[execName]; ok {
		if _, err := root.Parse(c); err != nil {
			return nil, err
		}
		delete(collected, execName)
	}
	for n, c := range collected {
		if _, err := root.New(n).Parse(c); err != nil {
			return nil, fmt.Errorf("'%s': %w", n, err)
		}
	}

	return &compiledEntry{set: root, execName: execName}, nil
}

// sourceOf retorna o conteúdo pré-processado de um template pelo nome.
func (e *Engine) sourceOf(name string) (string, bool) {
	e.mu.RLock()
	info, ok := e.sources[name]
	e.mu.RUnlock()
	if !ok {
		return "", false
	}
	return info.content, true
}

// collectTransitive adiciona name e todos os {{template}} que ele referencia.
func (e *Engine) collectTransitive(name string, dst map[string]string) {
	if _, seen := dst[name]; seen {
		return
	}
	info, ok := e.sources[name]
	if !ok {
		return
	}
	dst[name] = info.content
	for _, ref := range tmplRefs(info.content) {
		e.collectTransitive(ref, dst)
	}
}
