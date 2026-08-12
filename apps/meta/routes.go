// Package meta registra robots.txt, sitemap.xml e security.txt — só o
// mapeamento de caminho pra handler. Conteúdo e lógica ficam em
// apps/meta/views/views.go.
package meta

import (
	"kyrux/apps/meta/views"
	"kyrux/core/bootstrap"
	"kyrux/core/router"
)

func init() {
	bootstrap.RegisterApp("meta", Register)
}

func Register(r *router.Router, fw *bootstrap.Framework) {
	router.Include(r, []router.URLPattern{
		router.Path("GET", "/robots.txt", views.RobotsView(fw), "robots_txt"),
		router.Path("GET", "/sitemap.xml", views.SitemapView(fw), "sitemap_xml"),
		router.Path("GET", "/.well-known/security.txt", views.SecurityTxtView(fw), "security_txt"),
		router.Path("GET", "/security.txt", views.SecurityTxtRedirectView(fw), "security_txt_redirect"),
	})
}
