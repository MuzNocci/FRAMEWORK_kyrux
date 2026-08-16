// Package admin fornece um painel de administração opt-in por model —
// um só site (/admin/), mas nenhum model aparece nele a menos que o
// desenvolvedor registre explicitamente:
//
//	admin.Register[models.Produto]("produtos", "Produtos")
//
// Diferente do admin do Django, o Kyrux não expõe nada automaticamente:
// sem chamadas a Register, /admin/ nem é montado. O layout (HTML/CSS) segue
// o mesmo estilo visual da página de boas-vindas do framework — o dev pode
// copiar e adaptar os templates embutidos se quiser um visual próprio.
//
// Segurança: o acesso exige sessão ativa com IsStaff ou IsAdmin (model
// auth.User), verificado a cada requisição — não apenas no login. Campos
// kyrux:"hash" nunca são exibidos (nem mascarados a partir do valor real);
// hash/encrypt na escrita reaproveitam o caminho fail-closed do ORM
// (orm.Create/Query.Update), sem duplicar criptografia aqui.
package admin

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"kyrux/core/database"
	"kyrux/core/orm"
)

// reSlug valida o slug usado na URL (/admin/<slug>/): minúsculas, dígitos, hífen.
var reSlug = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// reservedSlugs são os sub-caminhos fixos de /admin/ — um model não pode usá-los.
var reservedSlugs = map[string]bool{"login": true, "logout": true}

var timeType = reflect.TypeOf(time.Time{})

// adminField descreve um campo do model para fins de exibição/edição no admin.
// Construído uma única vez em Register, a partir de orm.MetaOf + reflection.
type adminField struct {
	GoName    string
	Column    string
	Label     string // nome humanizado, ex: "Criado Em"
	GoIndex   int
	Widget    string // "text" | "number" | "number-float" | "checkbox" | "datetime" | "password" | "file" | "select"
	Optional  bool   // campo é ponteiro (*T) — em branco vira nil/NULL
	IsPK      bool
	IsHash    bool
	IsEncrypt bool
	IsAutoNow bool
	IsImage   bool
	IsFK      bool
	FKTable   string
	FKLabel   string // coluna do model relacionado usada como rótulo; vazio = mostra o id
}

// registeredModel é a representação type-erased de um Register[T] — as
// operações de CRUD ficam fechadas sobre T via closures (ver crud.go).
type registeredModel struct {
	Slug   string
	Label  string
	Conn   string
	App    string
	Table  string
	Fields []adminField

	listCols   []string // Column, na ordem de exibição da listagem
	searchCols []string // Column, apenas campos string pesquisáveis

	pkColumn string
	pkKind   reflect.Kind

	list        listFunc
	get         getFunc
	create      createFunc
	update      updateFunc
	delete      deleteFunc
	ensureTable ensureTableFunc

	// nomes de campo Go pendentes de resolução (setados pelas Options,
	// resolvidos após Fields ser construído).
	wantList   []string
	wantSearch []string
}

// Option configura o registro de um model no admin.
type Option func(*registeredModel)

// Conn define a conexão nomeada usada pelo model (padrão: "default").
func Conn(name string) Option {
	return func(rm *registeredModel) { rm.Conn = name }
}

// App identifica o app dono do model (o mesmo nome usado em
// bootstrap.RegisterApp/apps/<nome>) — obrigatório quando o model tem algum
// campo kyrux:"image", pois define a pasta de destino do upload
// (medias/<app>/<tabela>/).
func App(name string) Option {
	return func(rm *registeredModel) { rm.App = name }
}

// ListFields define quais campos aparecem na listagem, na ordem informada.
// Os nomes são os do struct Go (ex: "Nome"), não da coluna SQL.
// Padrão: todos os campos exceto os marcados kyrux:"hash".
func ListFields(names ...string) Option {
	return func(rm *registeredModel) { rm.wantList = names }
}

// SearchFields define quais campos (string) participam da busca (LIKE).
// Padrão: nenhum — a busca fica desativada até que seja configurada.
func SearchFields(names ...string) Option {
	return func(rm *registeredModel) { rm.wantSearch = names }
}

var (
	registryMu sync.RWMutex
	registry   = map[string]*registeredModel{}
	order      []string // ordem de registro, para a navegação lateral
)

// Register registra o model T no admin sob o slug informado (usado na URL:
// /admin/<slug>/); label é o nome exibido na navegação. Deve ser chamado no
// Register() do app — nunca em runtime de request — antes do bootstrap
// montar as rotas (admin.Mount). Faz panic em configuração inválida (slug
// duplicado/reservado, ausência de PK): erro de programação, falha no boot.
func Register[T any](slug, label string, opts ...Option) {
	if !reSlug.MatchString(slug) {
		panic(fmt.Sprintf("admin: slug inválido %q — use minúsculas, dígitos e hífen, começando por letra", slug))
	}
	if reservedSlugs[slug] {
		panic(fmt.Sprintf("admin: slug %q é reservado pelo próprio admin", slug))
	}

	meta := orm.MetaOf[T]()
	if meta.PKField == nil {
		panic(fmt.Sprintf("admin: model %q não tem campo kyrux:\"pk\" — obrigatório para o admin", label))
	}

	var zero T
	structType := reflect.TypeOf(zero)

	rm := &registeredModel{
		Slug:     slug,
		Label:    label,
		Conn:     "default",
		Table:    meta.Table,
		pkColumn: meta.PKField.Column,
		pkKind:   structType.Field(meta.PKField.GoIndex).Type.Kind(),
	}
	for _, o := range opts {
		o(rm)
	}

	rm.Fields = buildAdminFields(meta, structType)
	for _, f := range rm.Fields {
		if f.IsImage && rm.App == "" {
			panic(fmt.Sprintf("admin: model %q tem campo de imagem (%q) mas não foi registrado com admin.App(...) — obrigatório para saber em qual pasta de medias/ salvar o upload", label, f.Label))
		}
	}
	rm.listCols = resolveColumns(rm.Fields, rm.wantList, defaultListCols)
	rm.searchCols = resolveColumns(rm.Fields, rm.wantSearch, nil)

	registerCRUD[T](rm)

	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[slug]; exists {
		panic(fmt.Sprintf("admin: slug %q já registrado", slug))
	}
	registry[slug] = rm
	order = append(order, slug)
}

// buildAdminFields converte os Fields do ORM (coluna, tags) em adminFields
// (rótulo humanizado, widget de formulário) usando reflection sobre o struct.
func buildAdminFields(meta *orm.ModelMeta, t reflect.Type) []adminField {
	fields := make([]adminField, 0, len(meta.Fields))
	for _, f := range meta.Fields {
		sf := t.Field(f.GoIndex)
		if f.IsImage && sf.Type.Kind() != reflect.String &&
			!(sf.Type.Kind() == reflect.Ptr && sf.Type.Elem().Kind() == reflect.String) {
			panic(fmt.Sprintf("admin: campo %q tem kyrux:\"image\" mas não é string — o caminho salvo pelo upload precisa de um campo string/*string", f.Name))
		}
		widget, optional := detectWidget(sf.Type, f.IsHash, f.IsImage, f.FK != "")
		fields = append(fields, adminField{
			GoName:    f.Name,
			Column:    f.Column,
			Label:     humanize(f.Column),
			GoIndex:   f.GoIndex,
			Widget:    widget,
			Optional:  optional,
			IsPK:      f.IsPK,
			IsHash:    f.IsHash,
			IsEncrypt: f.IsEncrypt,
			IsAutoNow: f.IsAutoNow,
			IsImage:   f.IsImage,
			IsFK:      f.FK != "",
			FKTable:   f.FK,
			FKLabel:   f.FKLabel,
		})
	}
	return fields
}

// detectWidget escolhe o tipo de input HTML a partir do tipo Go do campo.
// Ponteiros são desembrulhados (Optional=true); hash sempre vira "password";
// image sempre vira "file" (upload); fk (kyrux:"fk:tabela") sempre vira
// "select", mesmo sem fklabel — evita que o admin aceite um id que não
// existe na tabela referenciada.
func detectWidget(t reflect.Type, isHash, isImage, isFK bool) (widget string, optional bool) {
	if isHash {
		return "password", false
	}
	if isImage {
		if t.Kind() == reflect.Ptr {
			return "file", true
		}
		return "file", false
	}
	if isFK {
		if t.Kind() == reflect.Ptr {
			return "select", true
		}
		return "select", false
	}
	if t.Kind() == reflect.Ptr {
		w, _ := detectWidget(t.Elem(), false, false, false)
		return w, true
	}
	switch t.Kind() {
	case reflect.Bool:
		return "checkbox", false
	case reflect.Float32, reflect.Float64:
		return "number-float", false
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "number", false
	case reflect.Struct:
		if t == timeType {
			return "datetime", false
		}
	}
	return "text", false
}

// defaultListCols é aplicado quando ListFields() não foi usado: todos os
// campos exceto hash (nunca exibido) e autonow (ruído — já aparece no form).
func defaultListCols(f adminField) bool { return !f.IsHash }

// resolveColumns converte nomes de campo Go em nomes de coluna, na ordem
// informada; sem names, aplica fallback(f) a cada campo na ordem do struct.
// Panic em nome desconhecido — erro de configuração do dev, falha no boot.
func resolveColumns(fields []adminField, names []string, fallback func(adminField) bool) []string {
	if len(names) == 0 {
		if fallback == nil {
			return nil
		}
		var cols []string
		for _, f := range fields {
			if fallback(f) {
				cols = append(cols, f.Column)
			}
		}
		return cols
	}
	byName := make(map[string]adminField, len(fields))
	for _, f := range fields {
		byName[f.GoName] = f
	}
	cols := make([]string, 0, len(names))
	for _, n := range names {
		f, ok := byName[n]
		if !ok {
			panic(fmt.Sprintf("admin: campo %q não existe no model", n))
		}
		cols = append(cols, f.Column)
	}
	return cols
}

// humanize converte snake_case em "Title Case" para rótulos de exibição.
func humanize(col string) string {
	parts := strings.Split(col, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

// Count retorna o número de models registrados — o bootstrap só monta
// /admin/ quando este número é maior que zero (e ADMIN_ENABLED=true).
func Count() int {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return len(registry)
}

// EnsureAllTables cria (CREATE TABLE IF NOT EXISTS) a tabela de cada model
// registrado em db, se ainda não existir. Uso exclusivo do bootstrap, sobre
// o SQLite de fallback criado quando o admin é habilitado sem nenhum banco
// configurado — nunca chame isto sobre uma conexão real gerenciada por
// migrations explícitas. Continua tentando os demais models mesmo se um
// falhar; os erros são agregados via errors.Join.
func EnsureAllTables(db *database.DB) error {
	var errs []error
	for _, rm := range modelsOrdered() {
		if err := rm.ensureTable(db); err != nil {
			errs = append(errs, fmt.Errorf("admin: model %q: %w", rm.Label, err))
		}
	}
	return errors.Join(errs...)
}

// modelsOrdered retorna os models na ordem de registro (para a navegação).
func modelsOrdered() []*registeredModel {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]*registeredModel, 0, len(order))
	for _, slug := range order {
		out = append(out, registry[slug])
	}
	return out
}

func modelBySlug(slug string) (*registeredModel, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	rm, ok := registry[slug]
	return rm, ok
}
