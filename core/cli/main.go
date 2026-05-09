package cli

import (
	"fmt"
	"kyrux/core/database"
	dbmigrate "kyrux/core/database/migrate"
	"kyrux/core/environment"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const installedFile = "core/apps/installed.go"

func Run(args []string) {
	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}

	command := args[0]

	switch command {
	case "startapp", "removeapp":
		if len(args) < 2 {
			printUsage()
			os.Exit(1)
		}
		appName := strings.ToLower(args[1])
		switch command {
		case "startapp":
			if err := startApp(appName); err != nil {
				fmt.Fprintf(os.Stderr, "erro: %v\n", err)
				os.Exit(1)
			}
		case "removeapp":
			if err := removeApp(appName); err != nil {
				fmt.Fprintf(os.Stderr, "erro: %v\n", err)
				os.Exit(1)
			}
		}
	case "migrate":
		if err := runMigrate(); err != nil {
			fmt.Fprintf(os.Stderr, "erro: %v\n", err)
			os.Exit(1)
		}
	case "makemigrations":
		if err := runMakeMigrations(); err != nil {
			fmt.Fprintf(os.Stderr, "erro: %v\n", err)
			os.Exit(1)
		}
	case "removemigration":
		if len(args) < 2 {
			printUsage()
			os.Exit(1)
		}
		migNum := args[1]
		removeAll := len(args) > 2 && args[2] == "all"
		if err := runRemoveMigration(migNum, removeAll); err != nil {
			fmt.Fprintf(os.Stderr, "erro: %v\n", err)
			os.Exit(1)
		}
	case "createsuperuser":
		if err := runCreateSuperuser(); err != nil {
			fmt.Fprintf(os.Stderr, "erro: %v\n", err)
			os.Exit(1)
		}
	case "createuser":
		if err := runCreateUser(); err != nil {
			fmt.Fprintf(os.Stderr, "erro: %v\n", err)
			os.Exit(1)
		}
	case "benchmark":
		if err := runBenchmark(); err != nil {
			fmt.Fprintf(os.Stderr, "erro: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("uso: go run main.go <comando> [args]")
	fmt.Println()
	fmt.Println("comandos:")
	fmt.Println("  startapp  <nome>              cria um novo app")
	fmt.Println("  removeapp <nome>              remove um app existente")
	fmt.Println("  migrate                       aplica migrações pendentes em database/migrations/")
	fmt.Println("  makemigrations                gera migration SQL a partir dos models em apps/*/models/")
	fmt.Println("  removemigration <num> [all]   remove migration do disco (ou disco+banco com 'all')")
	fmt.Println("  createsuperuser               cria um superusuário (is_admin + is_staff)")
	fmt.Println("  createuser                    cria um usuário comum")
	fmt.Println("  benchmark                     roda todos os testes de performance e salva o resultado em benchmark/")
}

// ── migrate ───────────────────────────────────────────────────────────────────

func runMigrate() error {
	_ = environment.Load(".env")

	if environment.GetOr("DB_ENABLED", "false") != "true" {
		return fmt.Errorf("DB_ENABLED=false — banco de dados não configurado")
	}

	driver := environment.GetOr("DB_DRIVER", "postgres")
	dsn := environment.Get("DB_DSN")
	if dsn == "" {
		return fmt.Errorf("DB_DSN não configurado no .env")
	}

	db, err := database.Open(driver, dsn)
	if err != nil {
		return fmt.Errorf("conectar ao banco: %w", err)
	}
	defer db.Close()

	fmt.Println("Aplicando migrações...")
	return dbmigrate.Run(db, "database/migrations")
}

// ── startapp ──────────────────────────────────────────────────────────────────

func startApp(name string) error {
	base := filepath.Join("apps", name)

	if _, err := os.Stat(base); err == nil {
		return fmt.Errorf("app '%s' já existe em %s", name, base)
	}

	dirs := []string{
		filepath.Join(base, "statics", "styles"),
		filepath.Join(base, "templates"),
		filepath.Join(base, "views"),
		filepath.Join(base, "models"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("criar diretório %s: %w", dir, err)
		}
	}

	files := []struct {
		path    string
		content string
	}{
		{filepath.Join(base, "routes.go"), routesTpl},
		{filepath.Join(base, "views", "views.go"), viewsTpl},
		{filepath.Join(base, "models", "models.go"), modelsTpl},
		{filepath.Join(base, "templates", "exemplo.html"), templateTpl},
		{filepath.Join(base, "statics", "styles", "exemplo.css"), cssTpl},
	}

	data := struct{ Name string }{Name: name}

	for _, f := range files {
		if err := writeTemplate(f.path, f.content, data); err != nil {
			return err
		}
	}

	if err := addToInstalled(name); err != nil {
		return fmt.Errorf("atualizar %s: %w", installedFile, err)
	}

	fmt.Printf("app '%s' criado em %s\n", name, base)
	return nil
}

// ── removeapp ─────────────────────────────────────────────────────────────────

func removeApp(name string) error {
	base := filepath.Join("apps", name)

	if _, err := os.Stat(base); os.IsNotExist(err) {
		return fmt.Errorf("app '%s' não encontrado em %s", name, base)
	}

	fmt.Printf("remover o app '%s' em %s? essa ação é irreversível. [s/N] ", name, base)

	var input string
	fmt.Scanln(&input)

	if strings.ToLower(strings.TrimSpace(input)) != "s" {
		fmt.Println("operação cancelada.")
		return nil
	}

	if err := os.RemoveAll(base); err != nil {
		return fmt.Errorf("remover %s: %w", base, err)
	}

	if err := removeFromInstalled(name); err != nil {
		return fmt.Errorf("atualizar %s: %w", installedFile, err)
	}

	fmt.Printf("app '%s' removido.\n", name)
	return nil
}

// ── installed.go ──────────────────────────────────────────────────────────────

// parseInstalledApps lê os apps registrados no installed.go a partir das linhas de import.
func parseInstalledApps() []string {
	content, err := os.ReadFile(installedFile)
	if err != nil {
		return nil
	}
	const prefix = `_ "kyrux/apps/`
	var apps []string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			name := strings.TrimSuffix(strings.TrimPrefix(line, prefix), `"`)
			if name != "" {
				apps = append(apps, name)
			}
		}
	}
	return apps
}

func addToInstalled(name string) error {
	return writeInstalledFile(append(parseInstalledApps(), name))
}

func removeFromInstalled(name string) error {
	current := parseInstalledApps()
	filtered := current[:0]
	for _, a := range current {
		if a != name {
			filtered = append(filtered, a)
		}
	}
	return writeInstalledFile(filtered)
}

// writeInstalledFile regenera core/apps/installed.go com a lista de apps fornecida.
// Com zero apps, escreve apenas o cabeçalho do pacote.
func writeInstalledFile(apps []string) error {
	var sb strings.Builder
	sb.WriteString("package apps\n")

	if len(apps) > 0 {
		sb.WriteString("\nimport (\n")
		sb.WriteString("\t\"kyrux/core/settings\"\n")
		sb.WriteString("\n")
		for _, a := range apps {
			fmt.Fprintf(&sb, "\t_ \"kyrux/apps/%s\"\n", a)
		}
		sb.WriteString(")\n")
		sb.WriteString("\nfunc init() {\n")
		sb.WriteString("\tsettings.InstalledApps = []string{\n")
		for _, a := range apps {
			fmt.Fprintf(&sb, "\t\t\"%s\",\n", a)
		}
		sb.WriteString("\t}\n")
		sb.WriteString("}\n")
	}

	return os.WriteFile(installedFile, []byte(sb.String()), 0600)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func writeTemplate(path, content string, data any) error {
	tpl, err := template.New("").Funcs(template.FuncMap{"title": title}).Parse(content)
	if err != nil {
		return fmt.Errorf("parse template %s: %w", path, err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("criar arquivo %s: %w", path, err)
	}
	defer f.Close()

	return tpl.Execute(f, data)
}

// ── templates dos arquivos gerados ────────────────────────────────────────────

var routesTpl = `package {{.Name}}

import (
	"kyrux/apps/{{.Name}}/views"
	"kyrux/core/bootstrap"
	"kyrux/core/router"
)

func init() {
	bootstrap.RegisterApp("{{.Name}}", Register)
}

func Register(r *router.Router, fw *bootstrap.Framework) {
	router.Include(r, []router.URLPattern{
		router.Path("GET", "/{{.Name}}/", views.ExemploView(fw), "exemplo_home"),
	})
}
`

var viewsTpl = `package views

import (
	"kyrux/core/bootstrap"
	"kyrux/core/render"
	"kyrux/core/router"
)

func ExemploView(fw *bootstrap.Framework) router.HandlerFunc {
	return func(ctx *router.Context) {
		// Lógica de negócios aqui (exemplo)

		// Renderiza o template com o contexto
		// O renderizador irá procurar o template "exemplo.html" dentro da pasta "{{.Name}}/templates/"
		context := map[string]any{
			"example": "example",
		}
		render.For("{{.Name}}").Render(ctx, "exemplo.html", context)
	}
}
`

var modelsTpl = `package models
`

var cssTpl = `*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--go-blue:#00ACD7;--go-blue-light:#5DC9E2;--go-blue-dark:#00758D;--bg:#0D1117;--surface:#161B22;--border:#1E2A38;--text:#E6EDF3;--muted:#8B949E}
html,body{height:100%}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:var(--bg);color:var(--text);display:flex;flex-direction:column;overflow:auto}
header{border-bottom:1px solid var(--border);padding:.9rem 2.5rem;display:flex;align-items:center;gap:.75rem;flex-shrink:0}
.logo-img{height:24px;width:auto;display:block}
header .version{margin-left:auto;font-size:.7rem;color:var(--muted);background:var(--border);padding:.2rem .55rem;border-radius:999px}
main{flex:1;display:flex;flex-direction:column;align-items:center;justify-content:center;padding:2.5rem 2rem;text-align:center;gap:1.1rem}
.badge{display:inline-flex;align-items:center;gap:.4rem;font-size:.75rem;color:var(--go-blue-light);background:rgba(0,172,215,.1);border:1px solid rgba(0,172,215,.2);padding:.3rem .8rem;border-radius:999px;margin-bottom:.25rem}
.dot{width:6px;height:6px;border-radius:50%;background:var(--go-blue-light);animation:pulse 2s ease-in-out infinite}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.35}}
h1{font-size:clamp(1.4rem,3.5vw,2.2rem);font-weight:800;letter-spacing:-.03em;color:var(--text)}
.subtitle{font-size:clamp(.82rem,1.4vw,.95rem);color:var(--muted);max-width:420px;line-height:1.7}
.cards{display:flex;gap:1rem;margin-top:.5rem;flex-wrap:wrap;justify-content:center}
.card{background:var(--surface);border:1px solid var(--border);border-radius:10px;padding:1.1rem 1.4rem;text-align:left;width:180px;transition:border-color .2s}
.card:hover{border-color:var(--go-blue-dark)}
.card-icon{font-size:1.3rem;margin-bottom:.5rem}
.card-title{font-size:.82rem;font-weight:700;color:var(--text);margin-bottom:.2rem}
.card-desc{font-size:.75rem;color:var(--muted);line-height:1.5}
.actions{display:flex;gap:.75rem;margin-top:.5rem;flex-wrap:wrap;justify-content:center}
.btn{display:inline-flex;align-items:center;gap:.35rem;padding:.5rem 1.2rem;border-radius:8px;font-size:.82rem;text-decoration:none;font-weight:600;transition:border-color .2s,color .2s,background .2s}
.btn-primary{background:var(--go-blue);color:#fff;border:1px solid var(--go-blue)}
.btn-primary:hover{background:var(--go-blue-dark);border-color:var(--go-blue-dark)}
.btn-ghost{background:transparent;border:1px solid var(--border);color:var(--muted)}
.btn-ghost:hover{border-color:var(--go-blue-dark);color:var(--text)}
footer{border-top:1px solid var(--border);padding:.75rem 2rem;text-align:center;font-size:.82rem;color:var(--muted);flex-shrink:0}
footer a{color:var(--go-blue);text-decoration:none}
footer a:hover{text-decoration:underline}
@media(max-width:640px){main{padding:2rem 1.25rem;gap:1rem}.cards{flex-direction:column;align-items:center}.card{width:100%;max-width:280px}}
@media(max-width:380px){.actions{flex-direction:column;align-items:center}}
`

var templateTpl = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{"{{"}} AppName {{"}}"}} | Página de exemplo — substitua este template para começar.</title>
<link rel="stylesheet" href="{{"{{"}} statics "{{.Name}}" "styles/exemplo.css" {{"}}"}}">
</head>
<body>
<header>
  <img src="/kyrux/statics/kyrux_wt.png" class="logo-img" alt="Kyrux">
  <span class="version">v{{"{{"}} Version {{"}}"}}</span>
</header>
<main>
  <span class="badge"><span class="dot"></span> Servidor operacional</span>
  <h1>Seu APP foi criado corretamente!</h1>
  <p class="subtitle">Esta é a página de exemplo de app no <strong>{{"{{"}} AppName {{"}}"}}</strong>. Substitua este template em <code>templates/exemplo.html</code> e comece a desenvolver.</p>

  <div class="cards">
    <div class="card">
      <div class="card-icon">&#128218;</div>
      <div class="card-title">Rotas</div>
      <div class="card-desc">Defina URLs em <code>routes.go</code> com <code>router.Path()</code>.</div>
    </div>
    <div class="card">
      <div class="card-icon">&#128065;</div>
      <div class="card-title">Views</div>
      <div class="card-desc">Lógica das páginas em <code>views/views.go</code> via <code>ctx</code>.</div>
    </div>
    <div class="card">
      <div class="card-icon">&#127912;</div>
      <div class="card-title">Templates</div>
      <div class="card-desc">HTML em <code>templates/</code> com o motor nativo do Go.</div>
    </div>
  </div>

  <div class="actions">
    <a href="/kyrux/debug/" class="btn btn-primary">&#9881; Debug</a>
    <a href="https://www.kyrux.com.br/docs/" target="_blank" class="btn btn-ghost">Documentação &#8599;</a>
  </div>
</main>
<footer>
  Kyrux Framework &mdash; construído com <a href="https://go.dev" target="_blank">Go</a>
  &nbsp;&middot;&nbsp;
  <a href="https://www.kyrux.com.br/docs/" target="_blank">Documentação</a>
  &nbsp;&middot;&nbsp;
  desenvolvido por <a href="https://www.nocciolli.com.br" target="_blank">M&uuml;ller Nocciolli</a>
</footer>
</body>
</html>
`
