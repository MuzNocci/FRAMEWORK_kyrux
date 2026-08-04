package core

import (
	"kyrux/core/adapters/restapi"
	"kyrux/core/adapters/sqlpostgres"
	"kyrux/core/cache"
	"kyrux/core/container"
	"kyrux/core/database"
	"kyrux/core/router"
)

// CacheNamespace agrupa os backends de cache disponíveis como módulos do Core.
type CacheNamespace struct{ core *Core }

// Memory ativa um cache em memória (core/cache, modo New()) como módulo do
// Core — exige o import (mesmo que só com _) de kyrux/core/adapters/cachememory
// em algum lugar da aplicação (hot-plug: ver core/adapters/cachememory).
// Custo adicional zero: core/cache já é dependência obrigatória do
// framework, usada por bootstrap.Init independente do Core.
func (n CacheNamespace) Memory() (*cache.Cache, error) {
	return Use[*cache.Cache](n.core, "cache.memory", "cache.memory")
}

// DatabaseNamespace agrupa os bancos de dados disponíveis como módulos do Core.
type DatabaseNamespace struct {
	core *Core
	SQL  SQLNamespace
}

// Get busca uma conexão SQL previamente ativada por name (o mesmo name
// passado a SQL.Postgres, por exemplo) — ok=false se nenhuma conexão com
// esse nome foi ativada ainda.
func (n DatabaseNamespace) Get(name string) (*database.DB, bool) {
	return container.Resolve[*database.DB](n.core.Container, "database.sql."+name)
}

// SQLNamespace agrupa os bancos relacionais disponíveis como módulos do Core.
type SQLNamespace struct{ core *Core }

// Postgres ativa uma conexão Postgres nomeada (core/database, o mesmo
// Manager usado por bootstrap.Init) como módulo do Core. name identifica
// esta conexão para buscas futuras (core.Database.Get(name)) — permite
// múltiplas conexões Postgres (ou outros bancos SQL) simultâneas, cada uma
// sob seu próprio nome, sem qualquer limitação de uso conjunto. Requer que
// o driver Postgres (ex: github.com/lib/pq) tenha sido importado (_) pela
// própria aplicação — como qualquer driver database/sql do Kyrux, o Core
// nunca importa drivers de banco.
func (n SQLNamespace) Postgres(name, dsn string) (*database.DB, error) {
	mod := sqlpostgres.New(name, dsn)
	return UseModule[*database.DB](n.core, mod, "database.sql."+name)
}

// APINamespace agrupa os protocolos de API disponíveis como módulos do Core.
type APINamespace struct{ core *Core }

// REST ativa uma API REST nomeada (por endereço) usando o mesmo motor de
// rotas do Kyrux (core/router — o mesmo usado por bootstrap.Init, aqui com
// seu próprio *http.Server independente). Devolve o *router.Router pronto
// para registrar rotas (router.Handle(pattern, handler)) — o servidor só
// escuta de verdade quando Core.Run() é chamado.
func (n APINamespace) REST(addr string) (*router.Router, error) {
	mod := restapi.New(addr)
	return UseModule[*router.Router](n.core, mod, "api.rest."+addr)
}
