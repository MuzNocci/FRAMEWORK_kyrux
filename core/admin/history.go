package admin

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"time"

	kyerrors "kyrux/core/errors"

	"kyrux/core/database"
	"kyrux/core/orm"
	"kyrux/core/router"
)

// HistoryLog é o registro de auditoria de uma ação feita através do admin
// (Create/Update/Delete individuais e ações em lote, embutida ou via
// BulkAction) — quem fez, quando, em qual model/registro. Changes carrega
// os valores novos submetidos em create/update (JSON); vazio em
// delete/ações em lote, que não guardam um diff de valores — o custo de
// reconsultar cada linha pra montar antes/depois não compensa na v1 deste
// recurso.
//
// Não é registrada via admin.Register (é somente leitura — sem rotas de
// criar/editar/excluir para ela mesma); sua tabela é criada automaticamente
// junto com a de auth.User (EnsureAllTables no fallback SQLite;
// makemigrations escaneia core/admin como escaneia core/security/auth).
type HistoryLog struct {
	ID        int64     `kyrux:"pk"`
	ModelSlug string    `kyrux:"column:model_slug,size:100"`
	Label     string    `kyrux:"size:150"`
	RecordPK  string    `kyrux:"column:record_pk,size:100"`
	Action    string    `kyrux:"size:40"`
	Changes   string    `kyrux:"column:changes"`
	UserID    int64     `kyrux:"column:user_id"`
	Username  string    `kyrux:"size:150"`
	CreatedAt time.Time `kyrux:"column:created_at,autonow_add"`
}

// historyActor identifica quem fez a ação, pra preencher HistoryLog.UserID/
// Username — resolvido em handlers.go a partir de ctxUser(ctx).
type historyActor struct {
	UserID   int64
	Username string
}

func actorFrom(ctx *router.Context) historyActor {
	if u := ctxUser(ctx); u != nil {
		return historyActor{UserID: u.ID, Username: u.Username}
	}
	return historyActor{}
}

// fieldValuesMap extrai coluna→valor de v (struct do model) para todos os
// campos exceto a PK — usado por rm.create para registrar os valores do
// registro recém-criado no histórico (o PK só existe depois do INSERT).
func fieldValuesMap(v reflect.Value, fields []adminField) map[string]any {
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		if f.IsPK {
			continue
		}
		out[f.Column] = v.Field(f.GoIndex).Interface()
	}
	return out
}

// redactedChangesJSON serializa changes para a coluna Changes, substituindo
// o valor de campos kyrux:"hash"/kyrux:"encrypt" por "***" — o histórico é
// uma cópia secundária dos dados que fica indefinidamente; não deve nunca
// carregar hash nem o plaintext de um campo cifrado, mesma proteção que
// esses campos já têm no resto do admin (formatDisplayValue/formatInputValue).
func redactedChangesJSON(fields []adminField, changes map[string]any) string {
	if len(changes) == 0 {
		return ""
	}
	byCol := make(map[string]adminField, len(fields))
	for _, f := range fields {
		byCol[f.Column] = f
	}
	out := make(map[string]string, len(changes))
	for col, v := range changes {
		if f, ok := byCol[col]; ok && (f.IsHash || f.IsEncrypt) {
			out[col] = "***"
			continue
		}
		out[col] = fmt.Sprint(v)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}

// logHistory grava uma entrada de auditoria para uma ação individual
// (create/update/delete). Nunca retorna erro pro chamador: a operação
// principal já aconteceu quando isto é chamado — perder uma entrada de
// histórico é preferível a reportar falha num CRUD que na verdade deu certo.
func logHistory(db *database.DB, rm *registeredModel, actor historyActor, action, recordPK string, changes map[string]any) {
	entry := &HistoryLog{
		ModelSlug: rm.Slug,
		Label:     rm.Label,
		RecordPK:  recordPK,
		Action:    action,
		Changes:   redactedChangesJSON(rm.Fields, changes),
		UserID:    actor.UserID,
		Username:  actor.Username,
	}
	if err := orm.Create(db, entry); err != nil {
		log.Printf("admin: histórico: falha ao gravar entrada (%s %s#%s): %v\n", action, rm.Slug, recordPK, err)
	}
}

// logBulkHistory grava uma entrada de auditoria por registro afetado por
// uma ação em lote (embutida "delete" ou via BulkAction) — sem diff de
// valores, mesmo motivo de logHistory em delete. CreateAll faz um único
// INSERT multi-VALUES em vez de N.
func logBulkHistory(db *database.DB, rm *registeredModel, actor historyActor, action string, pks []string) {
	entries := make([]*HistoryLog, len(pks))
	for i, pk := range pks {
		entries[i] = &HistoryLog{
			ModelSlug: rm.Slug,
			Label:     rm.Label,
			RecordPK:  pk,
			Action:    action,
			UserID:    actor.UserID,
			Username:  actor.Username,
		}
	}
	if err := orm.CreateAll(db, entries); err != nil {
		log.Printf("admin: histórico: falha ao gravar entradas em lote (%s %s): %v\n", action, rm.Slug, err)
	}
}

// ── página de visualização (somente leitura) ────────────────────────────────

// historyEntryView é HistoryLog já formatado para o template.
type historyEntryView struct {
	Model     string
	RecordPK  string
	Action    string
	Changes   string // "campo: valor, campo2: valor2" — Changes (JSON) já decodificado
	Username  string
	CreatedAt string
}

type historyPageData struct {
	baseData
	Entries     []historyEntryView
	ModelFilter string
	Models      []navItem // slug/label de todos os models registrados, para o <select> de filtro
	Page        int
	HasPrev     bool
	HasNext     bool
	PrevURL     string
	NextURL     string
	ClearURL    string
}

// formatChanges decodifica o JSON de HistoryLog.Changes num texto curto
// "campo: valor, campo2: valor2" — ordenado por nome de campo pra saída
// estável (map não tem ordem própria).
func formatChanges(raw string) string {
	if raw == "" {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil || len(m) == 0 {
		return ""
	}
	cols := make([]string, 0, len(m))
	for col := range m {
		cols = append(cols, col)
	}
	sort.Strings(cols)
	var sb []byte
	for i, col := range cols {
		if i > 0 {
			sb = append(sb, ", "...)
		}
		sb = append(sb, col...)
		sb = append(sb, ": "...)
		sb = append(sb, m[col]...)
	}
	return string(sb)
}

// handleHistory serve a listagem somente-leitura de HistoryLog. connName
// resolve pela conexão do model filtrado (cada entrada mora na mesma
// conexão do registro que ela descreve — ver logHistory/logBulkHistory);
// sem filtro de model, usa "default", cobrindo o caso comum de um único
// banco. Para ver o histórico de um model noutra conexão nomeada
// (admin.Conn("nome")), filtre por ele explicitamente.
func (s *site) handleHistory(ctx *router.Context) {
	modelFilter := ctx.Query("model")
	connName := "default"
	if modelFilter != "" {
		if rm, ok := modelBySlugFor(ctx, modelFilter); ok {
			connName = rm.Conn
		} else {
			modelFilter = ""
		}
	}
	db := s.dbm.Use(connName)
	if db == nil {
		kyerrors.Render(ctx.Writer, ctx.Request, http.StatusServiceUnavailable)
		return
	}

	page := ctx.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}

	q := orm.FromDB[HistoryLog](db)
	if modelFilter != "" {
		q = q.WhereEq("model_slug", modelFilter)
	}
	q = q.OrderBy("id DESC")

	p, err := q.PaginateNoCount(page, defaultPageSize)
	if err != nil {
		http.Error(ctx.Writer, "admin: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Entradas de um model SuperuserOnly nunca aparecem pra quem não tem
	// IsAdmin — mesma restrição que já vale pro model em si (o histórico
	// não pode ser um vazamento lateral do que a listagem já esconde).
	user := ctxUser(ctx)
	hidden := make(map[string]bool)
	models := make([]navItem, 0, Count())
	for _, rm := range modelsOrdered() {
		if !modelVisibleTo(rm, user) {
			hidden[rm.Slug] = true
			continue
		}
		models = append(models, navItem{Slug: rm.Slug, Label: rm.Label})
	}

	entries := make([]historyEntryView, 0, len(p.Items))
	for _, e := range p.Items {
		if hidden[e.ModelSlug] {
			continue
		}
		label := e.Label
		if label == "" {
			label = e.ModelSlug
		}
		entries = append(entries, historyEntryView{
			Model:     label,
			RecordPK:  e.RecordPK,
			Action:    e.Action,
			Changes:   formatChanges(e.Changes),
			Username:  e.Username,
			CreatedAt: e.CreatedAt.Format("2006-01-02 15:04"),
		})
	}

	historyPath := s.basePath + "historico/"
	data := historyPageData{
		baseData:    s.base(ctx, "historico", "Histórico"),
		Entries:     entries,
		ModelFilter: modelFilter,
		Models:      models,
		Page:        page,
		HasPrev:     page > 1,
		HasNext:     p.HasNext,
		PrevURL:     buildURL(historyPath, map[string]string{"model": modelFilter, "page": strconv.Itoa(page - 1)}),
		NextURL:     buildURL(historyPath, map[string]string{"model": modelFilter, "page": strconv.Itoa(page + 1)}),
		ClearURL:    historyPath,
	}
	renderPage(ctx.Writer, historyTpl, data)
}
