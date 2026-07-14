package admin

import (
	"errors"
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
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
	flashSessionKey = "_admin_flash"
	userCtxKey      = "admin_user"
)

// navItem representa um model na navegação lateral.
type navItem struct{ Slug, Label string }

// baseData carrega os dados comuns a todas as páginas (header, sidebar, CSRF).
type baseData struct {
	AppName      string
	Version      string
	PageTitle    string
	BasePath     string
	ActiveSlug   string
	Models       []navItem
	User         string
	CSRFField    string
	CSRFToken    string
	FlashError   string
	FlashSuccess string
}

type columnView struct {
	Column     string
	Label      string
	SortURL    string
	SortActive bool
	SortDesc   bool
}

type formField struct {
	Column   string
	Label    string
	Widget   string
	Value    string
	Checked  bool
	ReadOnly bool
	IsHash   bool
}

type loginPageData struct {
	baseData
	LoginValue string
}

type dashboardPageData struct {
	baseData
}

type listPageData struct {
	baseData
	Label      string
	Slug       string
	Columns    []columnView
	Rows       []adminRow
	Search     string
	Searchable bool
	Page       int
	TotalPages int
	Total      int64
	HasPrev    bool
	HasNext    bool
	PrevURL    string
	NextURL    string
	NewURL     string
	ClearURL   string
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

// Mount monta o site do admin em basePath (ex: "/admin/") — só se houver ao
// menos um model registrado (Count() > 0) e um banco de dados disponível
// para autenticar (auth.User vive na conexão "default"). Sem banco, o admin
// não pode ser protegido — o bootstrap não monta nada (fail-closed) e
// registra o motivo no log, em vez de expor rotas sem proteção possível.
//
// Chame após todos os apps terem registrado seus models via admin.Register,
// e depois que orm.LoadDatabases já tiver populado dbm.
func Mount(r *router.Router, dbm *database.Manager, store *session.Store, basePath, appName, version string) {
	if Count() == 0 {
		return
	}
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
	r.Handle("GET "+basePath+"<slug:str>/", guard(s.handleList))
	r.Handle("GET "+basePath+"<slug:str>/novo/", guard(s.handleNewForm))
	r.Handle("POST "+basePath+"<slug:str>/novo/", guard(s.handleCreate))
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

// requireStaff exige sessão ativa cujo usuário tenha IsStaff ou IsAdmin —
// verificado a CADA requisição (não apenas no login), para que revogar o
// acesso de um usuário tenha efeito imediato.
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
			user, err := auth.GetUser(db, store, ctx.Request)
			if err != nil || !(user.IsStaff || user.IsAdmin) {
				dest := basePath + "login/?next=" + url.QueryEscape(ctx.Request.URL.RequestURI())
				http.Redirect(ctx.Writer, ctx.Request, dest, http.StatusFound)
				return
			}
			ctx.Set(userCtxKey, user)
			next(ctx)
		}
	}
}

func (s *site) base(ctx *router.Context, activeSlug, pageTitle string) baseData {
	b := baseData{
		AppName:    s.appName,
		Version:    s.version,
		PageTitle:  pageTitle,
		BasePath:   s.basePath,
		ActiveSlug: activeSlug,
		CSRFField:  csrf.FieldName(),
		CSRFToken:  csrf.TokenFor(ctx),
	}
	for _, rm := range modelsOrdered() {
		b.Models = append(b.Models, navItem{Slug: rm.Slug, Label: rm.Label})
	}
	if v, ok := ctx.Get(userCtxKey); ok {
		if u, ok2 := v.(*auth.User); ok2 {
			b.User = u.Username
		}
	}
	if sess, ok := session.FromRequest(ctx.Request, s.store); ok {
		b.FlashError, b.FlashSuccess = popFlash(sess)
	}
	return b
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
	rm, ok := modelBySlug(ctx.Param("slug"))
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

	rows, total, err := rm.list(db, page, pageSize, search, sortCol, sortDesc)
	if err != nil {
		http.Error(ctx.Writer, "admin: "+err.Error(), http.StatusInternalServerError)
		return
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages < 1 {
		totalPages = 1
	}

	byCol := make(map[string]adminField, len(rm.Fields))
	for _, f := range rm.Fields {
		byCol[f.Column] = f
	}
	listPath := s.basePath + rm.Slug + "/"
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
			SortURL:    buildURL(listPath, map[string]string{"q": search, "sort": col, "dir": nextDir}),
			SortActive: active,
			SortDesc:   active && sortDesc,
		})
	}

	data := listPageData{
		baseData:   s.base(ctx, rm.Slug, rm.Label),
		Label:      rm.Label,
		Slug:       rm.Slug,
		Columns:    columns,
		Rows:       rows,
		Search:     search,
		Searchable: len(rm.searchCols) > 0,
		Page:       page,
		TotalPages: totalPages,
		Total:      total,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
		PrevURL:    buildURL(listPath, map[string]string{"q": search, "sort": sortCol, "dir": dir, "page": strconv.Itoa(page - 1)}),
		NextURL:    buildURL(listPath, map[string]string{"q": search, "sort": sortCol, "dir": dir, "page": strconv.Itoa(page + 1)}),
		NewURL:     listPath + "novo/",
		ClearURL:   listPath,
	}
	renderPage(ctx.Writer, listTpl, data)
}

// ── formulário (criar / editar) ────────────────────────────────────────────────

// formData monta o view-model do formulário. prefill, se não-nil, fornece os
// valores atuais dos campos (linha do banco na edição, ou o POST submetido
// quando a validação falha e o form precisa ser re-exibido com os dados
// digitados). Campos hash nunca são preenchidos a partir de prefill.
func (s *site) formData(ctx *router.Context, rm *registeredModel, isEdit bool, errMsg string, prefill url.Values, pk string) formPageData {
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
			ReadOnly: f.IsAutoNow,
		}
		if prefill != nil && !f.IsHash {
			raw := prefill.Get(f.Column)
			ff.Value = raw
			ff.Checked = raw == "true" || raw == "on" || raw == "1"
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
	rm, ok := modelBySlug(ctx.Param("slug"))
	if !ok {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusNotFound)
		return
	}
	renderPage(ctx.Writer, formTpl, s.formData(ctx, rm, false, "", nil, ""))
}

func (s *site) handleCreate(ctx *router.Context) {
	rm, ok := modelBySlug(ctx.Param("slug"))
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
		data := s.formData(ctx, rm, false, "dados de formulário inválidos", nil, "")
		renderPage(ctx.Writer, formTpl, data)
		return
	}
	if err := rm.create(db, ctx.Request.PostForm); err != nil {
		data := s.formData(ctx, rm, false, err.Error(), ctx.Request.PostForm, "")
		renderPage(ctx.Writer, formTpl, data)
		return
	}
	flashOnSession(s.store, ctx.Request, "success", rm.Label+" criado com sucesso.")
	ctx.Redirect(s.basePath+rm.Slug+"/", http.StatusFound)
}

func (s *site) handleEditForm(ctx *router.Context) {
	rm, ok := modelBySlug(ctx.Param("slug"))
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
	renderPage(ctx.Writer, formTpl, s.formData(ctx, rm, true, "", prefill, pk))
}

func (s *site) handleUpdate(ctx *router.Context) {
	rm, ok := modelBySlug(ctx.Param("slug"))
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
	if err := ctx.Request.ParseForm(); err != nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusBadRequest)
		return
	}
	if err := rm.update(db, pk, ctx.Request.PostForm); err != nil {
		data := s.formData(ctx, rm, true, err.Error(), ctx.Request.PostForm, pk)
		renderPage(ctx.Writer, formTpl, data)
		return
	}
	flashOnSession(s.store, ctx.Request, "success", rm.Label+" atualizado com sucesso.")
	ctx.Redirect(s.basePath+rm.Slug+"/", http.StatusFound)
}

func (s *site) handleDelete(ctx *router.Context) {
	rm, ok := modelBySlug(ctx.Param("slug"))
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
	if err := rm.delete(db, pk); err != nil {
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
