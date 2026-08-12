package core

import (
	"kyrux/core/adapters/oauth2"
	"kyrux/core/adapters/restapi"
	"kyrux/core/adapters/smtp"
	"kyrux/core/adapters/soapclient"
	"kyrux/core/adapters/sqlpostgres"
	"kyrux/core/cache"
	"kyrux/core/container"
	"kyrux/core/database"
	"kyrux/core/router"
	"kyrux/core/soap"

	xoauth2 "golang.org/x/oauth2"
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

// SOAPClient ativa um client SOAP nomeado (SOAP 1.1, encoding/xml puro —
// custo adicional zero) — devolve o *soap.Client, cujo Call(ctx,
// soapAction, request, response) monta o envelope, envia via HTTP POST e
// decodifica a resposta (ou um *soap.Fault, se o servidor responder com
// erro). Para expor seus próprios endpoints SOAP (servidor), não há sugar
// aqui — monte um soap.NewServer(), registre operações com Handle, e
// pendure no router de core.API.REST via api.HandlePrefix("/soap", server)
// (mesmo padrão do core/adapters/prometheus).
func (n APINamespace) SOAPClient(name, endpoint string) (*soap.Client, error) {
	mod := soapclient.New(name, endpoint)
	return UseModule[*soap.Client](n.core, mod, "api.soap.client."+name)
}

// AuthNamespace agrupa os mecanismos de autenticação disponíveis como
// módulos do Core.
type AuthNamespace struct{ core *Core }

// OAuth2 ativa uma configuração OAuth2 nomeada (fluxo Authorization Code) —
// funciona com qualquer provedor (Google, GitHub, um Identity Provider
// interno...), desde que authURL/tokenURL sejam informados. Devolve o
// *oauth2.Config pronto: cfg.AuthCodeURL(state) gera a URL de
// redirecionamento, cfg.Exchange(ctx, code) troca o código pelo token.
func (n AuthNamespace) OAuth2(name, clientID, clientSecret, authURL, tokenURL, redirectURL string, scopes ...string) (*xoauth2.Config, error) {
	mod := oauth2.New(name, clientID, clientSecret, authURL, tokenURL, redirectURL, scopes...)
	return UseModule[*xoauth2.Config](n.core, mod, "auth.oauth2."+name)
}

// MailNamespace agrupa os provedores de envio de e-mail disponíveis como
// módulos do Core. SendGrid e SES (core/adapters/sendgrid,
// core/adapters/ses) não ganham sugar aqui — dependem de SDKs de
// terceiros que a maioria dos projetos Kyrux nunca vai usar; ative-os
// direto com core.UseModule (ver core/adapters/sendgrid e core/adapters/ses).
type MailNamespace struct{ core *Core }

// SMTP ativa um client de envio de e-mail via SMTP nomeado — custo
// adicional zero (net/smtp + mime/multipart são stdlib, nenhuma
// dependência de terceiros). Devolve o *smtp.Client, que implementa
// mail.Sender (Send(ctx, mail.Message) error) — o mesmo formato de mensagem
// aceito por SendGrid e SES, então trocar de provedor não exige reescrever
// quem monta o e-mail. Porta "465" usa TLS implícito (SMTPS) automaticamente;
// useTLS controla STARTTLS nas demais portas (ex: 587).
func (n MailNamespace) SMTP(name, host, port, username, password string, useTLS bool) (*smtp.Client, error) {
	mod := smtp.New(name, host, port, username, password, useTLS)
	return UseModule[*smtp.Client](n.core, mod, "mail.smtp."+name)
}
