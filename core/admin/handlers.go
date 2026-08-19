package admin

import (
	"errors"
	"fmt"
	"kyrux/core/database"
	kyerrors "kyrux/core/errors"
	"kyrux/core/orm"
	"kyrux/core/router"
	"kyrux/core/security/auth"
	"kyrux/core/security/csrf"
	"kyrux/core/security/session"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
	flashSessionKey = "_admin_flash"
	userCtxKey      = "admin_user"

	// staffCheckSessionKey/staffCheckTTL: cache do resultado de IsStaff/
	// IsAdmin na própria sessão — evita o SELECT em auth.GetUser a cada
	// requisição. Troca revogação IMEDIATA por revogação em até
	// staffCheckTTL (5s): uma janela pequena e previsível, aceitável em
	// troca de eliminar a maior parte das idas ao banco numa sessão de
	// navegação normal (vários cliques em poucos segundos reusam o mesmo
	// cache). Só resultados POSITIVOS são cacheados — um usuário sem
	// IsStaff/IsAdmin nunca chega a ser cacheado, então continua pagando
	// o SELECT (e sendo barrado) em toda tentativa.
	staffCheckSessionKey = "_admin_staff_check"
)

// staffCheckTTL é var (não const) só para permitir que os testes encolham
// a janela temporariamente em vez de esperar 5s de verdade — nunca
// reatribuída fora de teste.
var staffCheckTTL = 5 * time.Second

// staffCheck é o que fica cacheado na sessão — só o suficiente pra
// reconstruir o que requireStaff/base precisam sem tocar o banco de novo.
type staffCheck struct {
	UserID    int64
	Username  string
	FirstName string
	LastName  string
	IsStaff   bool
	IsAdmin   bool
	CheckedAt time.Time
}

// navItem representa um model na navegação lateral.
type navItem struct{ Slug, Label string }

// baseData carrega os dados comuns a todas as páginas (header, sidebar, CSRF).
type baseData struct {
	AppName    string
	Version    string
	PageTitle  string
	BasePath   string
	ActiveSlug string
	CSSVer     string
	JSVer      string
	// Models é a lista completa (framework + apps), na ordem de registro —
	// usada pelo dashboard, que não precisa da separação por origem.
	Models []navItem
	// FrameworkModels/AppModels são a mesma lista de Models, particionada
	// pela origem do model (ver registeredModel.Framework) — usada pela
	// navegação lateral, que exibe as duas seções separadamente.
	FrameworkModels []navItem
	AppModels       []navItem
	User            string
	UserInitials    string
	CSRFField       string
	CSRFToken       string
	FlashError      string
	FlashSuccess    string
}

type columnView struct {
	Column     string
	Label      string
	SortURL    string
	SortActive bool
	SortDesc   bool
	IsPK       bool
}

type formField struct {
	Column   string
	Label    string
	Widget   string
	Value    string
	Checked  bool
	ReadOnly bool
	IsHash   bool
	IsImage  bool
	IsFK     bool
	Options  []fkOption
}

type loginPageData struct {
	baseData
	LoginValue string
}

type dashboardPageData struct {
	baseData
}

// bulkActionView é a opção de ação em lote exibida no <select> da listagem —
// "delete" (builtin) sempre vem primeiro, seguida das registradas via
// admin.BulkAction, na ordem de registro.
type bulkActionView struct {
	Name  string
	Label string
}

type listPageData struct {
	baseData
	Label         string
	Slug          string
	Columns       []columnView
	Rows          []adminRow
	Search        string
	Searchable    bool
	Filters       []filterView
	FiltersActive bool // ao menos um filtro de Filters tem valor atual — controla o link "Limpar"
	BulkActions   []bulkActionView
	Page          int
	HasPrev       bool
	HasNext       bool
	PrevURL       string
	NextURL       string
	NewURL        string
	ClearURL      string
	BulkURL       string
}

type formPageData struct {
	baseData
	Label     string
	IsEdit    bool
	Action    string
	CancelURL string
	Fields    []formField
	Error     string
	PK        string
}

// site agrega as dependências necessárias para servir o admin — resolvidas
// pelo bootstrap e passadas para Mount, nunca importadas diretamente (evita
// ciclo de import entre admin e bootstrap).
type site struct {
	dbm      *database.Manager
	store    *session.Store
	basePath string
	appName  string
	version  string
}

// Mount monta o site do admin em basePath (ex: "/admin/") sempre que houver
// um banco de dados disponível para autenticar (auth.User vive na conexão
// "default") — login e dashboard ficam acessíveis mesmo sem nenhum model
// registrado ainda (o dashboard mostra um estado vazio com instruções).
// Sem banco, o admin não pode ser protegido — o bootstrap não monta nada
// (fail-closed) e registra o motivo no log, em vez de expor rotas sem
// proteção possível.
//
// Chame após todos os apps terem registrado seus models via admin.Register,
// e depois que orm.LoadDatabases já tiver populado dbm.
func Mount(r *router.Router, dbm *database.Manager, store *session.Store, basePath, appName, version string) {
	basePath = normalizeBasePath(basePath)

	if dbm == nil || dbm.Use() == nil {
		log.Println("admin: ADMIN_ENABLED=true mas nenhum banco de dados disponível — admin NÃO montado (sem banco não há como autenticar)")
		return
	}

	s := &site{dbm: dbm, store: store, basePath: basePath, appName: appName, version: version}

	serveStatic(r, "GET "+basePath+"statics/admin.css", adminCSS, "text/css; charset=utf-8")
	serveStatic(r, "GET "+basePath+"statics/admin.js", adminJS, "application/javascript; charset=utf-8")

	r.Handle("GET "+basePath+"login/", s.handleLoginForm)
	r.Handle("POST "+basePath+"login/", s.handleLoginSubmit)

	guard := requireStaff(dbm, store, basePath)
	r.Handle("POST "+basePath+"logout/", guard(s.handleLogout))
	r.Handle("GET "+basePath, guard(s.handleDashboard))
	r.Handle("GET "+basePath+"historico/", guard(s.handleHistory))
	r.Handle("GET "+basePath+"<slug:str>/", guard(s.handleList))
	r.Handle("GET "+basePath+"<slug:str>/novo/", guard(s.handleNewForm))
	r.Handle("POST "+basePath+"<slug:str>/novo/", guard(s.handleCreate))
	r.Handle("POST "+basePath+"<slug:str>/lote/", guard(s.handleBulkAction))
	r.Handle("GET "+basePath+"<slug:str>/<pk:str>/", guard(s.handleEditForm))
	r.Handle("POST "+basePath+"<slug:str>/<pk:str>/", guard(s.handleUpdate))
	r.Handle("POST "+basePath+"<slug:str>/<pk:str>/excluir/", guard(s.handleDelete))

	log.Printf("admin: painel em %s (%d model(s) registrado(s))\n", basePath, Count())
}

func normalizeBasePath(basePath string) string {
	if basePath == "" {
		basePath = "/admin/"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	return basePath
}

// requireStaff exige sessão ativa cujo usuário tenha IsStaff ou IsAdmin.
// O resultado positivo fica cacheado na sessão por staffCheckTTL — dentro
// dessa janela, requisições seguintes reusam o cache em vez de repetir o
// SELECT em auth.GetUser. Isso significa que revogar IsStaff/IsAdmin leva
// até staffCheckTTL para ter efeito (não mais instantâneo) — ver o
// comentário da constante para o raciocínio da troca.
func requireStaff(dbm *database.Manager, store *session.Store, basePath string) router.MiddlewareFunc {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) {
			if !auth.IsDBEnabled() {
				kyerrors.Render(ctx.Writer, ctx.Request, http.StatusServiceUnavailable)
				return
			}
			db := dbm.Use()
			if db == nil {
				kyerrors.Render(ctx.Writer, ctx.Request, http.StatusServiceUnavailable)
				return
			}

			sess, hasSess := session.FromRequest(ctx.Request, store)
			if hasSess {
				if v, ok := sess.Get(staffCheckSessionKey); ok {
					if c, ok2 := v.(*staffCheck); ok2 && time.Since(c.CheckedAt) < staffCheckTTL {
						ctx.Set(userCtxKey, &auth.User{ID: c.UserID, Username: c.Username, FirstName: c.FirstName, LastName: c.LastName, IsStaff: c.IsStaff, IsAdmin: c.IsAdmin})
						next(ctx)
						return
					}
				}
			}

			user, err := auth.GetUser(db, store, ctx.Request)
			if err != nil || !(user.IsStaff || user.IsAdmin) {
				dest := basePath + "login/?next=" + url.QueryEscape(ctx.Request.URL.RequestURI())
				http.Redirect(ctx.Writer, ctx.Request, dest, http.StatusFound)
				return
			}
			if hasSess {
				sess.Set(staffCheckSessionKey, &staffCheck{
					UserID:    user.ID,
					Username:  user.Username,
					FirstName: user.FirstName,
					LastName:  user.LastName,
					IsStaff:   user.IsStaff,
					IsAdmin:   user.IsAdmin,
					CheckedAt: time.Now(),
				})
			}
			ctx.Set(userCtxKey, user)
			next(ctx)
		}
	}
}

// ctxUser devolve o *auth.User autenticado na requisição (setado por
// requireStaff), ou nil se por algum motivo não estiver presente.
func ctxUser(ctx *router.Context) *auth.User {
	if v, ok := ctx.Get(userCtxKey); ok {
		if u, ok2 := v.(*auth.User); ok2 {
			return u
		}
	}
	return nil
}

// modelVisibleTo diz se rm deve aparecer/ser acessível pro usuário —
// models SuperuserOnly exigem IsAdmin, não basta IsStaff.
func modelVisibleTo(rm *registeredModel, user *auth.User) bool {
	return !rm.SuperuserOnly || (user != nil && user.IsAdmin)
}

// modelBySlugFor resolve o model igual a modelBySlug, mas aplicando a
// restrição SuperuserOnly — pra quem não tem IsAdmin, um model restrito
// simplesmente não existe (ok=false), igual a um slug desconhecido.
func modelBySlugFor(ctx *router.Context, slug string) (*registeredModel, bool) {
	rm, ok := modelBySlug(slug)
	if !ok || !modelVisibleTo(rm, ctxUser(ctx)) {
		return nil, false
	}
	return rm, true
}

func (s *site) base(ctx *router.Context, activeSlug, pageTitle string) baseData {
	b := baseData{
		AppName:    s.appName,
		Version:    s.version,
		PageTitle:  pageTitle,
		BasePath:   s.basePath,
		ActiveSlug: activeSlug,
		CSSVer:     cssVer,
		JSVer:      jsVer,
		CSRFField:  csrf.FieldName(),
		CSRFToken:  csrf.TokenFor(ctx),
	}
	user := ctxUser(ctx)
	for _, rm := range modelsOrdered() {
		if !modelVisibleTo(rm, user) {
			continue
		}
		item := navItem{Slug: rm.Slug, Label: rm.Label}
		b.Models = append(b.Models, item)
		if rm.Framework {
			b.FrameworkModels = append(b.FrameworkModels, item)
		} else {
			b.AppModels = append(b.AppModels, item)
		}
	}
	if user != nil {
		b.User = user.Username
		if name := user.FullName(); name != "" {
			b.User = name
		}
		b.UserInitials = userInitials(user)
	}
	if sess, ok := session.FromRequest(ctx.Request, s.store); ok {
		b.FlashError, b.FlashSuccess = popFlash(sess)
	}
	return b
}

// userInitials devolve o texto do círculo de avatar no header: iniciais de
// nome+sobrenome, ou só a primeira letra de quem tiver apenas um dos dois —
// inclusive o superusuário sem nome cadastrado, que cai na primeira letra do
// Username. Indexação por rune (não por byte) evita cortar um caractere
// acentuado ao meio.
func userInitials(u *auth.User) string {
	first := strings.TrimSpace(u.FirstName)
	last := strings.TrimSpace(u.LastName)
	switch {
	case first != "" && last != "":
		return strings.ToUpper(string([]rune(first)[:1]) + string([]rune(last)[:1]))
	case first != "":
		return strings.ToUpper(string([]rune(first)[:1]))
	case last != "":
		return strings.ToUpper(string([]rune(last)[:1]))
	case u.Username != "":
		return strings.ToUpper(string([]rune(u.Username)[:1]))
	default:
		return "?"
	}
}

func setFlash(sess *session.Session, kind, msg string) {
	if sess == nil {
		return
	}
	sess.Set(flashSessionKey, kind+"|"+msg)
}

func popFlash(sess *session.Session) (errMsg, okMsg string) {
	v, ok := sess.Get(flashSessionKey)
	if !ok {
		return "", ""
	}
	sess.Delete(flashSessionKey)
	s, _ := v.(string)
	kind, msg, _ := strings.Cut(s, "|")
	if kind == "error" {
		return msg, ""
	}
	return "", msg
}

func flashOnSession(store *session.Store, r *http.Request, kind, msg string) {
	if sess, ok := session.FromRequest(r, store); ok {
		setFlash(sess, kind, msg)
	}
}

// ── login / logout ────────────────────────────────────────────────────────────

func (s *site) handleLoginForm(ctx *router.Context) {
	if db := s.dbm.Use(); db != nil {
		if user, err := auth.GetUser(db, s.store, ctx.Request); err == nil && (user.IsStaff || user.IsAdmin) {
			ctx.Redirect(s.basePath, http.StatusFound)
			return
		}
	}
	data := loginPageData{baseData: s.base(ctx, "", "Login")}
	renderPage(ctx.Writer, loginTpl, data)
}

func loginErrorMessage(err error) string {
	if errors.Is(err, auth.ErrTooManyAttempts) {
		return "Muitas tentativas. Aguarde um pouco antes de tentar novamente."
	}
	return "Usuário ou senha inválidos."
}

func (s *site) handleLoginSubmit(ctx *router.Context) {
	loginValue := ctx.Request.FormValue("login")
	password := ctx.Request.FormValue("password")

	db := s.dbm.Use()
	if db == nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusServiceUnavailable)
		return
	}

	sess, err := auth.Login(db, s.store, ctx.Writer, ctx.Request, loginValue, password)
	if err != nil {
		s.renderLoginError(ctx, loginValue, loginErrorMessage(err))
		return
	}

	v, _ := sess.Get("user_id")
	userID, _ := v.(int64)
	user, err := orm.FromDB[auth.User](db).Where("id = ?", userID).First()
	if err != nil || !(user.IsStaff || user.IsAdmin) {
		// Login válido não implica acesso ao admin — desfaz a sessão recém-criada.
		s.store.Delete(sess.ID)
		http.SetCookie(ctx.Writer, &http.Cookie{Name: session.CookieName(), Value: "", MaxAge: -1, Path: "/"})
		s.renderLoginError(ctx, loginValue, "Sua conta não tem permissão para acessar o admin.")
		return
	}

	next := auth.NextURL(ctx.Request, s.basePath)
	ctx.Redirect(next, http.StatusFound)
}

func (s *site) renderLoginError(ctx *router.Context, loginValue, msg string) {
	b := s.base(ctx, "", "Login")
	b.FlashError = msg
	renderPage(ctx.Writer, loginTpl, loginPageData{baseData: b, LoginValue: loginValue})
}

func (s *site) handleLogout(ctx *router.Context) {
	auth.Logout(s.store, ctx.Request, ctx.Writer)
	ctx.Redirect(s.basePath+"login/", http.StatusFound)
}

// ── dashboard ─────────────────────────────────────────────────────────────────

func (s *site) handleDashboard(ctx *router.Context) {
	renderPage(ctx.Writer, dashboardTpl, dashboardPageData{baseData: s.base(ctx, "", "Painel")})
}

// ── listagem ──────────────────────────────────────────────────────────────────

func buildURL(base string, params map[string]string) string {
	q := url.Values{}
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	if enc := q.Encode(); enc != "" {
		return base + "?" + enc
	}
	return base
}

func (s *site) handleList(ctx *router.Context) {
	rm, ok := modelBySlugFor(ctx, ctx.Param("slug"))
	if !ok {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusNotFound)
		return
	}
	db := s.dbm.Use(rm.Conn)
	if db == nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusServiceUnavailable)
		return
	}

	page := ctx.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	pageSize := ctx.QueryInt("page_size", defaultPageSize)
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	search := strings.TrimSpace(ctx.Query("q"))
	sortCol := ctx.Query("sort")
	dir := ctx.Query("dir")
	sortDesc := dir == "desc"
	// Nunca repassa "sort" do usuário sem validar contra as colunas reais do
	// model — mesmo que o builder já valide a forma do identificador.
	if sortCol != "" && !rm.hasColumn(sortCol) {
		sortCol = ""
	}

	filters := buildFilterViews(ctx, db, rm.filterFields)
	conds := filterConds(rm.filterFields, filters)
	filtersActive := len(filterURLParams(filters)) > 0

	rows, hasNext, err := rm.list(db, page, pageSize, search, sortCol, sortDesc, conds)
	if err != nil {
		http.Error(ctx.Writer, "admin: "+err.Error(), http.StatusInternalServerError)
		return
	}

	byCol := make(map[string]adminField, len(rm.Fields))
	for _, f := range rm.Fields {
		byCol[f.Column] = f
	}
	listPath := s.basePath + rm.Slug + "/"

	// linkParams é a base comum (busca + filtros ativos) de todo link que
	// precisa preservar o estado atual da listagem — ordenação e paginação
	// só adicionam sort/dir/page em cima disso.
	linkParams := func(extra map[string]string) map[string]string {
		params := map[string]string{"q": search}
		for k, v := range filterURLParams(filters) {
			params[k] = v
		}
		for k, v := range extra {
			params[k] = v
		}
		return params
	}

	columns := make([]columnView, 0, len(rm.listCols))
	for _, col := range rm.listCols {
		f := byCol[col]
		active := sortCol == col
		nextDir := "asc"
		if active && !sortDesc {
			nextDir = "desc"
		}
		columns = append(columns, columnView{
			Column:     col,
			Label:      f.Label,
			SortURL:    buildURL(listPath, linkParams(map[string]string{"sort": col, "dir": nextDir})),
			SortActive: active,
			SortDesc:   active && sortDesc,
			IsPK:       f.IsPK,
		})
	}

	bulkActions := make([]bulkActionView, 0, len(rm.bulkActions)+1)
	bulkActions = append(bulkActions, bulkActionView{Name: "delete", Label: "Excluir selecionados"})
	for _, ba := range rm.bulkActions {
		bulkActions = append(bulkActions, bulkActionView{Name: ba.Name, Label: ba.Label})
	}

	data := listPageData{
		baseData:      s.base(ctx, rm.Slug, rm.Label),
		Label:         rm.Label,
		Slug:          rm.Slug,
		Columns:       columns,
		Rows:          rows,
		Search:        search,
		Searchable:    len(rm.searchCols) > 0,
		Filters:       filters,
		FiltersActive: filtersActive,
		BulkActions:   bulkActions,
		Page:          page,
		HasPrev:       page > 1,
		HasNext:       hasNext,
		PrevURL:       buildURL(listPath, linkParams(map[string]string{"sort": sortCol, "dir": dir, "page": strconv.Itoa(page - 1)})),
		NextURL:       buildURL(listPath, linkParams(map[string]string{"sort": sortCol, "dir": dir, "page": strconv.Itoa(page + 1)})),
		NewURL:        listPath + "novo/",
		ClearURL:      listPath,
		BulkURL:       listPath + "lote/",
	}
	renderPage(ctx.Writer, listTpl, data)
}

// ── ação em lote ──────────────────────────────────────────────────────────────

func (s *site) handleBulkAction(ctx *router.Context) {
	rm, ok := modelBySlugFor(ctx, ctx.Param("slug"))
	if !ok {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusNotFound)
		return
	}
	db := s.dbm.Use(rm.Conn)
	if db == nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusServiceUnavailable)
		return
	}
	if err := ctx.Request.ParseForm(); err != nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusBadRequest)
		return
	}
	listPath := s.basePath + rm.Slug + "/"

	action := ctx.Request.PostForm.Get("action")
	rawIDs := ctx.Request.PostForm["ids"]
	if len(rawIDs) == 0 {
		flashOnSession(s.store, ctx.Request, "error", "Nenhum registro selecionado.")
		ctx.Redirect(listPath, http.StatusFound)
		return
	}

	pks := make([]any, 0, len(rawIDs))
	for _, raw := range rawIDs {
		pkArg, err := parsePKArg(raw, rm.pkKind)
		if err != nil {
			kyerrors.Render(ctx.Writer, ctx.Request, http.StatusBadRequest)
			return
		}
		pks = append(pks, pkArg)
	}

	var fn BulkActionFunc
	switch action {
	case "delete":
		fn = rm.deleteMany
	default:
		for _, ba := range rm.bulkActions {
			if ba.Name == action {
				fn = ba.Fn
				break
			}
		}
	}
	if fn == nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusBadRequest)
		return
	}

	if err := fn(db, pks); err != nil {
		flashOnSession(s.store, ctx.Request, "error", err.Error())
		ctx.Redirect(listPath, http.StatusFound)
		return
	}
	logBulkHistory(db, rm, actorFrom(ctx), action, rawIDs)
	flashOnSession(s.store, ctx.Request, "success", fmt.Sprintf("%d registro(s) processado(s).", len(pks)))
	ctx.Redirect(listPath, http.StatusFound)
}

// ── formulário (criar / editar) ────────────────────────────────────────────────

// formData monta o view-model do formulário. prefill, se não-nil, fornece os
// valores atuais dos campos (linha do banco na edição, ou o POST submetido
// quando a validação falha e o form precisa ser re-exibido com os dados
// digitados). Campos hash nunca são preenchidos a partir de prefill.
func (s *site) formData(ctx *router.Context, db *database.DB, rm *registeredModel, isEdit bool, errMsg string, prefill url.Values, pk string) formPageData {
	fields := make([]formField, 0, len(rm.Fields))
	for _, f := range rm.Fields {
		if f.IsPK {
			continue
		}
		if f.IsAutoNow && !isEdit {
			continue // autonow não existe ainda na criação — nada a mostrar
		}
		ff := formField{
			Column:   f.Column,
			Label:    f.Label,
			Widget:   f.Widget,
			IsHash:   f.IsHash,
			IsImage:  f.IsImage,
			IsFK:     f.IsFK,
			// nanoid: sempre gerado pelo sistema (ver core/orm/nanoid.go),
			// nunca digitável — inclusive na criação, diferente de autonow
			// (que nem aparece no form de criação, já que também não existe
			// ainda). O nanoid já existe conceitualmente antes do INSERT
			// (só o valor que ainda não foi sorteado), então continua
			// visível — só travado, mostrando "—" até ser gerado.
			ReadOnly: f.IsAutoNow || f.IsNanoID,
		}
		if prefill != nil && !f.IsHash {
			raw := prefill.Get(f.Column)
			ff.Value = raw
			ff.Checked = raw == "true" || raw == "on" || raw == "1"
		}
		if f.IsFK {
			opts, err := fetchFKOptions(db, f.FKTable, f.FKLabel)
			if err != nil {
				log.Printf("admin: %v\n", err)
			}
			// Valor atual (edição, ou re-exibição após erro de validação) que
			// não está entre as opções carregadas — mantém visível em vez de
			// desaparecer silenciosamente do <select> (registro órfão/excluído).
			if ff.Value != "" {
				found := false
				for _, o := range opts {
					if o.Value == ff.Value {
						found = true
						break
					}
				}
				if !found {
					opts = append([]fkOption{{Value: ff.Value, Label: ff.Value}}, opts...)
				}
			}
			ff.Options = opts
		}
		fields = append(fields, ff)
	}

	action := s.basePath + rm.Slug + "/novo/"
	if isEdit {
		action = s.basePath + rm.Slug + "/" + pk + "/"
	}
	return formPageData{
		baseData:  s.base(ctx, rm.Slug, rm.Label),
		Label:     rm.Label,
		IsEdit:    isEdit,
		Action:    action,
		CancelURL: s.basePath + rm.Slug + "/",
		Fields:    fields,
		Error:     errMsg,
		PK:        pk,
	}
}

func (s *site) handleNewForm(ctx *router.Context) {
	rm, ok := modelBySlugFor(ctx, ctx.Param("slug"))
	if !ok {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusNotFound)
		return
	}
	db := s.dbm.Use(rm.Conn)
	if db == nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusServiceUnavailable)
		return
	}
	renderPage(ctx.Writer, formTpl, s.formData(ctx, db, rm, false, "", nil, ""))
}

func (s *site) handleCreate(ctx *router.Context) {
	rm, ok := modelBySlugFor(ctx, ctx.Param("slug"))
	if !ok {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusNotFound)
		return
	}
	db := s.dbm.Use(rm.Conn)
	if db == nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusServiceUnavailable)
		return
	}
	if err := parseAdminForm(ctx.Request); err != nil {
		data := s.formData(ctx, db, rm, false, "dados de formulário inválidos", nil, "")
		renderPage(ctx.Writer, formTpl, data)
		return
	}
	if err := rm.create(db, ctx.Request, actorFrom(ctx)); err != nil {
		data := s.formData(ctx, db, rm, false, err.Error(), ctx.Request.PostForm, "")
		renderPage(ctx.Writer, formTpl, data)
		return
	}
	flashOnSession(s.store, ctx.Request, "success", rm.Label+" criado com sucesso.")
	ctx.Redirect(s.basePath+rm.Slug+"/", http.StatusFound)
}

func (s *site) handleEditForm(ctx *router.Context) {
	rm, ok := modelBySlugFor(ctx, ctx.Param("slug"))
	if !ok {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusNotFound)
		return
	}
	db := s.dbm.Use(rm.Conn)
	if db == nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusServiceUnavailable)
		return
	}
	pk := ctx.Param("pk")
	row, err := rm.get(db, pk)
	if err != nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusNotFound)
		return
	}
	prefill := url.Values{}
	for k, v := range row.Values {
		prefill.Set(k, v)
	}
	renderPage(ctx.Writer, formTpl, s.formData(ctx, db, rm, true, "", prefill, pk))
}

func (s *site) handleUpdate(ctx *router.Context) {
	rm, ok := modelBySlugFor(ctx, ctx.Param("slug"))
	if !ok {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusNotFound)
		return
	}
	db := s.dbm.Use(rm.Conn)
	if db == nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusServiceUnavailable)
		return
	}
	pk := ctx.Param("pk")
	if err := parseAdminForm(ctx.Request); err != nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusBadRequest)
		return
	}
	if err := rm.update(db, pk, ctx.Request, actorFrom(ctx)); err != nil {
		data := s.formData(ctx, db, rm, true, err.Error(), ctx.Request.PostForm, pk)
		renderPage(ctx.Writer, formTpl, data)
		return
	}
	flashOnSession(s.store, ctx.Request, "success", rm.Label+" atualizado com sucesso.")
	ctx.Redirect(s.basePath+rm.Slug+"/", http.StatusFound)
}

func (s *site) handleDelete(ctx *router.Context) {
	rm, ok := modelBySlugFor(ctx, ctx.Param("slug"))
	if !ok {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusNotFound)
		return
	}
	db := s.dbm.Use(rm.Conn)
	if db == nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusServiceUnavailable)
		return
	}
	pk := ctx.Param("pk")
	if err := rm.delete(db, pk, actorFrom(ctx)); err != nil {
		http.Error(ctx.Writer, "admin: "+err.Error(), http.StatusInternalServerError)
		return
	}
	flashOnSession(s.store, ctx.Request, "success", rm.Label+" excluído.")
	ctx.Redirect(s.basePath+rm.Slug+"/", http.StatusFound)
}

func (rm *registeredModel) hasColumn(col string) bool {
	for _, f := range rm.Fields {
		if f.Column == col {
			return true
		}
	}
	return false
}
