// Package soapclient é o adapter que expõe um client SOAP (core/soap) como
// um Module do Core (kyrux/core). Diferente da maioria dos outros
// adapters, não há conexão persistente nenhuma (SOAP é HTTP request/
// response pontual, como REST) — Configure só monta o *soap.Client; a I/O
// real acontece a cada Call().
//
// Custo adicional zero: core/soap usa só encoding/xml e net/http, ambos
// stdlib — por isso, ao contrário dos adapters de banco/fila/API pesada,
// core.API.SOAPClient tem sugar direto em kyrux/core (ver core/namespaces.go).
package soapclient

import (
	"context"
	"fmt"

	"kyrux/core/soap"
)

// Adapter implementa registry.Module para um client SOAP nomeado.
type Adapter struct {
	name     string
	endpoint string
	client   *soap.Client
}

// New cria um adapter de client SOAP. name identifica esta configuração
// entre outras do mesmo tipo.
func New(name, endpoint string) *Adapter {
	return &Adapter{name: name, endpoint: endpoint}
}

func (a *Adapter) Name() string { return "api.soap.client." + a.name }

func (a *Adapter) Init(ctx context.Context) error {
	if a.endpoint == "" {
		return fmt.Errorf("soapclient: endpoint vazio para %q", a.name)
	}
	return nil
}

// Configure monta o *soap.Client — não há I/O aqui; um endpoint inválido
// ou fora do ar só se manifesta na primeira chamada (soap.Client.Call).
func (a *Adapter) Configure(ctx context.Context) error {
	a.client = soap.NewClient(a.endpoint, nil)
	return nil
}

func (a *Adapter) Start(ctx context.Context) error { return nil }

// Shutdown não faz nada — cada Call é uma requisição HTTP independente.
func (a *Adapter) Shutdown(ctx context.Context) error { return nil }

// Value devolve o *soap.Client já pronto.
func (a *Adapter) Value() *soap.Client { return a.client }
