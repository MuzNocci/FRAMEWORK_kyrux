// Package oauth2 é o adapter que expõe um fluxo OAuth2 Authorization Code
// (golang.org/x/oauth2 — o client oficial) como um Module do Core
// (kyrux/core).
//
// Diferente da maioria dos outros adapters, este NÃO abre conexão nenhuma
// (OAuth2 é config + chamadas HTTP pontuais, não um client de longa
// duração) — Configure só monta o *oauth2.Config; toda a I/O real
// (AuthCodeURL, Exchange, Client) acontece quando você chama esses métodos
// no valor devolvido.
//
// Genérico por propósito: qualquer provedor OAuth2 (Google, GitHub,
// GitLab, um Identity Provider interno...) funciona, desde que você informe
// authURL/tokenURL — o pacote golang.org/x/oauth2/endpoints já traz esses
// dois valores prontos para os provedores mais comuns, sem custo extra
// (é só uma tabela de constantes, no mesmo módulo golang.org/x/oauth2).
package oauth2

import (
	"context"
	"fmt"

	xoauth2 "golang.org/x/oauth2"
)

// Adapter implementa registry.Module para uma configuração OAuth2 nomeada.
type Adapter struct {
	name         string
	clientID     string
	clientSecret string
	authURL      string
	tokenURL     string
	redirectURL  string
	scopes       []string
	cfg          *xoauth2.Config
}

// New cria (mas ainda não valida — isso acontece em Init) um adapter
// OAuth2. name identifica esta configuração entre outras do mesmo tipo
// (ex: "google", "github", dois ambientes do mesmo provedor).
func New(name, clientID, clientSecret, authURL, tokenURL, redirectURL string, scopes ...string) *Adapter {
	return &Adapter{
		name:         name,
		clientID:     clientID,
		clientSecret: clientSecret,
		authURL:      authURL,
		tokenURL:     tokenURL,
		redirectURL:  redirectURL,
		scopes:       scopes,
	}
}

func (a *Adapter) Name() string { return "auth.oauth2." + a.name }

func (a *Adapter) Init(ctx context.Context) error {
	if a.clientID == "" || a.clientSecret == "" {
		return fmt.Errorf("oauth2: client id/secret vazios para %q", a.name)
	}
	if a.authURL == "" || a.tokenURL == "" {
		return fmt.Errorf("oauth2: authURL/tokenURL vazios para %q", a.name)
	}
	return nil
}

// Configure monta o *oauth2.Config — não há I/O aqui; a validação de
// credenciais só acontece de fato no primeiro Exchange (troca do código de
// autorização pelo token), fora do controle deste adapter.
func (a *Adapter) Configure(ctx context.Context) error {
	a.cfg = &xoauth2.Config{
		ClientID:     a.clientID,
		ClientSecret: a.clientSecret,
		Endpoint:     xoauth2.Endpoint{AuthURL: a.authURL, TokenURL: a.tokenURL},
		RedirectURL:  a.redirectURL,
		Scopes:       a.scopes,
	}
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

// Shutdown não faz nada — não há conexão nem recurso para liberar.
func (a *Adapter) Shutdown(ctx context.Context) error { return nil }

// Value devolve o *oauth2.Config pronto:
//
//	cfg.AuthCodeURL(state)                 // URL de redirecionamento pro provedor
//	token, err := cfg.Exchange(ctx, code)   // troca o código pelo token
//	client := cfg.Client(ctx, token)        // *http.Client autenticado
func (a *Adapter) Value() *xoauth2.Config { return a.cfg }
