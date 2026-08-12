// Package views contém o conteúdo e a lógica dos arquivos servidos pelo app
// "meta": robots.txt, sitemap.xml e security.txt (RFC 9116). routes.go só
// mapeia caminho → handler; qualquer alteração de conteúdo é feita aqui.
package views

import (
	"kyrux/core/bootstrap"
	"kyrux/core/router"
	"net/http"
	"strings"
)

// siteURL devolve fw.Settings.App.URL (configurado via APP_URL no .env),
// sem barra final. Vazia se APP_URL não foi definida — nesse caso os
// campos que dependem de URL absoluta (Sitemap, Canonical) são omitidos em
// vez de escrever um domínio errado.
func siteURL(fw *bootstrap.Framework) string {
	return fw.Settings.App.URL
}

// RobotsView serve /robots.txt. Libera indexação total por padrão — ajuste
// aqui se alguma área do site precisar de Disallow.
func RobotsView(fw *bootstrap.Framework) router.HandlerFunc {
	return func(ctx *router.Context) {
		var b strings.Builder
		b.WriteString("User-agent: *\nAllow: /\n")
		if u := siteURL(fw); u != "" {
			b.WriteString("\nSitemap: " + u + "/sitemap.xml\n")
		}
		ctx.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		ctx.Writer.Write([]byte(b.String()))
	}
}

// sitemapPaths lista as páginas públicas do site pra sitemap.xml. O
// framework não descobre isso sozinho a partir das rotas registradas — nem
// toda rota GET é uma página que faz sentido indexar (endpoints JSON, rotas
// com parâmetro, etc.) — edite esta lista conforme as páginas reais do seu
// projeto.
var sitemapPaths = []string{
	"/",
}

// SitemapView serve /sitemap.xml a partir de sitemapPaths.
func SitemapView(fw *bootstrap.Framework) router.HandlerFunc {
	return func(ctx *router.Context) {
		base := siteURL(fw)
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
		b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
		for _, p := range sitemapPaths {
			b.WriteString("  <url><loc>" + base + p + "</loc></url>\n")
		}
		b.WriteString("</urlset>\n")
		ctx.Writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
		ctx.Writer.Write([]byte(b.String()))
	}
}

// securityContact e securityExpires são placeholders — troque pelo contato
// real de segurança do projeto e revise Expires todo ano (RFC 9116: depois
// dessa data, scanners voltam a reportar o arquivo como ausente/inválido
// mesmo ele existindo).
const (
	securityContact = "mailto:security@example.com"
	securityExpires = "2027-01-01T00:00:00.000Z"
)

// SecurityTxtView serve /.well-known/security.txt — local canônico segundo
// a RFC 9116.
func SecurityTxtView(fw *bootstrap.Framework) router.HandlerFunc {
	return func(ctx *router.Context) {
		var b strings.Builder
		b.WriteString("Contact: " + securityContact + "\n")
		b.WriteString("Expires: " + securityExpires + "\n")
		if u := siteURL(fw); u != "" {
			b.WriteString("Canonical: " + u + "/.well-known/security.txt\n")
		}
		ctx.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		ctx.Writer.Write([]byte(b.String()))
	}
}

// SecurityTxtRedirectView serve /security.txt (raiz) — alguns scanners
// antigos ainda checam esse local em vez do /.well-known/ canônico; em vez
// de duplicar o conteúdo, redireciona.
func SecurityTxtRedirectView(fw *bootstrap.Framework) router.HandlerFunc {
	return func(ctx *router.Context) {
		ctx.Redirect("/.well-known/security.txt", http.StatusMovedPermanently)
	}
}
