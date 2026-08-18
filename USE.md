# KYRUX — MANUAL DE USO

Framework web em Go baseado em SSR, EventBus e Realtime invisível.
Criado por Müller Nocciolli · [framework.kyrux.com.br/docs](https://framework.kyrux.com.br/docs/)

---

## ÍNDICE

1. [Início Rápido](#1-início-rápido)
2. [Estrutura do Projeto](#2-estrutura-do-projeto)
3. [Configuração (.env)](#3-configuração-env)
4. [CLI — Comandos](#4-cli--comandos)
5. [Rotas e URLs](#5-rotas-e-urls)
6. [Views e Context](#6-views-e-context)
7. [Templates](#7-templates)
8. [CSRF](#8-csrf)
9. [Middleware](#9-middleware)
10. [Banco de Dados](#10-banco-de-dados)
11. [Migrations](#11-migrations)
12. [ORM](#12-orm)
13. [Cache](#13-cache)
14. [Sessões](#14-sessões)
15. [Autenticação](#15-autenticação)
16. [EventBus](#16-eventbus)
17. [Realtime (DOM sem JS)](#17-realtime-dom-sem-js)
18. [Páginas de Erro](#18-páginas-de-erro)
19. [Debug Dashboard](#19-debug-dashboard)
20. [Admin (Painel de Administração)](#20-admin-painel-de-administração)
21. [Fluxo do Sistema](#21-fluxo-do-sistema)
22. [Performance](#22-performance)
23. [MongoDB (NoSQL)](#23-mongodb-nosql)
24. [Redis como banco (NoSQL)](#24-redis-como-banco-nosql)
25. [Cassandra (NoSQL)](#25-cassandra-nosql)
26. [Elasticsearch (NoSQL)](#26-elasticsearch-nosql)
27. [DynamoDB (NoSQL)](#27-dynamodb-nosql)
28. [Core (fundação modular) — experimental](#28-core-fundação-modular--experimental)
29. [Mail (fw.Mail)](#29-mail-fwmail)
30. [Captcha (core/security/captcha)](#30-captcha-coresecuritycaptcha)

---

## 1. Início Rápido

### Pré-requisitos

```bash
go 1.26.2+   # versão mínima exigida pelo go.mod do framework
```

### Instalar o Air (hot reload — apenas uma vez)

```bash
go install github.com/air-verse/air@latest
```

### Iniciar o servidor

```bash
go run main.go
```

O modo é detectado automaticamente pelo `.env`:

| `APP_ENV`     | Comportamento                                      |
|---------------|----------------------------------------------------|
| `development` | Inicia com Air (live reload), debug e pprof ativos |
| `production`  | Inicia direto, otimizado, sem debug                |

### Criar o primeiro app

```bash
go run main.go startapp blog
```

Isso cria a estrutura `apps/blog/`, registra o app em `InstalledApps` e já o importa automaticamente.

---

## 2. Estrutura do Projeto

```
projeto/
├── main.go
├── .env
├── .env.example
├── apps/
│   └── blog/
│       ├── routes.go          ← definição de URLs
│       ├── views/
│       │   └── views.go       ← handlers das páginas
│       ├── models/
│       │   └── models.go      ← structs e queries
│       ├── templates/
│       │   ├── base.html      ← template base
│       │   └── exemplo.html   ← página de exemplo gerada pelo CLI
│       └── statics/
│           ├── css/
│           └── js/
├── core/                      ← núcleo do framework (não editar)
└── statics/                   ← arquivos estáticos globais
```

---

## 3. Configuração (.env)

```env
# ── Ambiente ──────────────────────────────────────────────────────
# development → debug, hotreload e pprof ativados automaticamente
# production  → modo otimizado, debug desligado
APP_ENV=development

# ── Servidor ──────────────────────────────────────────────────────
SERVER_HOST=0.0.0.0     # bind — veja a nota abaixo antes de abrir no navegador
SERVER_PORT=8000
SERVER_WORKERS=4        # omitir para usar todos os CPUs disponíveis

# ── Hosts permitidos ──────────────────────────────────────────────
# Ignorado em development. Obrigatório em production.
ALLOWED_HOSTS=meusite.com.br,www.meusite.com.br

# ── Banco de dados ────────────────────────────────────────────────
# Cada bloco DB_NAME inicia um novo banco. O primeiro é o padrão.
DB_NAME=principal
DB_ENABLED=true
DB_DRIVER=postgres
DB_DSN=postgres://user:password@localhost:5432/meudb?sslmode=disable

# DB_NAME=analytics
# DB_ENABLED=true
# DB_DRIVER=postgres
# DB_DSN=postgres://user:password@localhost:5432/analytics?sslmode=disable

# ── Cache ─────────────────────────────────────────────────────────
CACHE_ENABLED=false
CACHE_DRIVER=memory     # memory | redis
CACHE_ADDR=localhost:6379
# CACHE_PASSWORD=       # AUTH do redis (requirepass) — só com CACHE_DRIVER=redis

# ── Queue (fila de tarefas em background) ─────────────────────────
QUEUE_ENABLED=false
QUEUE_DRIVER=memory     # memory | redis
QUEUE_ADDR=localhost:6379
# QUEUE_PASSWORD=       # AUTH do redis (requirepass) — só com QUEUE_DRIVER=redis
QUEUE_WORKERS=4

# ── Admin (painel opt-in por model — precisa de admin.Register[T] no código)
ADMIN_ENABLED=false
ADMIN_PATH=/admin/

# Opcional — cria o superusuário inicial no boot, se ainda não existir
# ninguém com esse login (não redefine senha de conta já existente)
# ADMIN_SUPERUSER_USERNAME=admin
# ADMIN_SUPERUSER_PASSWORD=troque-esta-senha-provisoria

# ── Mail (fw.Mail — ver seção 29) ──────────────────────────────────
MAIL_ENABLED=false
MAIL_HOST=smtp.exemplo.com.br
MAIL_PORT=587           # 465 usa TLS implícito (SMTPS) automaticamente; qualquer outra porta usa STARTTLS
MAIL_USER=no-reply@exemplo.com.br
MAIL_PASSWORD=troque-esta-senha
# Com QUEUE_ENABLED=true, fw.Mail.Send() enfileira e devolve na hora —
# não bloqueia o caller esperando a sessão SMTP (ver core/mail.Queued).

# ── Segurança ─────────────────────────────────────────────────────
SECRET_KEY=sua-chave-secreta-forte-aqui
SESSION_TTL=3600        # duração da sessão em segundos

# Pepper aplicado antes do hash Argon2id — nunca armazenar no banco
# Gere com: openssl rand -base64 32
PASSWORD_PEPPER=seu-pepper-forte-aqui

# Chave AES-256-GCM para campos kyrux:"encrypt" — nunca armazenar no banco
# Gere com: openssl rand -base64 32
FIELD_ENCRYPTION_KEY=sua-chave-de-criptografia-forte-aqui

# Content-Security-Policy padrão (opcional) — vazia usa o DefaultCSP
# embutido (ver seção 9). Sobrescreve só a política global; exceções por
# rota continuam via secmiddleware.CSPOverride no código.
# CSP_POLICY=default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:

# ── Runtime (opcional) ────────────────────────────────────────────
# Percentual de GC do Go. Padrão: 100. Reduzir (ex: 75) diminui heap, aumenta frequência de GC.
# RUNTIME_GOGC=75
```

> Em produção `SECRET_KEY`, `PASSWORD_PEPPER`, `FIELD_ENCRYPTION_KEY` e `ALLOWED_HOSTS` são **obrigatórios** — o servidor recusa iniciar sem eles.

### `SERVER_HOST=0.0.0.0` é o endereço de *bind*, não de acesso

`0.0.0.0` diz ao servidor para escutar em **todas** as interfaces de rede da
máquina — não é um endereço real para o qual um navegador consiga se
conectar. Digitar `http://0.0.0.0:8000` na barra de endereços resulta em
`ERR_CONNECTION_REFUSED` (ou comportamento inconsistente, dependendo do SO).

| Endereço | O que é | Uso |
|---|---|---|
| `0.0.0.0` | Curinga "todas as interfaces" | Só para `SERVER_HOST` (bind) |
| `127.0.0.1` | Loopback — esta máquina, sempre existente e roteável | Acessar localmente |
| `localhost` | Nome que o SO resolve para `127.0.0.1` (ou `::1`) | Acessar localmente |

Para acessar o servidor rodando localmente, use sempre `http://127.0.0.1:PORTA`
ou `http://localhost:PORTA` — nunca `0.0.0.0`, mesmo com `SERVER_HOST=0.0.0.0`
configurado (o bind continua em todas as interfaces; só o endereço de acesso
muda). O Kyrux já traduz isso sozinho nas mensagens que imprime (console,
welcome page, debug dashboard) — todas mostram `127.0.0.1`, nunca `0.0.0.0`.

---

## 4. CLI — Comandos

### Criar um novo app

```bash
go run main.go startapp <nome>
```

Gera automaticamente:
- `apps/<nome>/routes.go` — com rota `GET /` e view `ExemploView`
- `apps/<nome>/views/views.go` — com `func ExemploView`
- `apps/<nome>/models/models.go`
- `apps/<nome>/templates/exemplo.html` — página de boas-vindas estilizada
- `apps/<nome>/statics/styles/exemplo.css` — estilos da página de exemplo
- Pasta `statics/styles/`
- Registra em `InstalledApps` e adiciona o import automaticamente

### Remover um app

```bash
go run main.go removeapp <nome>
```

Remove a pasta e desfaz o registro. Pede confirmação antes.

### Gerar migrations automáticas

```bash
go run main.go makemigrations
```

Lê todos os structs com `kyrux:"pk"` em `apps/*/models/*.go` e em `core/security/auth/*.go`, detecta tabelas ainda não migradas e gera um arquivo `database/migrations/NNNN_auto.sql`. **Revise o arquivo antes de aplicar.**

### Aplicar migrations

```bash
go run main.go migrate
```

Aplica todos os arquivos `.sql` em `database/migrations/` que ainda não foram executados. Registra cada migration na tabela `kyrux_migrations` — idempotente. Requer `DB_ENABLED=true`.

### Criar superusuário

```bash
go run main.go createsuperuser
```

Cria interativamente um usuário com `is_admin=true` e `is_staff=true`. Requer `DB_ENABLED=true`.

O campo marcado com `kyrux:"login"` no model `auth.User` é sempre obrigatório. O outro identificador (username ou e-mail) é solicitado como opcional — se informado, também é verificado quanto à unicidade.

### Criar usuário comum

```bash
go run main.go createuser
```

Cria interativamente um usuário comum (pergunta se é staff). Requer `DB_ENABLED=true`.

Segue o mesmo comportamento do `createsuperuser` quanto ao campo de login obrigatório e identificador opcional.

### Remover migration

```bash
go run main.go removemigration 0003        # remove apenas do disco
go run main.go removemigration 0003 all    # remove do disco + da tabela kyrux_migrations
```

Remove a migration pelo número (prefixo `NNNN`). A variante `all` requer `DB_ENABLED=true`.
Útil para corrigir uma migration gerada com erro antes de aplicá-la em produção.

### Rodar os benchmarks do framework

```bash
go run main.go benchmark
```

Roda as camadas de teste de performance descritas na [seção 22](#22-performance)
em sequência (microbenchmark, framework sem TCP, regressão e throughput real) e
salva a saída completa em `benchmark/benchmark_AAAA-MM-DD_HH-MM-SS.txt`, junto
com a versão do Go e o modelo de CPU detectado. Atalho para não digitar os
comandos `go test` de cada camada manualmente.

---

## 5. Rotas e URLs

### Definindo rotas

Em `apps/<nome>/routes.go`:

```go
var URLPatterns = []router.URLPattern{
    router.Path("GET",    "/",                  views.HomeView,    "home"),
    router.Path("POST",   "/contato/",          views.ContatoView, "contato"),
    router.Path("GET",    "/posts/<slug:str>/",  views.PostView,    "post_detalhe"),
    router.Path("GET",    "/users/<id:int>/",    views.UserView,    "user_detalhe"),
    router.Path("DELETE", "/users/<id:int>/",    views.DeleteUser,  "user_delete"),
}
```

### Parâmetros de path — tipos suportados

| Sintaxe           | Exemplo de URL        | Descrição                        |
|-------------------|-----------------------|----------------------------------|
| `<nome:str>`      | `/posts/meu-post/`    | Qualquer texto sem barra         |
| `<nome:string>`   | `/posts/meu-post/`    | Idêntico a `str`                 |
| `<id:int>`        | `/users/42/`          | Número inteiro                   |
| `<preco:float>`   | `/produto/9.99/`      | Número decimal                   |
| `<slug:slug>`     | `/artigos/go-lang/`   | Texto sem barra                  |
| `<uid:uuid>`      | `/item/550e8400.../`  | UUID                             |
| `<resto:path>`    | `/arquivos/a/b/c`     | Múltiplos segmentos (com barras) |

### Barra final — comportamento

O Kyrux aceita a rota **com e sem barra** automaticamente:

```
/contato   →  serve a view
/contato/  →  serve a view (sem redirect)
```

Não é necessário registrar as duas variantes.

### Query string

Parâmetros de query (`?chave=valor`) funcionam em qualquer rota:

```
/busca?q=golang&page=2
/busca/?q=golang&page=2   ← ambas funcionam
```

Acesso na view via `ctx.Query()` (ver seção 6).

### Resolução de URLs nos templates

```html
<a href="{{ url "home" }}">Início</a>
<a href="{{ url "post_detalhe" }}">Post</a>
<form action="{{ url "contato" }}">...</form>
```

### Registrar rotas avançadas (sem o CLI)

```go
func Register(r *router.Router) {
    router.Include(r, URLPatterns)

    // Rota direta sem URLPattern:
    r.Handle("GET /ping", func(ctx *router.Context) {
        ctx.JSON(200, map[string]string{"status": "ok"})
    })
}
```

---

## 6. Views e Context

Uma view é uma função `func(ctx *router.Context)`.

```go
func PostView(ctx *router.Context) {
    slug := ctx.Param("slug")           // parâmetro de path
    page := ctx.QueryInt("page", 1)     // query string com fallback

    post := models.GetBySlug(slug)
    if post == nil {
        ctx.Error(404)
        return
    }

    render.For("blog").Render(ctx, "post.html", map[string]any{
        "post": post,
        "page": page,
    })
}
```

> ⚠️ **Nunca use `ctx` em goroutines que vivem além do handler** — o Context
> volta a um pool quando o handler retorna e é reutilizado por outra requisição.
> Para trabalho assíncrono, faça uma cópia:
>
> ```go
> cp := ctx.Copy() // Params, dados e Request para leitura; Writer é nil
> go func() { processar(cp.Param("id")) }()
> ```

### Métodos do Context

#### Parâmetros de path

```go
ctx.Param("slug")             // string — retorna "" se ausente
ctx.ParamInt("id")            // (int, bool) — (0, false) se inválido
```

#### Query string

```go
ctx.Query("q")                // primeiro valor, "" se ausente
ctx.QueryDefault("order", "asc") // com fallback
ctx.QueryInt("page", 1)       // int com fallback
ctx.QueryAll("tag")           // []string — múltiplos valores do mesmo parâmetro
```

#### Respostas

```go
// Renderizar HTML com template
render.For("meuapp").Render(ctx, "index.html", map[string]any{
    "titulo": "Olá mundo",
})

// JSON
ctx.JSON(200, map[string]any{"id": 1, "nome": "Kyrux"})

// HTML inline
ctx.HTML(200, "<h1>Olá</h1>")

// Redirect
ctx.Redirect("/login/", http.StatusFound)       // 302
ctx.Redirect("/home/", http.StatusMovedPermanently) // 301

// Página de erro
ctx.Error(404)
ctx.Error(403)
ctx.Error(500)
```

#### Dados internos do contexto (entre middlewares)

```go
// Guardar
ctx.Set("usuario", usuario)

// Recuperar
v, ok := ctx.Get("usuario")
usuario := v.(*models.Usuario)
```

#### Acesso direto ao request e writer

```go
ctx.Request           // *http.Request
ctx.Request.Method    // "GET", "POST", etc.
ctx.Request.Header    // http.Header
ctx.Writer            // http.ResponseWriter
```

---

## 7. Templates

O Kyrux usa herança de templates no estilo Django. Os templates ficam em `apps/<nome>/templates/`.

### Convenção de variáveis

| Forma            | Origem          | Exemplo                    |
|------------------|-----------------|----------------------------|
| `{{ .titulo }}`  | Dados da view   | `map[string]any{"titulo": "..."}` |
| `{{ AppName }}`  | Framework       | Nome do app no `.env`      |
| `{{ Version }}`  | Framework       | Versão do app              |
| `{{ Env }}`      | Framework       | `development` / `production` |
| `{{ Addr }}`     | Framework       | `0.0.0.0:8000`             |
| `{{ GoVersion }}`| Framework       | `go1.22.3`                 |
| `{{ url "nome" }}`| Framework      | Resolve a URL pelo nome    |
| `{{ statics "path/arquivo.css" }}` / `{{ statics "app" "path/arquivo.css" }}` | Framework | Resolve URL de arquivo estático (global ou por app) |
| `{{ csrf_token }}`| Framework      | Input hidden de segurança  |

### Herança de templates

**`apps/blog/templates/base.html`:**
```html
<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <title>{% block "title" %}{{ AppName }}{% endblock "title" %}</title>
  <link rel="stylesheet" href="{{ statics "blog" "styles/style.css" }}">
</head>
<body>
  {% include "partials/navbar.html" %}

  <main>
    {% block "content" %}{% endblock "content" %}
  </main>

  {% block "scripts" %}{% endblock "scripts" %}
</body>
</html>
```

**`apps/blog/templates/post.html`:**
```html
{% extends "base.html" %}

{% block "title" %}{{ .post.Titulo }} — {{ AppName }}{% endblock "title" %}

{% block "content" %}
  <article>
    <h1>{{ .post.Titulo }}</h1>
    <p>{{ .post.Conteudo }}</p>
  </article>
{% endblock "content" %}

{% block "scripts" %}
  <script src="{{ statics "blog" "scripts/post.js" }}"></script>
{% endblock "scripts" %}
```

### Diretivas de template

| Diretiva                         | Descrição                         |
|----------------------------------|-----------------------------------|
| `{% extends "base.html" %}`      | Herda de outro template           |
| `{% block "nome" %}...{% endblock "nome" %}` | Define/sobrescreve um bloco |
| `{% include "partials/nav.html" %}` | Inclui um template parcial   |

### Renderizar fragmento (para Realtime)

```go
html, err := render.Partial("blog", "partials/lista.html", map[string]any{
    "posts": posts,
})
```

### Context Processors — variáveis globais nos templates

Um ContextProcessor é uma função que adiciona variáveis ao contexto de **todo** template de um app — sem precisar passá-las manualmente em cada view.

```go
import "kyrux/core/render"

// Processor global: disponível em TODOS os templates de TODOS os apps
render.AddDefaultProcessor(func(ctx *router.Context) map[string]any {
    return map[string]any{
        "site_nome": "Meu Site",
        "ano_atual": time.Now().Year(),
    }
})
```

Acesso no template (com ponto):
```html
<footer>{{ .site_nome }} — {{ .ano_atual }}</footer>
```

Processors são acumulados: cada chamada a `AddDefaultProcessor` adiciona um novo — não substitui os anteriores.

### Funções personalizadas nos templates

Registre funções Go para uso em todos os templates via `render.AddFunc`:

```go
import "kyrux/core/render"

// Registrar antes de qualquer render (ex: no init() do app ou no bootstrap)
render.AddFunc("formatarData", func(t time.Time) string {
    return t.Format("02/01/2006")
})

render.AddFunc("upper", strings.ToUpper)
```

Uso no template:
```html
<span>{{ .post.CriadoEm | formatarData }}</span>
<h1>{{ .titulo | upper }}</h1>
```

### Arquivos estáticos

Existem dois tipos de estáticos, ambos servidos automaticamente em `/statics/` pelo mesmo handler:

- **Por app**: ficam em `apps/<nome>/statics/`.
- **Globais**: ficam em `statics/` na raiz do projeto, compartilhados entre todos os apps.

Use a função `statics` no template para gerar a URL — todos os argumentos passados são concatenados (com `/`) para formar o caminho.

**Sintaxe:**
```html
{{ statics "caminho/arquivo.ext" }}                <!-- estático global -->
{{ statics "nome-do-app" "caminho/arquivo.ext" }}  <!-- estático do app -->
```

**Como a resolução funciona:** para cada requisição em `/statics/<caminho>`, o framework tenta, nessa ordem:
1. `statics/<caminho>` na raiz do projeto.
2. Se não encontrar, usa o primeiro segmento de `<caminho>` como nome de app e busca em `apps/<app>/statics/<resto>`.

Por isso, ao chamar `{{ statics "blog" "styles/base.css" }}`, a URL gerada inclui o nome do app (`/statics/blog/styles/base.css`) — isso é necessário para o fallback por app funcionar. Já `{{ statics "styles/base.css" }}` (sem nome de app) gera `/statics/styles/base.css`, resolvido direto na pasta raiz.

**Exemplos — estático por app:**
```html
<!-- CSS e JS -->
<link rel="stylesheet" href="{{ statics "blog" "styles/base.css" }}">
<script src="{{ statics "blog" "scripts/app.js" }}"></script>

<!-- Imagem fixa -->
<img src="{{ statics "blog" "images/logo.png" }}" alt="Logo">

<!-- Imagem com nome vindo do banco de dados -->
<img src="{{ statics "blog" "uploads/" .UserPhoto }}" alt="Foto">

<!-- Caminho completamente dinâmico -->
<img src="{{ statics "blog" .ImagePath }}" alt="Imagem">

<!-- Combinando prefixo, subpasta e variável -->
<img src="{{ statics "blog" "uploads/" .Category "/" .FileName }}" alt="Arquivo">
```

**URLs geradas:**
```
/statics/blog/styles/base.css
/statics/blog/uploads/avatar.jpg
```

**Estrutura de pastas por app:**
```
apps/
└── blog/
    └── statics/
        ├── styles/       ← CSS
        ├── scripts/      ← JavaScript
        └── images/       ← Imagens e outros assets
```

**Estáticos globais** (compartilhados entre todos os apps, resolvidos com prioridade sobre os estáticos por app):
```
statics/              ← raiz do projeto
├── styles/
├── scripts/
└── images/
```

**Exemplos — estático global:**
```html
<link rel="stylesheet" href="{{ statics "styles/global.css" }}">
<!-- gera /statics/styles/global.css, resolvido em statics/styles/global.css -->

<img src="{{ statics "images/logo.png" }}" alt="Logo">
```

Use estáticos globais para arquivos compartilhados entre apps (ex: reset CSS, framework JS, fontes, ícones do site); use estáticos por app para arquivos exclusivos de um app.

---

## 8. CSRF

O CSRF é validado automaticamente em `POST`, `PUT`, `PATCH` e `DELETE`.

### Em formulários HTML

```html
<form method="POST" action="{{ url "criar_post" }}">
  {{ csrf_token }}
  <input type="text" name="titulo" placeholder="Título">
  <button type="submit">Publicar</button>
</form>
```

O `{{ csrf_token }}` injeta automaticamente:
```html
<input type="hidden" name="kyrux_csrf_token" value="abc123...">
```

### Em requisições AJAX

O token enviado deve ser o valor **assinado** — o mesmo que `{{ csrf_token }}`
injeta no hidden input (o cookie `kyrux_csrf` guarda o valor bruto, que **não**
é aceito). Leia do input renderizado na página:

```javascript
const token = document.querySelector('input[name="kyrux_csrf_token"]').value;

fetch('/api/posts/', {
    method: 'POST',
    headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': token,
    },
    body: JSON.stringify({ titulo: 'Meu Post' }),
});
```

### Isentar rotas de API (Bearer token)

APIs autenticadas por JWT (`RequireAuth`) não usam cookies — isente o prefixo
da validação CSRF no `Register` do app:

```go
import "kyrux/core/security/csrf"

csrf.Exempt("/api/")
```

---

## 9. Middleware

### Middlewares globais (aplicados a todas as rotas)

Registrados no `bootstrap` — já ativos por padrão:

| Middleware          | Descrição                                                    |
|---------------------|---------------------------------------------------------------|
| `Recovery()`        | Captura panics — mostra debug page (dev) ou 500 (prod)        |
| `MaxBodySize(32MB)` | Limita o tamanho do body — defesa contra payloads gigantes    |
| `SecureHeaders`     | HSTS, X-Frame-Options, CSP (configurável — ver abaixo) — **apenas em production** (`APP_ENV=development` não ativa) |
| `AllowedHosts()`    | Bloqueia hosts não autorizados (ignorado em dev)              |
| `csrf.Middleware`   | Valida token CSRF em métodos não seguros                      |

### Middlewares opcionais

| Middleware | Uso | Descrição |
|---|---|---|
| `Compress` | `r.Use(compress.Compress)` | Compressão gzip das respostas |
| `CORS(origins)` | `r.Use(secmiddleware.CORS(...))` | Cabeçalhos CORS para as origens permitidas |
| `RequireAuth(a)` | por rota ou global | Exige Bearer token JWT — APIs stateless |
| `RequireLogin(store, url)` | por rota ou global | Exige sessão ativa — views SSR; redireciona para `url` se não autenticado |
| `RateLimit(max, janela)` | por rota (recomendado no login) | Máximo de `max` requisições por IP por janela; excedentes recebem 429 |
| `LocalhostOnly` | por rota (ex: dashboards internos) | 403 para qualquer IP que não seja loopback — é o que protege `/kyrux/debug/` |
| `MaxBodySize(n)` | por rota, além do global de 32MB | Limite específico menor (ou maior) para uma rota (ex: upload) |

```go
import (
    "kyrux/core/compress"
    secmiddleware "kyrux/core/security/middleware"
)

// Compressão gzip
r.Use(compress.Compress)

// CORS
r.Use(secmiddleware.CORS([]string{"https://meusite.com.br"}))

// Exigir sessão ativa em views SSR — redireciona para /login/ se não autenticado
r.Use(secmiddleware.RequireLogin(fw.Sessions, "/login/"))

// Exigir JWT em rotas de API
r.Use(secmiddleware.RequireAuth(fw.Auth))

// Rate limit por rota — essencial no login (Argon2 é caro por design):
router.Path("POST", "/login/",
    secmiddleware.RateLimit(10, time.Minute)(views.LoginPost(fw)), "login_post"),
```

### Middleware personalizado

```go
func LogMiddleware(next router.HandlerFunc) router.HandlerFunc {
    return func(ctx *router.Context) {
        log.Printf("%s %s", ctx.Request.Method, ctx.Request.URL.Path)
        next(ctx)
    }
}

// Aplicar globalmente
r.Use(LogMiddleware)
```

### Middleware por rota (via wrapper)

```go
func Register(r *router.Router) {
    // Rota SSR protegida por sessão
    loginRequired := secmiddleware.RequireLogin(fw.Sessions, "/login/")
    r.Handle("GET /dashboard/", loginRequired(func(ctx *router.Context) {
        render.For("painel").Render(ctx, "dashboard.html", nil)
    }))

    // Rota de API protegida por JWT
    jwtRequired := secmiddleware.RequireAuth(fw.Auth)
    r.Handle("GET /api/perfil/", jwtRequired(func(ctx *router.Context) {
        claims := ctx.Get("claims").(*auth.Claims)
        ctx.JSON(200, map[string]string{"user_id": claims.UserID})
    }))
}
```

### Content-Security-Policy (CSP) configurável

A CSP enviada por `SecureHeaders` em produção tem um padrão estrito
(`secmiddleware.DefaultCSP`: `default-src 'self'; script-src 'self';
style-src 'self' 'unsafe-inline'; img-src 'self' data:`) — configurável em
dois níveis, sem precisar editar `core/`:

**Global** — troca a política do site inteiro, normalmente a partir de
`CSP_POLICY` no `.env` (o bootstrap já chama isso sozinho):

```go
secmiddleware.SetCSP(cfg.Security.CSPPolicy) // policy vazia mantém o DefaultCSP
```

**Por rota** — sobrescreve só numa página que precisa de uma exceção
pontual (um script/iframe de terceiro só ali — ex: um mapa incorporado ou
um provedor de captcha), sem afrouxar a política padrão do resto do site:

```go
mapaCSP := "default-src 'self'; frame-src https://www.google.com/maps/"

router.Path("GET", "/contato/",
    secmiddleware.CSPOverride(mapaCSP)(views.ContatoView(fw)), "contato"),
```

Funciona porque middleware por rota roda depois do global (`SecureHeaders`
inclusive) na cadeia — o `Set` do `CSPOverride` substitui, nunca soma, o
header que `SecureHeaders` já escreveu. Também funciona sem
`SecureHeaders` ativo (development): só adiciona um header a mais,
inofensivo.

---

## 10. Banco de Dados

O Kyrux não importa nenhum driver. Você adiciona o que precisar.

### 1. Adicionar o driver

```bash
go get github.com/lib/pq                    # PostgreSQL
go get github.com/go-sql-driver/mysql       # MySQL / MariaDB
go get modernc.org/sqlite                   # SQLite (sem CGO)
go get github.com/jackc/pgx/v5/stdlib       # PostgreSQL (pgx)
go get github.com/microsoft/go-mssqldb      # SQL Server
go get github.com/sijms/go-ora/v2           # Oracle (puro Go)
```

### 2. Importar com blank identifier no `main.go`

```go
import _ "github.com/lib/pq"
```

### 3. Configurar o `.env`

Cada bloco iniciado por `DB_NAME` define um banco. O primeiro é o padrão (`Use()`). Adicione quantos blocos precisar — sem numeração, sem chaves extras.

```env
DB_NAME=principal
DB_ENABLED=true
DB_DRIVER=postgres
DB_DSN=postgres://user:password@localhost:5432/meudb?sslmode=disable

DB_NAME=analytics
DB_ENABLED=true
DB_DRIVER=postgres
DB_DSN=postgres://user:password@localhost:5432/analytics?sslmode=disable

DB_NAME=legado
DB_ENABLED=true
DB_DRIVER=mysql
DB_DSN=user:password@tcp(localhost:3306)/legado
```

### Usar a conexão nas views

As views recebem `fw` via closure no `routes.go`. Dentro da view, escolha o banco pelo nome definido em `DB_NAME`:

```go
func MinhaView(fw *bootstrap.Framework) router.HandlerFunc {
    return func(ctx *router.Context) {
        db          := fw.DB.Use()               // primeiro banco do .env (padrão)
        dbAnalytics := fw.DB.Use("analytics")    // banco cujo DB_NAME=analytics
        dbLegado    := fw.DB.Use("legado")        // banco cujo DB_NAME=legado

        posts, err := orm.FromDB[models.Post](db).All()
        stats, err := orm.FromDB[models.Stat](dbAnalytics).All()
        _ = dbLegado
    }
}
```

> `Use()` sem argumento é equivalente a `Use("default")` e sempre retorna o primeiro banco habilitado no `.env`.

### Múltiplas conexões em runtime

Para adicionar conexões fora do `.env` (ex: multi-tenant dinâmico):

```go
fw.DB.Add("tenant_xyz", "postgres", dsn)
db := fw.DB.Use("tenant_xyz")
```

### Transações

O callback recebe `*database.Tx` — commit se retornar `nil`, rollback caso
contrário (inclusive em panic). **O ORM funciona dentro da transação** via
`orm.FromTx` e `orm.CreateTx` (equivalente ao `transaction.atomic()` do Django):

```go
import "kyrux/core/database"

err := fw.DB.Use().Transaction(func(tx *database.Tx) error {
    pedido := Pedido{Total: 99.90}
    if err := orm.CreateTx(tx, &pedido); err != nil {
        return err // rollback automático
    }
    // SQL cru também funciona — tx embute *sql.Tx:
    _, err := tx.Exec("UPDATE stats SET total = total + 1")
    if err != nil {
        return err
    }
    return orm.FromTx[Saldo](tx).
        Where("id = ?", 1).
        Update(map[string]any{"valor": 100}) // commit se nil
})
```

### Multi-tenant com schema

```go
// Retorna uma cópia da conexão com o schema definido — a original não é alterada.
db := fw.DB.Use().WithSchema("tenant_abc")
```

Todas as queries executadas com essa conexão usarão `tenant_abc.<tabela>` automaticamente.
Veja a seção [ORM](#12-orm) para uso completo com multi-tenant.

### Drivers suportados

| Driver      | Módulo Go                          | Observação          |
|-------------|-------------------------------------|---------------------|
| `postgres`  | `github.com/lib/pq`                 |                     |
| `pgx`       | `github.com/jackc/pgx/v5/stdlib`    | Mais moderno, mas ~15-25% mais lento que `postgres` via `database/sql` (medido) |
| `mysql`     | `github.com/go-sql-driver/mysql`    | Também MariaDB      |
| `sqlite`    | `modernc.org/sqlite`                | Sem CGO             |
| `sqlite3`   | `github.com/mattn/go-sqlite3`       | Requer CGO          |
| `sqlserver` | `github.com/microsoft/go-mssqldb`   |                     |
| `oracle`    | `github.com/sijms/go-ora/v2`        | Puro Go, sem CGO    |

> Para MongoDB, Redis, Cassandra e DynamoDB use os clients nativos. Consulte `core/database/drivers.go`.

---

## 11. Migrations

O Kyrux inclui um sistema de migrations baseado em arquivos `.sql` numerados, com rastreamento automático de quais já foram aplicadas.

### Como funciona

- Arquivos `.sql` ficam em `database/migrations/`
- O nome segue o padrão `NNNN_descricao.sql` (ex: `0001_create_users.sql`)
- A tabela `kyrux_migrations` rastreia o que já foi aplicado — criada automaticamente no primeiro `migrate`
- Cada migration é aplicada **uma única vez** (idempotente)
- Se a tabela já existe no banco mesmo que o arquivo tenha sido removido, apenas registra a migration sem reexecutar o SQL

### Formato do arquivo de migration

Cada arquivo `.sql` pode conter duas seções separadas pelo marcador `-- down`:

```sql
-- up (implícito — tudo antes de "-- down")
CREATE TABLE IF NOT EXISTS posts (
    id         BIGSERIAL    PRIMARY KEY,
    titulo     VARCHAR(200) NOT NULL DEFAULT '',
    publicado  BOOLEAN      NOT NULL DEFAULT FALSE
);

CREATE UNIQUE INDEX IF NOT EXISTS posts_titulo_idx ON posts (titulo);

-- down
DROP INDEX IF EXISTS posts_titulo_idx;
DROP TABLE IF EXISTS posts;
```

A seção **up** é aplicada pelo `migrate`. A seção **down** é executada pelo `removemigration all` para desfazer o schema com segurança antes de remover o registro.

### Gerar migrations automaticamente (`makemigrations`)

O comando lê todos os structs com `kyrux:"pk"` em `apps/*/models/*.go` e `core/security/auth/*.go`, compara com o schema das migrations existentes e gera o SQL com as seções up e down automaticamente:

- **Tabela nova** → `CREATE TABLE` completo (+ índices unique/fk).
- **Campo novo em model existente** → `ALTER TABLE ... ADD COLUMN` (autodetectado).
- **Campo removido do model** → a remoção **não** é gerada (perda de dados);
  o comando avisa e escreve a sugestão `-- ALTER TABLE ... DROP COLUMN ...;`
  comentada na migration, para você descomentar após revisar.
- **Renomear campo ou mudar tipo** → migration manual (o detector veria um
  ADD + um aviso de coluna removida).

```bash
go run main.go makemigrations
```

> Revise o arquivo gerado antes de aplicar — índices compostos, constraints e defaults personalizados devem ser ajustados manualmente.

### Aplicar migrations (`migrate`)

```bash
go run main.go migrate
```

Aplica a seção **up** de todos os arquivos `.sql` ainda não registrados em `kyrux_migrations`. Múltiplas instruções SQL por arquivo são suportadas (separadas por `;`).

**Comportamento inteligente:**
- Se o arquivo SQL ainda existe: executa normalmente e registra
- Se o arquivo foi removido mas a tabela existe no banco: apenas registra (evita erros de "table already exists")
- Se já foi registrada: pula (status `~`)

### Remover uma migration (`removemigration`)

```bash
# Remove apenas do disco (permite regenerar a migration)
go run main.go removemigration 0001

# Executa a seção down, remove o registro do banco e apaga o arquivo
go run main.go removemigration 0001 all
```

O `removemigration all` executa as seguintes etapas em ordem:
1. Lê o arquivo e extrai a seção `-- down`
2. Executa o SQL de reversão no banco (ex: `DROP TABLE IF EXISTS ...`)
3. Remove o registro de `kyrux_migrations`
4. Apaga o arquivo do disco

> Se o arquivo não tiver seção `-- down`, o comando aborta com uma mensagem indicando o que adicionar — nunca remove silenciosamente sem desfazer o schema.

### Tipos SQL gerados

| Tipo Go | PostgreSQL | MySQL / SQLite |
|---|---|---|
| `string` (sem `size`) | `TEXT` | `TEXT` |
| `string` + `kyrux:"size:N"` | `VARCHAR(N)` | `VARCHAR(N)` |
| `int`, `int32` | `INTEGER` | `INTEGER` |
| `int64` | `BIGINT` | `INTEGER` |
| `float32`, `float64` | `DECIMAL` | `DECIMAL` |
| `bool` | `BOOLEAN` | `BOOLEAN` |
| `time.Time` | `TIMESTAMPTZ` | `DATETIME` |
| campo `kyrux:"pk"` | `BIGSERIAL PRIMARY KEY` | `INTEGER PRIMARY KEY` |
| campo `kyrux:"unique"` | `CREATE UNIQUE INDEX` | `CREATE UNIQUE INDEX` |
| campo `kyrux:"fk:tabela"` | `REFERENCES tabela(id)` + `CREATE INDEX` | idem |
| campo `kyrux:"fts"` | Índice GIN | `FULLTEXT` (MySQL) / tabela virtual FTS5 + triggers (SQLite) — diferente entre os dois, ver [Busca full-text (Search)](#busca-full-text-search) |

> Para `fk:` a tabela referenciada precisa existir antes — declare o model
> referenciado primeiro ou em uma migration anterior.

Campos com ponteiro (`*string`, `*int`) são gerados sem `NOT NULL`. Campos não-ponteiro recebem `NOT NULL DEFAULT <zero>`.

### Migrations manuais

Para alterações que o `makemigrations` não cobre (ALTER TABLE, índices compostos, dados iniciais), inclua sempre a seção down:

```sql
-- database/migrations/0003_add_slug_to_posts.sql
ALTER TABLE posts ADD COLUMN IF NOT EXISTS slug VARCHAR(200) NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS posts_slug_idx ON posts (slug);

-- down
DROP INDEX IF EXISTS posts_slug_idx;
ALTER TABLE posts DROP COLUMN IF EXISTS slug;
```

Execute com `go run main.go migrate`.

### Estrutura do diretório

```
database/
└── migrations/
    ├── 0001_create_users.sql    ← incluída pelo framework (auth.User)
    ├── 0002_auto.sql            ← gerada por makemigrations
    └── 0003_add_slug.sql        ← escrita manualmente
```

> `makemigrations` **não** gera migrations de ALTER TABLE — apenas cria novas tabelas. Para mudanças em tabelas existentes, escreva a migration manualmente.

---

## 12. ORM

ORM leve e fluente integrado ao framework. Usa generics, reflection cacheada e SQL explícito com placeholders — sem magia, sem surpresas.

```go
import "kyrux/core/orm"
```

### Definição de model

Um model é qualquer struct Go com campos exportados. Use a tag `kyrux` para configurar comportamento:

```go
type Post struct {
    ID        int64     `kyrux:"pk"`
    Titulo    string    `kyrux:"size:200"`
    Slug      string    `kyrux:"size:200,unique"`
    Publicado bool      `kyrux:"default:false"`
    CriadoEm time.Time `kyrux:"column:criado_em,default:NOW()"`
}

// Model com campos sensíveis:
type Cliente struct {
    ID    int64  `kyrux:"pk"`
    Nome  string
    CPF   string `kyrux:"size:14,encrypt"` // AES-256-GCM: cifrado no banco, decifrado na leitura
    Token string `kyrux:"hash"`            // Argon2id: hash na escrita, nunca revertido
}
```

#### Tags disponíveis

| Tag | Descrição |
|---|---|
| `kyrux:"pk"` | Chave primária. Ignorado no INSERT, preenchido de volta após criação. |
| `kyrux:"column:nome"` | Override do nome da coluna SQL. Padrão: `snake_case` do nome do campo Go. |
| `kyrux:"size:N"` | Tamanho máximo — usado no `makemigrations` para gerar `VARCHAR(N)`. |
| `kyrux:"unique"` | Gera `CREATE UNIQUE INDEX` no `makemigrations` (efeito apenas na migration). |
| `kyrux:"default:valor"` | Valor SQL usado no INSERT quando o campo for zero Go. Ex: `default:NOW()`, `default:true`, `default:0`. Literal SQL — sem placeholder `?`. |
| `kyrux:"hash"` | Hash automático **Argon2id+pepper** na escrita (Create/Update). Nunca revertido. |
| `kyrux:"encrypt"` | **AES-256-GCM** — cifra na escrita, decifra automaticamente na leitura. Requer `FIELD_ENCRYPTION_KEY`. |
| `kyrux:"login"` | Exclusivo do `auth.User`. Marca o campo de login (username ou email). Apenas um campo por struct. Imutável após o primeiro migrate. |
| `kyrux:"autonow"` | `CURRENT_TIMESTAMP` automático em todo `Update` (ex: `updated_at`). Também preenche no `Create` se o campo estiver zerado. |
| `kyrux:"fk:tabela"` | `REFERENCES tabela(id)` + índice na migration. A tabela referenciada precisa existir antes (declare-a primeiro ou em migration anterior). No admin, vira `<select>` com as linhas existentes. |
| `kyrux:"fklabel:coluna"` | Só afeta o admin — coluna de `tabela` (do `fk:`) usada como rótulo no `<select>`. Sem isso, mostra o id. |
| `kyrux:"fts"` | Habilita busca full-text nativa via `Query.Search()` — ver [Busca full-text (Search)](#busca-full-text-search) abaixo. |
| `kyrux:"image"` | Campo `string` vira upload de arquivo no admin — ver [Upload de imagem](#upload-de-imagem-kyruximage) na seção 20. |

> **`default:valor`** — quando o campo tiver valor zero Go (`""`, `0`, `false`),
> o ORM usa o literal diretamente no SQL (`VALUES (..., NOW(), ...)`), sem passar como argumento.
> Útil para timestamps, UUIDs e qualquer função SQL de banco. Como entra sem
> placeholder, `valor` só aceita número, string entre aspas simples ou
> identificador/palavra-chave opcionalmente seguido de `()` (`NOW()`,
> `CURRENT_TIMESTAMP`, `true`, `'pending'`) — qualquer outra forma faz o
> framework recusar o model com panic na primeira vez que ele é usado (erro
> de configuração, não de runtime). O mesmo vale para `fk:tabela` e
> `fklabel:coluna`: só identificadores simples.

#### Nome da tabela

Gerado automaticamente a partir do nome do struct em `snake_case` plural:

| Struct | Tabela |
|---|---|
| `User` | `users` |
| `Post` | `posts` |
| `Category` | `categories` |
| `UserProfile` | `user_profiles` |
| `Address` | `addresses` |

### `From` vs `FromDB`

| Função | Recebe | Uso |
|---|---|---|
| `orm.From[T](connName string)` | Nome da conexão registrada (`"default"`, `"analytics"`, ...) | Quando você só tem o nome da conexão em mãos, sem `fw.DB` por perto. |
| `orm.FromDB[T](db *database.DB)` | Uma conexão já resolvida (`fw.DB.Use()`, `fw.DB.Use("analytics")`, com ou sem `WithSchema`) | O padrão usado em toda esta seção — o que views e funções em `models/` normalmente recebem. |

`orm.From[T]("default")` e `orm.FromDB[T](fw.DB.Use())` chegam ao mesmo lugar —
`From` só resolve o nome para uma `*database.DB` internamente e delega para
`FromDB`. Passar uma `*database.DB` para `From` (ou uma string para `FromDB`)
não compila — são assinaturas diferentes; escolha conforme o que você já tem
em mãos.

### Leitura

#### All — buscar todos

```go
db := fw.DB.Use()

posts, err := orm.FromDB[Post](db).All()

// Com filtros
posts, err := orm.FromDB[Post](db).
    Where("publicado = ?", true).
    OrderBy("criado_em DESC").
    Limit(10).
    Offset(20).  // página 3
    All()
```

#### First — buscar o primeiro

Retorna `sql.ErrNoRows` se nenhuma linha for encontrada.

```go
post, err := orm.FromDB[Post](db).
    Where("slug = ?", slug).
    First()

if errors.Is(err, sql.ErrNoRows) {
    ctx.Error(404)
    return
}
```

#### Count — contar linhas

```go
total, err := orm.FromDB[Post](db).Count()

publicados, err := orm.FromDB[Post](db).
    Where("publicado = ?", true).
    Count()
```

#### Métodos de filtro encadeáveis

Para o filtro comum (igualdade, comparação, `LIKE`, `NULL`), prefira os
métodos tipados abaixo: eles validam o nome da coluna e não têm risco de SQL
injection por descuido. `Where`/`OrWhere` continuam existindo para o que os
tipados não cobrem — SQL livre, então a segurança depende de nunca
concatenar entrada do usuário na condição, só passá-la em `args`.

| Método | Descrição |
|---|---|
| `WhereEq(col string, val any)` / `OrWhereEq(...)` | `col = ?` — forma tipada e segura de `Where("col = ?", val)`. |
| `WhereNe(col string, val any)` | `col <> ?` |
| `WhereGt/WhereGte/WhereLt/WhereLte(col string, val any)` | `col >`, `>=`, `<`, `<=` `?` |
| `WhereLike(col, pattern string)` / `OrWhereLike(...)` | `col LIKE ?` — inclua os `%` no pattern: `WhereLike("nome", "%maria%")`. |
| `WhereNull(col string)` / `WhereNotNull(col string)` | `col IS NULL` / `col IS NOT NULL`. |
| `Where(cond string, args ...any)` | Condição `AND` em **SQL livre** — use `?` como placeholder, nunca concatene valores na string. Múltiplos `Where` são combinados com `AND`. Idêntico a `WhereSQL`, mantido por compatibilidade. |
| `OrWhere(cond string, args ...any)` | Condição `OR` em SQL livre (precedência: AND liga mais forte, como no SQL). Idêntico a `OrWhereSQL`. |
| `WhereSQL(cond string, args ...any)` / `OrWhereSQL(...)` | Sinônimos de `Where`/`OrWhere` — nome recomendado quando o filtro é SQL livre de propósito, deixando isso explícito na leitura do código. |
| `WhereIn(col string, vals ...any)` | `col IN (...)` com expansão de placeholders. Aceita slice: `WhereIn("id", ids)`. Lista vazia = nenhum resultado; lista com mais de 5000 valores gera erro (pagine ou use subquery/JOIN). |
| `OrderBy(cols ...string)` | Define `ORDER BY` — múltiplas colunas: `OrderBy("criado_em DESC", "id ASC")`. `orm.Asc("col")`/`orm.Desc("col")` são sintaxe alternativa para montar isso programaticamente: `OrderBy(orm.Desc("criado_em"), orm.Asc("id"))`. |
| `Join(tabela, on)` / `LeftJoin(tabela, on)` | JOIN para **filtrar** pela tabela relacionada. O SELECT vira `tabela_base.*` — o resultado continua `[]T`. |
| `Search(col, termo)` | Busca full-text nativa numa coluna `kyrux:"fts"` — ver [Busca full-text (Search)](#busca-full-text-search) abaixo. |
| `Select(cols ...string)` | Restringe as colunas do `SELECT` (padrão: `SELECT *`). Colunas fora da lista ficam com zero value no struct — não use para depois `Update` o registro inteiro de volta. |
| `Distinct()` | `SELECT DISTINCT`. |
| `Limit(n int)` | Máximo de linhas retornadas. |
| `Offset(n int)` | Linhas a pular — use com `Limit` para paginação. |

```go
// Antes (SQL livre) — continua funcionando, mas sem validação de coluna:
orm.FromDB[Post](db).Where("status = ?", status).All()

// Preferido — coluna validada, mesmo resultado:
orm.FromDB[Post](db).WhereEq("status", status).All()
```

#### Métodos de execução

| Método | Retorno | Descrição |
|---|---|---|
| `All()` | `[]T, error` | Todas as linhas do filtro. |
| `Each(fn func(*T) error)` | `error` | Streaming linha a linha — memória O(1), para result sets grandes. |
| `First()` / `Last()` | `*T, error` | Primeira/última linha (Last inverte a ordenação; sem OrderBy usa PK DESC). |
| `Exists()` | `bool, error` | `SELECT 1 ... LIMIT 1` — mais barato que Count. |
| `Count()` | `int64, error` | Número de linhas do filtro. |
| `Sum/Avg/Min/Max(col)` | `float64, error` | Agregações numéricas (NULL → 0). |
| `GetOrCreate(defaults *T)` | `*T, bool, error` | Busca; se não existir, insere `defaults` (created=true). Preencha `defaults` com os valores do filtro também. Se o `Create` colidir com uma constraint `UNIQUE` (corrida: outro processo criou entre a busca e a inserção), refaz a busca automaticamente em vez de propagar o erro — só protege de verdade quando as colunas do filtro têm `UNIQUE` no banco. |
| `UpdateOrCreate(values, defaults *T)` | `bool, error` | Atualiza as linhas do filtro; se nenhuma, insere `defaults`. Mesma recuperação de corrida de `GetOrCreate`. |

### Criação

Passe sempre um **ponteiro** para que o campo PK seja preenchido de volta.

```go
post := Post{
    Titulo:    "Olá Kyrux",
    Slug:      "ola-kyrux",
    Publicado: true,
}

err := orm.Create(db, &post)
fmt.Println(post.ID) // preenchido com o ID gerado pelo banco
```

PostgreSQL usa `RETURNING` internamente — sem round-trip extra.
MySQL e SQLite usam `LastInsertId`.

Para inserir muitas linhas, `CreateAll` faz um único INSERT multi-VALUES
(chunks de 500 — equivalente ao `bulk_create` do Django):

```go
posts := []*Post{{Titulo: "a"}, {Titulo: "b"}, {Titulo: "c"}}
err := orm.CreateAll(db, posts)
// No PostgreSQL, posts[i].ID são preenchidos de volta.
```

### Atualização

Exige ao menos um `Where` para evitar updates acidentais em toda a tabela.

```go
err := orm.FromDB[Post](db).
    Where("id = ?", 1).
    Update(map[string]any{
        "titulo":    "Título atualizado",
        "publicado": true,
    })
```

### Deleção

Exige ao menos um `Where` para evitar deleções acidentais em toda a tabela.

```go
err := orm.FromDB[Post](db).
    Where("id = ?", 1).
    Delete()
```

### Paginação

`Paginate` executa um `COUNT(*)` + `SELECT` com `LIMIT/OFFSET` em uma só chamada e retorna metadados prontos para uso no template.

```go
page := ctx.QueryInt("page", 1)

p, err := orm.FromDB[Post](db).
    Where("publicado = ?", true).
    OrderBy("criado_em DESC").
    Paginate(page, 20) // página atual, itens por página

// p.Items      → []Post da página atual
// p.Total      → total de registros
// p.TotalPages → número de páginas
// p.HasNext    → true se há próxima página
// p.HasPrev    → true se há página anterior
// p.Page       → página atual
// p.PageSize   → itens por página

render.For("blog").Render(ctx, "lista.html", map[string]any{
    "page": p,
})
```

Para tabelas grandes e feeds infinitos, `PaginateNoCount` evita o `COUNT(*)`
(caro no PostgreSQL): busca `pageSize+1` linhas e infere `HasNext`.
`Total` fica em `-1` e `TotalPages` em `0` (desconhecidos):

```go
p, err := orm.FromDB[Post](db).OrderBy("id DESC").PaginateNoCount(page, 20)
```

`Paginate`/`PaginateNoCount` limitam `pageSize` a 1000 (valores maiores são
reduzidos silenciosamente) — importante quando `page`/`pageSize` vêm direto
de query string sem validação própria.

Ambas ainda usam `OFFSET`, que fica mais caro conforme a página avança (o
banco varre e descarta as linhas puladas). Para tabelas muito grandes ou
scroll infinito, use `PaginateAfter` — keyset (cursor) pagination, cujo
custo não cresce com a posição da página:

```go
cursor := ctx.QueryInt64("cursor", 0) // 0 = primeira página

p, err := orm.FromDB[Post](db).
    Where("publicado = ?", true).
    PaginateAfter("id", cursor, false, 20) // col, after, desc, limit

// p.Items      → []Post da página atual
// p.NextCursor → passe para PaginateAfter na próxima chamada
// p.HasNext    → true se há próxima página
```

`col` precisa ser única e ordenável (tipicamente a PK ou uma coluna
`autonow`); `PaginateAfter` adiciona `col > ?` (ou `col < ?` com `desc:
true`) e ordena por `col` — não combine com um `OrderBy` próprio na mesma
query, pois `PaginateAfter` substitui a ordenação por `col`. Diferente de
`Paginate`, não há `Total`/`TotalPages`/números de página — só "próxima
página existe ou não".

### Relações: filtrar com Join, carregar com Prefetch

Declare a FK no model com `kyrux:"fk:tabela"` (gera `REFERENCES` + índice na
migration). Para **filtrar** pela tabela relacionada, use `Join` — o SELECT
usa `tabela_base.*`, então o resultado continua sendo `[]T` sem conflito de
colunas (qualifique as colunas do `Where`):

```go
// Posts de usuários ativos:
posts, _ := orm.FromDB[Post](db).
    Join("users", "users.id = posts.user_id").
    Where("users.is_active = ?", true).
    All()
```

Para **carregar** os registros relacionados, use `Prefetch` — o equivalente
explícito do `prefetch_related` do Django (1 query + agrupamento em memória,
sem N+1):

```go
posts, _ := orm.FromDB[Post](db).All()
ids := make([]int64, len(posts))
for i, p := range posts { ids[i] = p.ID }

// 1 query: SELECT * FROM comentarios WHERE post_id IN (...)
porPost, _ := orm.Prefetch[Comentario](db, "post_id", ids,
    func(c *Comentario) int64 { return c.PostID })

for _, p := range posts {
    comentarios := porPost[p.ID] // []Comentario do post
}
```

> `Update`/`Delete` não aceitam `Join` (sintaxe não-portável entre bancos) —
> use subquery no `Where`: `Where("user_id IN (SELECT id FROM users WHERE ...)")`.

No template:

```html
{{ range .page.Items }}
    <article>{{ .Titulo }}</article>
{{ end }}

{{ if .page.HasPrev }}
    <a href="?page={{ sub .page.Page 1 }}">← Anterior</a>
{{ end }}
{{ if .page.HasNext }}
    <a href="?page={{ add .page.Page 1 }}">Próxima →</a>
{{ end }}
```

### Busca full-text (Search)

`LIKE`/`ILIKE` não escala (full scan) e não é preciso (falha com acentos,
maiúsculas, plural). Busca de verdade usa o mecanismo de full-text nativo do
banco — o Kyrux expõe isso via `Query.Search`, suportado em **três drivers**,
cada um com seu próprio índice e sintaxe internos:

| Driver | Índice gerado pelo `makemigrations` | Mecanismo de busca |
|---|---|---|
| `postgres`/`pgx` | `CREATE INDEX ... USING GIN (to_tsvector('portuguese', col))` | `to_tsvector`/`plainto_tsquery` + `ts_rank` |
| `mysql` | `CREATE FULLTEXT INDEX` | `MATCH(col) AGAINST(? IN NATURAL LANGUAGE MODE)` |
| `sqlite`/`sqlite3` | Tabela virtual `FTS5` (`<tabela>_<coluna>_fts`) + triggers de INSERT/UPDATE/DELETE que a mantêm sincronizada | `MATCH` na tabela virtual, `JOIN` pelo `rowid` |

Em qualquer outro driver (`sqlserver`, `oracle`), `Search` retorna erro —
**não existe fallback silencioso em `LIKE`**, que não seria full-text de verdade.

#### Marcar o campo

```go
type Artigo struct {
    ID       int64  `kyrux:"pk"`
    Titulo   string `kyrux:"size:200"`
    Conteudo string `kyrux:"fts"`
}
```

`go run main.go makemigrations` detecta a tag e gera o índice/tabela virtual
certos para o `DB_DRIVER` do seu `.env` automaticamente.

#### Buscar

```go
artigos, err := orm.FromDB[Artigo](db).Search("conteudo", "framework golang").All()
```

- **Resultado já vem ordenado por relevância** (mais relevante primeiro) — não
  precisa chamar `OrderBy` para isso. Se chamar `OrderBy` depois de `Search`,
  ele só **acrescenta** um critério de desempate; a relevância continua sendo
  o critério primário.
- **Termos múltiplos são combinados com E, não OU** — `Search("conteudo",
  "framework golang")` só encontra registros que contenham **as duas**
  palavras (comportamento do `plainto_tsquery`/`MATCH...NATURAL LANGUAGE
  MODE`/FTS5). Busque um termo por vez se quiser OU.
- `Search` exige que a coluna tenha `kyrux:"fts"` no model — chamar em
  qualquer outra coluna retorna erro (falha rápida, sem gerar uma busca que
  nunca usaria índice nenhum).

```go
// Composável com o resto do builder normalmente:
orm.FromDB[Artigo](db).
    Search("conteudo", "kyrux").
    Where("publicado = ?", true).
    Limit(20).
    All()
```

> No SQLite, `Search` adiciona um `JOIN` interno com a tabela virtual FTS5 —
> por isso `Update`/`Delete` encadeados diretamente após `Search` são
> rejeitados (mesma regra que já vale para `Join`/`LeftJoin`): filtre por
> subquery no `Where` nesses casos.

### Multi-tenant com schema

```go
// Middleware de tenant define o schema
db := fw.DB.Use().WithSchema("tenant_" + tenantID)

// Todas as queries usam o schema automaticamente
posts, _ := orm.FromDB[Post](db).Where("publicado = ?", true).All()
// → SELECT * FROM tenant_abc.posts WHERE publicado = ?

post := Post{Titulo: "Novo"}
orm.Create(db, &post)
// → INSERT INTO tenant_abc.posts (titulo, ...) VALUES (?)
```

### Compatibilidade de drivers

O ORM detecta o driver automaticamente. Você escreve sempre `?` — para PostgreSQL os placeholders são reescritos para `$1, $2, ...` internamente.

| Driver | Placeholder gerado |
|---|---|
| `postgres`, `pgx` | `$1, $2, ...` |
| `mysql`, `sqlite` | `?` |

### Uso em models

Padrão recomendado — funções no pacote `models` que recebem `*database.DB`:

```go
// apps/blog/models/models.go
package models

import (
    "kyrux/core/database"
    "kyrux/core/orm"
)

type Post struct {
    ID        int64  `kyrux:"pk"`
    Titulo    string
    Publicado bool
}

func ListarPublicados(db *database.DB) ([]Post, error) {
    return orm.FromDB[Post](db).
        Where("publicado = ?", true).
        OrderBy("id DESC").
        All()
}

func BuscarPorID(db *database.DB, id int64) (*Post, error) {
    return orm.FromDB[Post](db).
        Where("id = ?", id).
        First()
}

func Criar(db *database.DB, post *Post) error {
    return orm.Create(db, post)
}
```

```go
// apps/blog/views/views.go
func ListaView(ctx *router.Context) {
    posts, err := models.ListarPublicados(fw.DB.Use())
    if err != nil {
        ctx.Error(500)
        return
    }
    render.For("blog").Render(ctx, "lista.html", map[string]any{
        "posts": posts,
    })
}
```

---

## 13. Cache

Chave/valor com TTL. Ativado via `CACHE_ENABLED=true`, com dois drivers:

- **`CACHE_DRIVER=memory`** (padrão) — mapa local ao processo. `Get` devolve
  exatamente o valor Go original (mesmo ponteiro/tipo passado a `Set`).
- **`CACHE_DRIVER=redis`** — chaves compartilhadas entre todas as réplicas via
  `CACHE_ADDR` (+ `CACHE_PASSWORD` se o Redis tiver `requirepass`). Os valores
  são serializados com `encoding/json`: um `Set` com um struct/slice volta do
  `Get` como `map[string]any`/`[]any` (JSON decodificado), **não** no tipo
  original — o exemplo abaixo (`v.([]*models.Post)`) só funciona em modo
  memória. Prefira tipos simples nesse driver, ou decodifique você mesmo a
  partir do valor bruto retornado.

Se `CACHE_DRIVER=redis` e a conexão falhar no boot, o Kyrux cai para memória
automaticamente e loga um aviso — nunca recusa subir por causa do cache.

```go
// Guardar
fw.Cache.Set("posts:lista", posts, 5*time.Minute)

// Recuperar
if v, ok := fw.Cache.Get("posts:lista"); ok {
    posts := v.([]*models.Post)
}

// Remover
fw.Cache.Delete("posts:lista")
```

**Padrão recomendado de uso em view:**

```go
func ListaView(ctx *router.Context) {
    const cacheKey = "posts:lista"

    if v, ok := fw.Cache.Get(cacheKey); ok {
        render.For("blog").Render(ctx, "lista.html", map[string]any{
            "posts": v,
        })
        return
    }

    posts, _ := models.ListarPublicados(fw.DB.Use())
    fw.Cache.Set(cacheKey, posts, 2*time.Minute)

    render.For("blog").Render(ctx, "lista.html", map[string]any{
        "posts": posts,
    })
}
```

---

## 14. Sessões

Sessões em memória server-side com TTL configurável via `SESSION_TTL`. O cookie `kyrux_session` é `HttpOnly`, `SameSite=Strict` e `Secure` em produção — a flag Secure é aplicada automaticamente pelo bootstrap quando `APP_ENV=production`, inclusive atrás de proxy reverso (nginx/Caddy).

> ⚠️ **Estado em memória**: sessões (e revogações de JWT) vivem no processo.
> Reiniciar/redeployar desloga todos os usuários, e múltiplas réplicas atrás de
> load balancer não compartilham sessões — nesse cenário, use *sticky sessions*
> no balanceador até existir backend externo (ex: Redis).

### API de sessão direta

Use quando precisar de controle total sobre o que é guardado na sessão.
Use `Get`/`Set`/`Delete` — são seguros para requisições paralelas da mesma sessão:

```go
import "kyrux/core/security/session"

// Criar sessão manualmente
sess, err := fw.Sessions.New()
sess.Set("chave", valor)
session.SetCookie(ctx.Writer, sess.ID, ctx.Request.TLS != nil)

// Ler sessão do request
sess, ok := session.FromRequest(ctx.Request, fw.Sessions)

// Ler valor
val, ok := sess.Get("chave")

// Remover sessão
fw.Sessions.Delete(sess.ID)
```

> Para autenticação com `auth.User`, use `auth.Login` e `auth.Logout` — veja a seção [15. Autenticação](#15-autenticação).

---

## 15. Autenticação

O Kyrux oferece dois modelos de autenticação: **SSR por sessão** (views HTML) e **JWT stateless** (APIs). Ambos coexistem.

### Model de usuário padrão

`auth.User` é o model de usuário do sistema, disponível em `kyrux/core/security/auth`.

```go
type User struct {
    ID        int64     `kyrux:"column:id,pk"`
    UUID      string    `kyrux:"column:uuid,size:36"`
    FirstName string    `kyrux:"column:first_name,size:150"`
    LastName  string    `kyrux:"column:last_name,size:150"`
    Username  string    `kyrux:"column:username,size:150,unique,login"` // campo de login padrão
    Email     *string   `kyrux:"column:email,size:254,unique"`           // opcional quando não é login
    Password  string    `kyrux:"column:password,size:128"`
    Group     string    `kyrux:"column:user_group,size:100"`
    IsAdmin   bool      `kyrux:"column:is_admin"`
    IsStaff   bool      `kyrux:"column:is_staff"`
    IsActive  bool      `kyrux:"column:is_active,default:true"` // nova conta ativa por padrão
    CreatedAt time.Time `kyrux:"column:created_at"`
    UpdatedAt time.Time `kyrux:"column:updated_at"`
}
```

A tag `login` marca qual campo é usado para autenticação. Apenas um campo pode ter essa tag. Trocar o campo após o primeiro migrate exige uma nova migration — trate como decisão de schema.

`Email *string` é ponteiro porque é opcional quando `Username` é o campo de login. O banco permite múltiplos `NULL` sem violar o índice `UNIQUE`. Quando `Email` tiver a tag `login`, `Username` passa a ser o campo opcional.

`auth.LoginFieldName()` retorna o nome do campo Go marcado com `login` (`"Username"` ou `"Email"`). Útil para adaptar formulários e lógica de autenticação dinamicamente:

```go
field := auth.LoginFieldName() // "Username" — determinado pelas tags do model
```

O hash de senha usa **Argon2id** (64 MB, 3 iterações, 4 threads) com pepper definido em `PASSWORD_PEPPER`.

```go
// E-mail é *string — use ponteiro ou nil
email := "joao@exemplo.com"
user := &auth.User{Username: "joao", Email: &email}
user.SetPassword("minha-senha-forte")   // hash Argon2id + pepper
user.CheckPassword("minha-senha-forte") // → true
user.FullName()                         // → "João Silva"

// Sem e-mail (campo opcional quando login = username)
user := &auth.User{Username: "joao", Email: nil}
```

### Autenticação SSR (sessão + cookie)

Indicada para views HTML renderizadas no servidor. O campo de login é determinado pela tag `kyrux:"login"` no model `auth.User` — por padrão `username`. Alterar esse campo após o primeiro migrate equivale a uma mudança de schema e exige nova migration.

#### Login

```go
import "kyrux/core/security/auth"

func LoginView(ctx *router.Context) {
    if ctx.Request.Method == http.MethodGet {
        render.For("auth").Render(ctx, "login.html", nil)
        return
    }

    // Use o nome do campo de login definido no model (tag kyrux:"login")
    loginValue := ctx.Request.FormValue(strings.ToLower(auth.LoginFieldName()))
    password   := ctx.Request.FormValue("password")

    _, err := auth.Login(fw.DB.Use(), fw.Sessions, ctx.Writer, ctx.Request, loginValue, password)
    switch err {
    case nil:
        ctx.Redirect("/dashboard/", http.StatusFound)
    case auth.ErrUserNotFound, auth.ErrWrongPassword:
        render.For("auth").Render(ctx, "login.html", map[string]any{
            "erro": "Usuário ou senha inválidos.",
        })
    case auth.ErrInactiveUser:
        render.For("auth").Render(ctx, "login.html", map[string]any{
            "erro": "Conta inativa.",
        })
    default:
        ctx.Error(500)
    }
}
```

#### Logout

```go
func LogoutView(ctx *router.Context) {
    auth.Logout(fw.Sessions, ctx.Request, ctx.Writer)
    ctx.Redirect("/login/", http.StatusFound)
}
```

#### Obter usuário logado

```go
func DashboardView(ctx *router.Context) {
    user, err := auth.GetUser(fw.DB.Use(), fw.Sessions, ctx.Request)
    if err != nil {
        ctx.Redirect("/login/", http.StatusFound)
        return
    }
    render.For("painel").Render(ctx, "dashboard.html", map[string]any{
        "user": user,
    })
}
```

#### Proteger rotas com `RequireLogin`

```go
import secmiddleware "kyrux/core/security/middleware"

// Rota individual
loginRequired := secmiddleware.RequireLogin(fw.Sessions, "/login/")
r.Handle("GET /dashboard/", loginRequired(DashboardView))

// Todas as rotas do app
r.Use(secmiddleware.RequireLogin(fw.Sessions, "/login/"))
```

A sessão fica disponível em `ctx.Get("session")` dentro da view protegida:

```go
sess := ctx.Get("session").(*session.Session)
```

#### Redirecionamento `?next=` pós-login

`RequireLogin` adiciona automaticamente `?next=<URL atual>` ao redirect para o login. Após autenticar, use `auth.NextURL` para redirecionar o usuário de volta:

```go
func LoginView(ctx *router.Context) {
    if ctx.Request.Method == http.MethodPost {
        _, err := auth.Login(fw.DB.Use(), fw.Sessions, ctx.Writer, ctx.Request,
            ctx.Request.FormValue("username"),
            ctx.Request.FormValue("password"),
        )
        if err == nil {
            // auth.NextURL lê ?next= e valida que é URL relativa (proteção open redirect)
            dest := auth.NextURL(ctx.Request, "/dashboard/")
            ctx.Redirect(dest, http.StatusFound)
            return
        }
        // ... tratar erro ...
    }
    render.For("auth").Render(ctx, "login.html", nil)
}
```

`auth.NextURL(r, fallback)` aceita apenas URLs relativas começando com `/` (não `//`) — qualquer tentativa de open redirect é ignorada e o fallback é retornado.

---

### Autenticação JWT (APIs stateless)

Indicada para APIs consumidas por clientes externos (mobile, SPA, etc.).

#### Gerar token

```go
func LoginAPIView(ctx *router.Context) {
    // ... validar usuário ...
    token, err := fw.Auth.GenerateToken(strconv.FormatInt(user.ID, 10), 24*time.Hour)
    if err != nil {
        ctx.Error(500)
        return
    }
    ctx.JSON(200, map[string]string{"token": token})
}
```

#### Validar token manualmente

```go
claims, err := fw.Auth.ValidateToken(token)
// claims.UserID    string
// claims.ExpiresAt time.Time
```

#### Middleware `RequireAuth`

O token deve vir no header `Authorization: Bearer <token>`. As claims ficam em `ctx.Get("claims")`.

```go
r.Handle("GET /api/perfil/", secmiddleware.RequireAuth(fw.Auth)(func(ctx *router.Context) {
    claims := ctx.Get("claims").(*auth.Claims)
    ctx.JSON(200, map[string]string{"user_id": claims.UserID})
}))
```

---

## 16. EventBus

Sistema de eventos desacoplados. Útil para comunicação entre apps sem importação direta.

### Publicar evento

```go
// Em qualquer view ou service:
fw.Events.Publish("usuario.criado", map[string]any{
    "id":    usuario.ID,
    "email": usuario.Email,
})
```

### Assinar evento

```go
// No init() do app ou no Register():
func init() {
    // fw deve ser acessível aqui (via variável de pacote ou injeção)
    fw.Events.Subscribe("usuario.criado", func(payload any) {
        dados := payload.(map[string]any)
        email := dados["email"].(string)
        enviarEmailBoasVindas(email)
    })
}
```

### Cancelar assinatura

```go
fw.Events.Unsubscribe("usuario.criado")
```

> Handlers do EventBus rodam em goroutines separadas. Use sincronização se necessário.

### Fila de tarefas (Queue)

Cache, EventBus e Queue têm papéis distintos:

| Componente | Papel | Garantia |
|---|---|---|
| `fw.Cache` | Armazenamento chave/valor com TTL | Leitura rápida; dados podem expirar |
| `fw.Events` | Pub/sub fire-and-forget | TODOS os subscribers recebem; sem retry |
| `fw.Queue` | Fila de tarefas em background | Cada tarefa processada por UM worker, com retry e drenagem no shutdown |

Use a Queue para trabalho que não pode se perder num pico: e-mails, webhooks,
processamento de mídia. Ative no `.env`:

```env
QUEUE_ENABLED=true
QUEUE_DRIVER=memory   # memory | redis
QUEUE_WORKERS=4
```

- **`QUEUE_DRIVER=memory`** (padrão) — fila local ao processo (channel Go),
  não compartilhada entre réplicas, perdida num restart.
- **`QUEUE_DRIVER=redis`** — lista Redis (`QUEUE_ADDR` + `QUEUE_PASSWORD` se
  houver `requirepass`) compartilhada entre réplicas: qualquer instância pode
  processar uma tarefa enfileirada por outra, e o que ainda não foi
  processado sobrevive a um restart (fica na lista até alguém consumir).
  Todas as réplicas precisam registrar os mesmos handlers via `Register`.
  Assim como no cache Redis, o payload é serializado com JSON — um handler
  que espera um struct recebe `map[string]any`, não o tipo original. Se a
  conexão falhar no boot, cai para memória automaticamente com um aviso.

Registre handlers no `Register` do app e enfileire nas views:

```go
// Register do app — antes de qualquer Enqueue:
fw.Queue.Register("email.boasvindas", func(payload any) error {
    return enviarEmail(payload.(string)) // erro ≠ nil → retry (3× com backoff)
})

// Na view:
if err := fw.Queue.Enqueue("email.boasvindas", usuario.Email); err != nil {
    // ErrQueueFull = backpressure explícito: decida entre 503, log ou retry
    log.Printf("fila cheia: %v", err)
}
```

No shutdown do servidor a fila é **drenada** antes do processo encerrar —
tarefas enfileiradas não se perdem num deploy. Um panic num handler não
derruba o worker (logado com stack trace).

---

## 17. Realtime (DOM sem JS)

O Kyrux injeta automaticamente um WebSocket em toda página renderizada. O desenvolvedor não escreve nenhum JS — apenas atributos HTML e funções Go.

### 1. Marcar o elemento no template

```html
<div kyrux-target="lista-posts">
  {% include "partials/lista.html" %}
</div>
```

### 2. Atualizar o DOM na view

```go
func CriarPostView(ctx *router.Context) {
    // ... salvar no banco ...

    // Renderiza o fragmento atualizado
    html, _ := render.Partial("blog", "partials/lista.html", map[string]any{
        "posts": models.ListarPublicados(fw.DB.Use()),
    })

    fw.Realtime.Replace("lista-posts", html)   // substitui o innerHTML
    fw.Realtime.Append("lista-posts", html)    // adiciona ao final
    fw.Realtime.Prepend("lista-posts", html)   // adiciona ao início
    fw.Realtime.Remove("lista-posts")          // remove o elemento do DOM

    ctx.Redirect("/posts/", http.StatusFound)
}
```

> ⚠️ **`Replace`/`Append`/`Prepend`/`Remove` são broadcast global** — todos os
> clientes conectados recebem a atualização. Nunca as use com dados privados.

### Atualizar apenas um usuário (variantes `For`)

Para conteúdo por usuário (saldo, notificações, carrinho), use as variantes
com escopo de sessão — apenas as conexões (abas) daquela sessão recebem:

```go
sess, _ := ctx.Get("session") // colocado por RequireLogin
s := sess.(*session.Session)

fw.Realtime.ReplaceFor(s.ID, "saldo", html)       // só as abas deste usuário
fw.Realtime.AppendFor(s.ID, "notificacoes", html)
fw.Realtime.RemoveFor(s.ID, "alerta")
fw.Realtime.ReplaceTextFor(s.ID, "contador", "3") // textContent — seguro p/ conteúdo do usuário
```

### 3. Broadcast via EventBus

```go
// Disparar atualização para todos os clientes conectados via evento:
fw.Realtime.Broadcast("novo.post", payload)
```

### Como funciona

```
View salva no banco
  → render.Partial() renderiza o HTML atualizado
  → fw.Realtime.Replace() envia via WebSocket
  → browser recebe JSON {type:"kyrux:dom", target, html, action}
  → JavaScript injetado localiza [kyrux-target="..."] e atualiza o DOM
```

Zero JavaScript escrito pelo desenvolvedor.

---

## 18. Páginas de Erro

### Comportamento por ambiente

| Ambiente      | 404 / 4xx / 5xx      | Panic / Exceção           |
|---------------|----------------------|---------------------------|
| `production`  | Página de erro estilizada | Página de erro 500   |
| `development` | Página de debug (com rotas registradas) | Página de debug (com stack trace) |

A página de debug é exibida **automaticamente** — não há rota dedicada. Qualquer erro ou panic em `development` a aciona diretamente.

### Acionar erro em uma view

```go
func PostView(ctx *router.Context) {
    post := models.Buscar(ctx.ParamInt("id"))
    if post == nil {
        ctx.Error(404)   // renderiza a página 404
        return
    }
    // ...
}
```

### Personalizar página de erro

```go
import "kyrux/core/errors"

// Em qualquer init() ou Register():
errors.Set(404, func(w http.ResponseWriter, r *http.Request) {
    // Renderize seu próprio template de 404
    // ou retorne JSON para APIs:
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(404)
    json.NewEncoder(w).Encode(map[string]string{"error": "não encontrado"})
})

errors.Set(500, func(w http.ResponseWriter, r *http.Request) {
    render.For("meuapp").Render(/* ... */)
})
```

> Handlers registrados via `errors.Set()` sempre têm prioridade, em qualquer ambiente.

---

## 19. Debug Dashboard

Disponível automaticamente em `APP_ENV=development`:

```
http://localhost:8000/kyrux/debug/
```

Exibe:

| Seção            | Informações                                                                 |
|------------------|-----------------------------------------------------------------------------|
| Aplicação        | Nome, versão, ambiente, endereço, workers, uptime                           |
| Runtime          | Go version, OS/arch, goroutines, heap alocado, heap total, GC cycles        |
| Bancos de Dados  | Cada conexão registrada: nome, driver e status ao vivo (online / offline)   |
| Cache            | Status habilitado/desabilitado e número de entradas ativas                  |
| Fila (Queue)     | Status habilitado/desabilitado e número de tarefas pendentes                |
| Rotas            | Todas as rotas registradas com método e path                                |

### Navegação

- A **página de boas-vindas** (`/`) exibe um botão **⚙ Debug** que leva diretamente ao dashboard.
- O **debug dashboard** exibe um botão **← Início** no header que retorna à rota raiz do projeto.

### Status dos bancos de dados

O status é verificado via `Ping` com timeout de 2 segundos a cada acesso ao dashboard. O indicador visual é:

- `●` verde — conexão respondendo normalmente
- `●` vermelho — conexão inacessível ou timeout

> O debug dashboard só é registrado em `APP_ENV=development`. Em produção a rota não existe.

---

## 20. Admin (Painel de Administração)

Um único painel (`/admin/`) para todos os apps — mas **nenhum model aparece nele
por padrão**. Diferente do admin do Django, o Kyrux não expõe nada
automaticamente: sem `admin.Register`, `/admin/` nem é montado. O layout segue
o mesmo estilo visual da página de boas-vindas do framework (mesma paleta,
header e rodapé) — os templates são embutidos, mas o código é seu: copie e
adapte se quiser uma cara própria.

### Ativar

Dois portões precisam estar abertos — o `.env` (kill-switch global) e o
código (opt-in por model):

```env
ADMIN_ENABLED=true
ADMIN_PATH=/admin/    # opcional — renomear dificulta descoberta por scanners
```

```go
// No Register() do app, depois de importar o model:
import "kyrux/core/admin"

func Register(r *router.Router, fw *bootstrap.Framework) {
    // ...

    admin.Register[models.Produto]("produtos", "Produtos",
        admin.SearchFields("Nome"),        // opcional — campos de busca (LIKE)
        admin.ListFields("Nome", "Preco"), // opcional — colunas da listagem
        admin.Conn("analytics"),           // opcional — outra conexão nomeada
    )
}
```

`Register[T]` exige que o model tenha um campo `kyrux:"pk"` — falha no boot
(panic) se não tiver, junto com slug inválido/duplicado/reservado (`login`,
`logout`). Sem `ListFields`, a listagem mostra todos os campos exceto os
marcados `kyrux:"hash"` (nunca exibidos, em lugar nenhum). Sem `SearchFields`,
a busca fica desativada para aquele model.

### Upload de imagem (`kyrux:"image"`)

Um campo `string` marcado `kyrux:"image"` vira um input de upload no
formulário do admin em vez de um campo de texto — o dev não precisa hospedar
a imagem em outro lugar nem colar URL manualmente:

```go
type Produto struct {
    ID    int64  `kyrux:"pk"`
    Nome  string `kyrux:"size:255"`
    Capa  string `kyrux:"size:500,image"` // vira <input type="file"> no admin
}
```

```go
admin.Register[models.Produto]("produtos", "Produtos",
    admin.App("catalogo"), // obrigatório para todo model com campo image
)
```

- O arquivo é salvo em `medias/<app>/<tabela>/<nome-único>` (na raiz do
  projeto, ao lado de `statics/`) e servido de volta em
  `/medias/<app>/<tabela>/<nome-único>` — o valor gravado no campo já é esse
  caminho pronto para usar em `<img src="...">`, sem helper de template
  adicional.
- `admin.App("nome")` é **obrigatório** em qualquer model com campo
  `kyrux:"image"` — é o que define a pasta de destino do upload; falha no
  boot (panic) se faltar.
- Só aceita `jpeg`, `png`, `gif` e `webp` — validado pelos bytes reais do
  arquivo (`http.DetectContentType`), não pela extensão nem pelo
  Content-Type que o navegador declarou (ambos falsificáveis).
- Nome do arquivo em disco é gerado (prefixo aleatório + nome original
  saneado) — nunca sobrescreve um upload anterior e nunca confia no nome
  enviado pelo navegador para montar o caminho (bloqueia path traversal do
  tipo `../../etc/passwd`).
- Na edição, deixar o campo em branco mantém a imagem atual (mesmo padrão de
  `kyrux:"hash"` para senha) — só troca se um novo arquivo for selecionado.
- `/medias/` nunca lista diretório (mesma proteção de `/statics/`) e recebe
  `Cache-Control: immutable` em produção — seguro porque cada upload tem nome
  único, nunca é sobrescrito no lugar.

### Sem banco configurado

Se `ADMIN_ENABLED=true` e nenhum banco estiver configurado no `.env`, o
comportamento depende do ambiente:

- **`APP_ENV=development`**: o Kyrux cria sozinho um SQLite local em
  `database/kyrux.sqlite3` (pasta já ignorada pelo git), registra-o como
  conexão `"default"` e gera automaticamente a tabela de `auth.User` e a de
  cada model registrado via `admin.Register` — o admin funciona de imediato,
  sem `makemigrations`/`migrate`. É a **única** situação em que o Kyrux abre
  um banco por conta própria; para qualquer banco real, o driver continua
  sendo escolha e responsabilidade sua.
- **`APP_ENV=production`**: nada disso acontece. O Kyrux **nunca** cria um
  SQLite automaticamente em produção — escrever silenciosamente num arquivo
  local efêmero em vez do banco pretendido seria pior que simplesmente
  recusar. O log explica o motivo e o admin não é montado (404).

> O fallback só faz `CREATE TABLE IF NOT EXISTS` — nunca `ALTER`. Se o model
> evoluir (novo campo), configure um banco de verdade e use
> `makemigrations`/`migrate`; o fallback é para começar rápido, não para
> acompanhar mudanças de schema ao longo do tempo.

### Superusuário inicial

Duas formas de criar o primeiro usuário com acesso ao admin:

1. **Interativa** — `go run main.go createsuperuser` (ver [seção 4](#4-cli--comandos)).
2. **Via `.env`** — defina as duas variáveis abaixo; o Kyrux cria a conta
   sozinho no próximo boot, **somente se ainda não existir ninguém com esse
   login**:

```env
ADMIN_SUPERUSER_USERNAME=admin
ADMIN_SUPERUSER_PASSWORD=troque-esta-senha-provisoria
```

- O valor de `ADMIN_SUPERUSER_USERNAME` é gravado no campo marcado com
  `kyrux:"login"` no model `auth.User` — `Username` ou `Email`, conforme a
  tag (ver [seção 15](#15-autenticação)). O nome da variável não muda mesmo
  quando o campo de login é `Email`.
- **Nunca redefine a senha de uma conta já existente** — mesmo que você
  edite `ADMIN_SUPERUSER_PASSWORD` depois e reinicie o servidor (Air
  recompila e reinicia a cada save; sem essa proteção, um `.env` esquecido
  resetaria a senha a cada hot reload). Para trocar a senha depois de criada,
  use o próprio `/admin/` ou `go run main.go createsuperuser`/`createuser`.
- Senha com menos de 8 caracteres é rejeitada (a criação falha e o log
  explica o motivo — nenhuma conta é criada).
- Funciona com qualquer banco, inclusive o SQLite de fallback acima —
  nesse caso a tabela de `auth.User` já existe antes desta checagem rodar.
- Deixe as duas variáveis vazias (padrão) para desativar — nada acontece no
  boot, use `createsuperuser` normalmente.

### Segurança

- **Acesso exige `IsStaff` ou `IsAdmin`** no model `auth.User`, não apenas no
  login. O resultado positivo fica cacheado na sessão por até 5s (evita um
  `SELECT` a cada requisição — medido ~27x mais rápido com cache quente);
  revogar `IsStaff` de um usuário encerra o acesso em até 5s, não mais
  instantaneamente. Pra revogação imediata de verdade, encerre a sessão do
  usuário diretamente (ex: `store.Delete(sessionID)`) em vez de só editar o
  campo no banco.
- **Login válido não implica acesso.** Um usuário sem `IsStaff`/`IsAdmin` que
  autentica corretamente no `/admin/login/` recebe "sua conta não tem
  permissão" e a sessão recém-criada é imediatamente revogada.
- **Brute-force**: o freio embutido em `auth.Login` (10 falhas/minuto por
  conta+IP) já protege o login do admin — nenhuma configuração adicional.
- **CSRF** roda pelo middleware global de sempre — todo formulário do admin
  inclui o token automaticamente.
- **Campos `kyrux:"hash"` nunca são exibidos** — nem mascarados a partir do
  valor real, o valor simplesmente não sai do banco para o template. Na
  edição, o campo de senha fica em branco: preencher define uma nova senha
  (hash automático), deixar em branco mantém a atual.
- **Sem banco disponível, sem admin**: se mesmo após a tentativa de fallback
  (ou em produção, onde não há fallback) a conexão `"default"` não existir, o
  admin recusa montar — não há como protegê-lo sem autenticação, então ele
  simplesmente não sobe (log explica o motivo).
- Toda entrada do usuário (busca, ordenação, paginação) é validada contra os
  metadados reais do model antes de chegar ao SQL — `sort=coluna_falsa` é
  silenciosamente ignorado, nunca vira uma coluna arbitrária na query.

### O que o admin faz e o que não faz

| Recurso | Suporte |
|---|---|
| CRUD completo (criar, listar, editar, excluir) | ✅ |
| Busca textual (`LIKE`) | ✅ — opt-in via `SearchFields` |
| Ordenação por coluna (clique no cabeçalho) | ✅ |
| Paginação | ✅ — Anterior/Próxima, sem total exato (ver nota abaixo) |
| hash/encrypt automático na escrita | ✅ — reaproveita `orm.Create`/`Query.Update` |
| Upload de imagem | ✅ — `kyrux:"image"` + `admin.App("nome")` |
| Múltiplas conexões | ✅ — `admin.Conn("nome")` |
| FK como `<select>` (só ids existentes) | ✅ — automático em qualquer `kyrux:"fk:tabela"`; rótulo via `kyrux:"fklabel:coluna"` |
| Inlines (editar relação dentro do model pai) | ❌ |
| Filtros avançados / actions em lote | ❌ |
| Histórico de alterações | ❌ |

Inlines e filtros avançados ficam de fora por design nesta primeira versão —
o objetivo é um CRUD sólido e seguro sobre um model por vez, não replicar toda
a superfície do Django admin.

### FK como `<select>` (`kyrux:"fklabel:coluna"`)

Todo campo com `kyrux:"fk:tabela"` vira automaticamente um `<select>` no
admin, populado com as linhas que realmente existem em `tabela` — evita
salvar um id que não existe (o que antes só falhava depois, na constraint
do banco). Sem `fklabel`, cada opção mostra o próprio id; com `fklabel`,
mostra a coluna indicada:

```go
type Pedido struct {
    ID         int64  `kyrux:"pk"`
    ClienteID  int64  `kyrux:"column:cliente_id,fk:clientes,fklabel:nome"`
}
```

- A tabela referenciada precisa ter uma coluna `id` (mesma suposição já
  feita pela migration ao gerar `REFERENCES tabela(id)`).
- O `<select>` carrega até 1000 linhas (`ORDER BY` pela própria coluna de
  rótulo) — tabelas maiores continuam funcionais, só não listam tudo.
- Editando um registro cujo valor atual não está mais entre as opções
  carregadas (linha órfã/excluída), o valor continua visível como uma
  opção extra em vez de sumir silenciosamente do formulário.

A listagem usa `PaginateNoCount` internamente (não `Paginate`) — a página
mostra "Página N" e Anterior/Próxima, mas não "N de M" nem o total de
registros. Medido: evitar o `SELECT COUNT(*)` a cada carregamento da lista
é ~40% mais rápido no nível do ORM (benchmark isolado, sem HTTP/sessão no
meio). Se seu caso de uso precisa do total exato, troque para `Paginate` em
`core/admin/crud.go` — é a única linha que muda.

> `admin.Mount`, `admin.EnsureAllTables` e `admin.Count` são exportadas mas de
> uso interno — `bootstrap.Init()` já as chama sozinho na ordem certa (depois
> que os apps registraram seus models, antes de montar as rotas). Código de
> app não precisa (nem deve) chamá-las diretamente; a única API voltada para
> o desenvolvedor é `admin.Register[T]` e suas opções.

O render de cada página do admin (`core/admin/templates.go`) reaproveita o
buffer de saída via `sync.Pool` entre requisições, em vez de alocar um
`bytes.Buffer` novo a cada render — mesmo padrão já usado pelo motor de
templates da aplicação (`core/render`). Medido (microbenchmark isolado,
2026-08-04): ~26% menos memória alocada por render e ~7% menos tempo, sem
nenhuma mudança de comportamento observável.

---

## 21. Fluxo do Sistema

### Fluxo de uma requisição

```
Request
  → Recovery (captura panics)
  → Allowed Hosts (valida o host — production)
  → CSRF (valida token em POST/PUT/PATCH/DELETE)
  → Router (encontra a view)
  → Middleware da view (se houver)
  → View (lógica do desenvolvedor)
    → Service / Model (queries via ORM ou SQL raw)
    → DB / Cache
    → EventBus.Publish()
    → Realtime.Replace() / Append() / Prepend()
  → Render (SSR — renderiza o HTML)
    → Injeta liveScript (WebSocket)
    → Injeta reloadScript (hotreload — apenas em dev)
  → Response
```

### Fluxo de desenvolvimento

```
Arquivo .go alterado
  → Air detecta a mudança
  → Recompila o projeto
  → Reinicia o servidor

Arquivo .html / .css / .js alterado
  → hotreload detecta via inotify
  → Envia evento via SSE (/__kyrux_reload__)
  → Browser recarrega automaticamente
```

### Fluxo de Realtime

```
Usuário A faz POST /posts/criar/
  → View salva no banco
  → render.Partial() renderiza "partials/lista.html"
  → fw.Realtime.Replace("lista-posts", html)
    → JSON enviado via WebSocket para todos os clientes conectados
    → Browser de cada cliente atualiza [kyrux-target="lista-posts"]
  → DOM atualizado sem reload e sem JS manual
```

---

## 22. Performance

Benchmarks medidos com `ab` (HTTP real, TCP, keep-alive) e suite `testing.B` do Go.
Hardware: Intel Core i5-1235U · Go 1.26.2 · Linux · **SERVER_WORKERS=4** (conforme `.env`).

### HTTP real — `ab` (keep-alive, 100 conexões simultâneas)

| Cenário | Req/s | Latência média | Falhas |
|---|---|---|---|
| `GET /ping/` — rota estática | **127.474** | 0,784 ms | 0 |
| `GET /usuarios/42/` — path param | **126.454** | 0,791 ms | 0 |
| `GET /busca/?q=kyrux&page=3` — query string | **109.302** | 0,915 ms | 0 |
| 500 conexões simultâneas (pico) | **99.110** | 5,045 ms | 0 |

Zero falhas em 200.000 requisições totais.

### Latência — percentis (100 conexões, 50k req)

| Percentil | Tempo |
|---|---|
| P50 | 0,89 ms |
| P90 | 1,34 ms |
| P95 | 1,72 ms |
| P99 | 2,44 ms |
| P100 (pior caso) | 6,36 ms |

> Medição pontual num hardware específico — trate como ordem de grandeza, não
> como benchmark oficial reproduzível em qualquer máquina. Para números atuais
> no seu próprio ambiente: `go run main.go benchmark` (roda as 3 camadas de
> teste do framework — microbenchmark, framework sem TCP e throughput real —
> e salva o resultado em `benchmark/`).

### Benchmarks Go nativos — `testing.B` (GOMAXPROCS=4, sem TCP overhead)

| Cenário | ns/op | Req/s estimado | Allocs/op |
|---|---|---|---|
| Rota estática | 1.104 | ~906.000 | 15 |
| Path param | 1.313 | ~762.000 | 18 |
| Query string | 1.715 | ~583.000 | 22 |
| 1 middleware | 878 | ~1.139.000 | 12 |
| 3 middlewares | 908 | ~1.101.000 | 12 |
| Estático paralelo (4 cores) | 923 | ~4.334.000 | 15 |
| Path param paralelo (4 cores) | 697 | ~5.743.000 | 18 |

### Notas

- **+13% no throughput estático e path param** em relação à versão anterior — ganho principal vem do `Content-Length` nas respostas JSON, que elimina o overhead de `chunked transfer encoding` no HTTP/1.1.
- **Degradação sob 500 conexões:** queda de ~14% esperada — com 4 workers, mais conexões competem pelo mesmo pool de goroutines. Aumentar `SERVER_WORKERS` atenua isso.
- **Middlewares têm custo baixo:** 3 middlewares encadeados custam menos de 30 ns a mais que 1 — o chain é compilado uma vez antes do request chegar.
- **Query string cacheada por request:** `ctx.Query()`, `ctx.QueryInt()` etc. parseiam a URL uma única vez — chamadas subsequentes na mesma view reutilizam o resultado.
- **Gargalo esperado:** rotas 404 em `development` são ~9× mais lentas (renderizam o template HTML de debug completo). Em `production`, a página estática é mais rápida.
- **Reflection sem custo no ORM:** metadata de struct é computada uma única vez por tipo e cacheada em `sync.Map` — o hot path não faz nenhuma reflection.

> Medido em localhost com `SERVER_WORKERS=4`, respeitando a configuração do `.env`.
> Resultados variam conforme hardware e carga de trabalho da view.

### Checklist de produção (segurança)

O bootstrap **recusa iniciar** em `APP_ENV=production` se algum segredo estiver
ausente ou for o valor de exemplo. Antes de subir:

1. `APP_ENV=production`.
2. `SECRET_KEY` forte (≥ 32 caracteres). `PASSWORD_PEPPER` definido. Se usar
   campos `kyrux:"encrypt"`, também `FIELD_ENCRYPTION_KEY`. Gere cada um com
   `openssl rand -base64 32`. **Sem `FIELD_ENCRYPTION_KEY` a criptografia de
   campos falha ao gravar** (fail-closed — nunca cifra com chave fraca).
3. `ALLOWED_HOSTS` com os domínios reais (bloqueia Host header forjado).
4. Atrás de proxy reverso com TLS: defina `TRUSTED_PROXY_HEADER=X-Forwarded-For`
   e garanta que o proxy **sobrescreve** esse header (senão o cliente forja o
   IP e escapa do RateLimit). Sem proxy, deixe em branco.
5. Aplique `RateLimit` nas rotas sensíveis. O login já tem freio de brute-force
   embutido (10 falhas/minuto por conta+IP; ajuste com `auth.SetLoginThrottle`).

Em produção, os headers de segurança (HSTS, CSP, X-Frame-Options DENY, nosniff),
o cookie de sessão `Secure` e a página de erro sem stack trace são ativados
automaticamente. A debug page só existe em `development` e apenas para localhost.

### Deploy e HTTP/2

O Kyrux serve HTTP/1.1 (sem TLS próprio). O deploy recomendado é atrás de um
proxy reverso (nginx, Caddy, Traefik) que termina TLS e fala HTTP/2/3 com o
browser — o proxy conversa com o Kyrux em HTTP/1.1 com keep-alive, o que não
limita o throughput. Configure `TRUSTED_PROXY_HEADER` (acima) para o `RateLimit`
e o throttle de login enxergarem o IP real do cliente.

Sobre `SERVER_WORKERS`: **omita em produção** — o runtime Go usa todos os CPUs
por padrão, que é o mais rápido. Defina apenas para limitar consumo de CPU
(ex: máquina compartilhada); valores abaixo do número de CPUs reduzem o throughput.

### Health check

O bootstrap já registra `GET /health` automaticamente — sem tocar banco,
cache ou fila, só confirma que o processo está de pé e respondendo
(`200 ok`, `Cache-Control: no-store`). Pronto para o `HEALTHCHECK` do
Docker ou o probe de liveness de qualquer orquestrador, sem nenhuma
configuração adicional:

```yaml
healthcheck:
  test: ["CMD", "wget", "--spider", "-q", "http://127.0.0.1:8000/health"]
  interval: 30s
  timeout: 5s
  retries: 3
  start_period: 10s
```

### Rodando os testes de performance

Todos os benchmarks ficam em `core/router/benchmark/` e estão organizados em três camadas.

#### Layer 1 — Microbenchmark (custo de registro de rotas)

```bash
go test ./core/router/benchmark/ -bench='^Benchmark(Router|Handle)' -benchmem -benchtime=3s -run='^$'
```

Mede apenas o custo de registrar rotas via API pública — sem request, sem TCP.

#### Layer 2 — Framework benchmark (router + middleware, sem TCP)

```bash
go test ./core/router/benchmark/ -bench='^Benchmark(Route|Middleware|Parallel)' -benchmem -benchtime=3s -run='^$'
```

Usa `httptest.NewRecorder` — elimina overhead de rede. Referência para custo relativo entre tipos de rota e chains de middleware.

#### Layer 2 — Regressão automática

```bash
go test ./core/router/benchmark/ -run TestRegressionCheck -v -count=1
```

Falha se qualquer cenário regredir mais de 5% em relação ao baseline. Atualizar as constantes em `bench_regression_test.go` após otimizações intencionais ou troca de hardware.

#### Layer 3 — Throughput via Go client

```bash
# Router puro (sem bootstrap, sem templates)
go test ./core/router/benchmark/ -run TestThroughputRouter -v -count=1

# Stack completo (bootstrap + apps + templates) — requer ao menos um app com rota GET /
go test ./core/router/benchmark/ -run TestThroughputStack -v -count=1
```

Ambos sobem um servidor real em porta aleatória, disparam requisições por 5 s com múltiplas goroutines e reportam req/s. `TestThroughputStack` força `APP_ENV=production` automaticamente.

> **Nunca usar `./...` para os testes de throughput** — pacotes rodando em paralelo dividem CPU artificialmente e distorcem os resultados.

#### Layer 3 — Throughput via `ab` (capacidade máxima)

```bash
# 1. Subir o servidor de benchmark
go run /tmp/kyrux_bench_server.go &

# 2. Rodar os testes
ab -n 50000 -c 100 -k http://127.0.0.1:8000/ping/
ab -n 50000 -c 100 -k http://127.0.0.1:8000/usuarios/42/
ab -n 50000 -c 100 -k "http://127.0.0.1:8000/busca/?q=kyrux&page=3"
ab -n 50000 -c 500 -k http://127.0.0.1:8000/ping/   # teste de pico

# 3. Encerrar o servidor — SEMPRE ao final
kill $(lsof -ti :8000) 2>/dev/null
```

O servidor de benchmark (`/tmp/kyrux_bench_server.go`) usa `runtime.GOMAXPROCS(4)` e registra três rotas (estática, path param, query string). O template completo está em `.claude/performance_testing.md`.

| Camada | req/s típico | O que mede |
|---|---|---|
| Layer 1 — registro | sub-µs por rota | Custo da primitiva, sem request |
| Layer 2 — framework (`-bench`) | ~433k–592k | Router + handler, sem syscall de rede |
| Layer 2 — regressão | ~620k–1.2M | Mesmo código, contexto mais aquecido |
| Layer 3 — Go client (`TestThroughputReal`) | ~2k (rota com banco) a ~60k (rota sem banco) | Throughput real com overhead de `net/http`, autenticação de sessão e ORM nas rotas que usam banco |
| Layer 3 — `ab` | ~120k–220k | Capacidade máxima com cliente C otimizado |

Não comparar números entre camadas — cada uma mede uma coisa diferente.

> **2026-08-04:** o cliente de `TestThroughputReal` fechava o body da
> resposta sem drenar o conteúdo antes — isso impede o `net/http` de
> reaproveitar a conexão via keep-alive, forçando uma conexão TCP nova a
> cada requisição (competindo por CPU com o próprio servidor, já que
> cliente e servidor rodam no mesmo processo/`GOMAXPROCS`). Corrigido com
> `io.Copy(io.Discard, resp.Body)` antes do `Close()`. O intervalo acima já
> reflete o cliente corrigido — antes da correção, as rotas sem gargalo de
> banco chegavam a medir de 2x a 4x mais lento do que o framework
> realmente entrega (o handler nunca foi o gargalo; o cliente de teste é
> que media errado).

---

## 23. MongoDB (NoSQL)

MongoDB **não passa pelo ORM relacional** (`core/orm`, `Query[T]`) — e não vai
passar, de propósito. O ORM é um construtor de SQL: `Where` gera uma cláusula
SQL crua, `Join` gera `INNER JOIN`, migrations geram `CREATE TABLE`. MongoDB
não tem esse modelo — são documentos BSON em coleções, filtrados por
**documentos** (`mongo.M{"idade": mongo.M{"$gt": 18}}`), sem tabelas, colunas
ou JOIN nativo. Forçar isso dentro do `Query[T]` fingiria uma portabilidade
que não existe. Em vez disso, `core/nosql/mongo` é um client dedicado e
idiomático.

### Não é wireado automaticamente — e isso é proposital

Diferente de Cache/Queue, **não existe `fw.Mongo`, `MONGO_ENABLED` nem painel
no debug dashboard**. `core/bootstrap` não importa `core/nosql/mongo` em
lugar nenhum — assim como o Kyrux não importa nenhum driver relacional por
conta própria (`import _ "github.com/lib/pq"` é responsabilidade sua), o
client MongoDB só entra no binário se **você** importar `kyrux/core/nosql/mongo`
em algum lugar do seu código.

Isso importa de verdade: o driver oficial (`go.mongodb.org/mongo-driver/v2`)
sozinho adiciona **~3,5 MB (~13%)** ao binário compilado, mesmo sem nenhuma
conexão aberta — só pelo código de BSON, autenticação SCRAM e compressão ser
linkado. Um projeto que nunca usa MongoDB não deve pagar esse custo. Medido
comparando o binário com e sem o import neste framework.

### Usar

```go
import (
    "context"
    "kyrux/core/environment"
    "kyrux/core/nosql/mongo"
)

type Produto struct {
    Nome  string  `bson:"nome"`
    Preco float64 `bson:"preco"`
    Ativo bool    `bson:"ativo"`
}

// Construa e guarde o client você mesmo — no Register() do seu app, por
// exemplo, ou num pacote próprio de infra. environment.Get já está
// disponível (mesmo pacote usado pelo core/settings internamente).
var mongoClient *mongo.Client

func Register(r *router.Router, fw *bootstrap.Framework) {
    mc, err := mongo.New(environment.Get("MONGO_URI"), environment.Get("MONGO_DATABASE"))
    if err != nil {
        log.Fatalf("mongo: %v", err) // ou trate como preferir — a decisão é sua
    }
    mongoClient = mc
}

produtos := mongo.CollectionOf[Produto](mongoClient, "produtos")

// Inserir
err := produtos.InsertOne(ctx, &Produto{Nome: "Caneca", Preco: 29.9, Ativo: true})

// Buscar (mongo.M é só um map[string]any — não precisa de um segundo import)
lista, err := produtos.Find(ctx, mongo.M{"ativo": true})
um, err := produtos.FindOne(ctx, mongo.M{"nome": "Caneca"}) // mongo.ErrNoDocuments se não achar

// Atualizar — MongoDB exige operador ($set), diferente do UPDATE SQL:
// um update sem operador FALHA, não substitui o documento inteiro.
n, err := produtos.UpdateOne(ctx, mongo.M{"nome": "Caneca"}, mongo.M{"$set": mongo.M{"preco": 39.9}})

// Remover
n, err = produtos.DeleteMany(ctx, mongo.M{"ativo": false})

// Contar
total, err := produtos.Count(ctx, mongo.M{})
```

`MONGO_URI`/`MONGO_DATABASE` acima são só uma sugestão de nome de variável
(o Kyrux não lê nem valida essas chaves) — use o nome que quiser no seu
`.env`, já que a leitura é toda sua via `environment.Get`.

Todos os métodos recebem `context.Context` explícito (diferente do ORM
relacional) — operações de rede contra o MongoDB são canceláveis/têm timeout
por chamada. Feche a conexão você mesmo no shutdown (`mongoClient.Close(ctx)`)
— o bootstrap não sabe que ela existe, então não vai fechar por você.

### Tags `bson:"..."`

O nome do campo no documento é controlado por tags `bson`, exatamente como
`encoding/json` — sem relação com as tags `kyrux:"..."` do ORM relacional
(que não se aplicam aqui: não existe `kyrux:"pk"`, `kyrux:"unique"` etc. para
documentos).

### Escape hatch

`client.Raw()` devolve o `*mongo.Client` nativo do driver oficial, e
`coll.Raw()` devolve a `*mongo.Collection` nativa — para agregações,
transações, change streams e qualquer coisa que este wrapper não cobre.

---

## 24. Redis como banco (NoSQL)

**Não confunda com `fw.Cache`/`fw.Queue`** — aqueles já usam Redis
internamente, mas só como chave/valor com TTL e fila, respectivamente
(`CACHE_DRIVER=redis`/`QUEUE_DRIVER=redis`). `core/nosql/redis` expõe as
**estruturas de dados reais do Redis** — hash, lista, conjunto, conjunto
ordenado, pub/sub — para usar Redis como banco de propósito geral, não só
como cache.

Mesmo padrão do MongoDB: **não é importado por `core/bootstrap`**, não tem
`fw.Redis` nem variável de ambiente lida automaticamente. Você importa
`kyrux/core/nosql/redis` no seu próprio código, só se for usar. Diferença
prática: se seu projeto já usa `CACHE_DRIVER=redis` ou `QUEUE_DRIVER=redis`,
o `go-redis` já está no binário por causa deles — usar este pacote também
não adiciona peso extra, é a mesma dependência reaproveitada.

### Usar

```go
import "kyrux/core/nosql/redis"

rc, err := redis.New("localhost:6379", "", 0) // addr, password, db lógico (0-15)
defer rc.Close()

ctx := context.Background()

// String simples (sem TTL obrigatório, diferente do fw.Cache)
rc.Set(ctx, "sessao:abc", "user_id=42", time.Hour)
v, err := rc.Get(ctx, "sessao:abc") // redis.ErrNil se não existir

// Valores estruturados via JSON
rc.SetJSON(ctx, "produto:1", Produto{Nome: "Caneca", Preco: 29.9}, 0)
var p Produto
rc.GetJSON(ctx, "produto:1", &p)

// Hash — campos de um "objeto"
rc.HSet(ctx, "user:1", map[string]any{"nome": "Ana", "idade": 30})
nome, _ := rc.HGet(ctx, "user:1", "nome")
todos, _ := rc.HGetAll(ctx, "user:1")

// Lista
rc.RPush(ctx, "fila:emails", "a@x.com", "b@x.com")
itens, _ := rc.LRange(ctx, "fila:emails", 0, -1)

// Conjunto
rc.SAdd(ctx, "tags:post:1", "go", "web")
membros, _ := rc.SMembers(ctx, "tags:post:1")

// Conjunto ordenado — ranking/leaderboard
rc.ZAdd(ctx, "ranking", redis.ZMember{Score: 100, Member: "jogador1"})
top10, _ := rc.ZRange(ctx, "ranking", 0, 9)

// Pub/Sub
sub := rc.Subscribe(ctx, "notificacoes")
defer sub.Close()
for msg := range sub.Channel() {
    fmt.Println(msg.Payload)
}
```

Todos os métodos recebem `context.Context` explícito. `redis.ErrNil` é
devolvido quando uma chave/campo não existe (equivalente ao `sql.ErrNoRows`
do ORM e ao `mongo.ErrNoDocuments` do client Mongo).

### Escape hatch

`client.Raw()` devolve o `*redis.Client` nativo do driver oficial
(`github.com/redis/go-redis/v9`) — para scripts Lua, pipelines e
transações (`MULTI`/`EXEC`) que este wrapper não cobre.

---

## 25. Cassandra (NoSQL)

CQL (Cassandra Query Language) parece SQL, mas tem uma restrição
fundamental que o diferencia de qualquer banco relacional: `WHERE` só
filtra eficientemente pela **partition key** (e clustering columns) — um
`WHERE` numa coluna qualquer exige `ALLOW FILTERING`, que faz o cluster
inteiro varrer todos os nós (anti-padrão, nunca gerado por este pacote).
Não existe `JOIN`. Migrations no sentido do ORM relacional também não
existem — schema é CQL aplicado manualmente. `core/nosql/cassandra` é
honesto sobre essas restrições em vez de fingir que não existem.

Mesmo padrão dos outros: **não é importado por `core/bootstrap`** — o
driver oficial (`github.com/gocql/gocql`) só entra no binário se você
mesmo importar `kyrux/core/nosql/cassandra`.

### Usar

```go
import "kyrux/core/nosql/cassandra"

c, err := cassandra.New([]string{"127.0.0.1"}, "meu_keyspace")
defer c.Close()

ctx := context.Background()

// DDL/DML sem retorno de linhas
err = c.Exec(ctx, `CREATE TABLE IF NOT EXISTS produtos (
    id uuid PRIMARY KEY, nome text, preco double
)`)
err = c.Exec(ctx, "INSERT INTO produtos (id, nome, preco) VALUES (?, ?, ?)", id, "Caneca", 29.9)
err = c.Exec(ctx, "DELETE FROM produtos WHERE id = ?", id)

// Ler como map — sempre correto, sem conversão de tipos
rows, err := c.SelectMap(ctx, "SELECT nome, preco FROM produtos WHERE id = ?", id)

// Ler decodificado num struct (via tags `json:"..."`) — ergonômico para
// tipos comuns (texto, número, booleano, timestamp); para uuid/list/set/map
// nativos do Cassandra, prefira SelectMap ou Raw().Query(...).Scan direto.
type Produto struct {
    Nome  string  `json:"nome"`
    Preco float64 `json:"preco"`
}
produtos, err := cassandra.Select[Produto](ctx, c, "SELECT nome, preco FROM produtos WHERE id = ?", id)
```

`WHERE` numa coluna que não é a partition key (sem índice, sem `ALLOW
FILTERING`) é rejeitado pelo próprio Cassandra com erro — não é algo que
este wrapper poderia (ou deveria) contornar.

### Escape hatch

`client.Raw()` devolve a `*gocql.Session` nativa do driver oficial — para
batches, paginação manual (`PageState`), políticas de retry customizadas e
qualquer coisa que este wrapper não cobre.

---

## 26. Elasticsearch (NoSQL)

Elasticsearch é um motor de busca/documentos, não um banco transacional:
índices sem schema fixo (mapping dinâmico), consultas via **Query DSL em
JSON** (não SQL), sem `JOIN`, sem transações ACID, e busca em **quase tempo
real** — um documento gravado só aparece numa busca depois do próximo
refresh do índice (padrão ~1s; por isso `Index[T].Refresh()` existe, para
testes e casos onde você precisa ver o documento imediatamente).

Mesmo padrão dos outros: **não é importado por `core/bootstrap`** — o
driver oficial (`github.com/elastic/go-elasticsearch/v8`) só entra no
binário se você mesmo importar `kyrux/core/nosql/elasticsearch`.

### Usar

```go
import "kyrux/core/nosql/elasticsearch"

c, err := elasticsearch.New([]string{"http://localhost:9200"})

type Artigo struct {
    Titulo   string `json:"titulo"`
    Conteudo string `json:"conteudo"`
    Ativo    bool   `json:"ativo"`
}

artigos := elasticsearch.IndexOf[Artigo](c, "artigos")
ctx := context.Background()

// Gravar (id vazio = Elasticsearch gera um ID automaticamente)
id, err := artigos.Put(ctx, "1", &Artigo{Titulo: "Kyrux", Conteudo: "framework em Go", Ativo: true})

// Buscar por ID
doc, found, err := artigos.Get(ctx, "1")

// Buscar por Query DSL (documento JSON — map[string]any já resolve a
// maioria dos casos, sem precisar de outro import)
resultados, err := artigos.Search(ctx, map[string]any{
    "query": map[string]any{
        "match": map[string]any{"conteudo": "golang"},
    },
})

// Contar
total, err := artigos.Count(ctx, nil) // nil = conta o índice inteiro

// Remover
err = artigos.Delete(ctx, "1")

// Só em testes / quando precisar ver a gravação imediatamente:
err = artigos.Refresh(ctx)
```

### Escape hatch

`client.Raw()` devolve o `*elasticsearch.Client` nativo do driver oficial —
para agregações, bulk API, ILM (index lifecycle management) e qualquer
coisa que este wrapper não cobre.

---

## 27. DynamoDB (NoSQL)

DynamoDB não tem linguagem de query — é uma API de operações sobre uma
chave primária obrigatória (partition key, + sort key opcional):
`GetItem` exige a chave exata; `Query` filtra pela partition key (e uma
condição na sort key); qualquer outro filtro exige `Scan` — varredura da
tabela inteira, cara e lenta — ou um Global Secondary Index criado
antecipadamente. Sem `JOIN`. `core/nosql/dynamodb` reflete essas operações
tal como existem, em vez de fingir um `WHERE` genérico.

Mesmo padrão dos outros: **não é importado por `core/bootstrap`**. Vale um
destaque a mais aqui: o SDK oficial da AWS (`aws-sdk-go-v2`) é uma
dependência pesada de verdade — medimos **~5,4 MB (~20%)** de aumento no
binário quando o client é efetivamente usado (não só importado — a
eliminação de código morto do linker Go some com boa parte do peso de um
import nunca chamado, então o número real só aparece quando você constrói
um client de fato). Ainda mais motivo pra só importar se for usar.

### Ativar

```go
import "kyrux/core/nosql/dynamodb"

// endpoint vazio = AWS de verdade, credenciais pela cadeia padrão do SDK
// (env vars, ~/.aws/credentials, role IAM). endpoint não-vazio (ex:
// DynamoDB Local em dev) exige accessKey/secretKey explícitos — o serviço
// não valida essas credenciais, mas o SDK exige que existam estruturalmente.
c, err := dynamodb.New(ctx, "us-east-1", "http://localhost:8000", "dummy", "dummy")
```

### Usar

```go
type Produto struct {
    PK    string  `dynamodbav:"pk"`
    SK    string  `dynamodbav:"sk"`
    Nome  string  `dynamodbav:"nome"`
    Preco float64 `dynamodbav:"preco"`
}

produtos := dynamodb.TableOf[Produto](c, "produtos")

// Gravar (cria ou substitui o item inteiro)
err := produtos.Put(ctx, &Produto{PK: "produto#1", SK: "meta", Nome: "Caneca", Preco: 29.9})

// Buscar pela chave primária exata
p, found, err := produtos.Get(ctx, map[string]any{"pk": "produto#1", "sk": "meta"})

// Buscar pela partition key (+ condição opcional na sort key) — a forma
// eficiente de filtrar, sem Scan
itens, err := produtos.Query(ctx, "pk = :pk", map[string]any{":pk": "produto#1"})

// Remover
err = produtos.Delete(ctx, map[string]any{"pk": "produto#1", "sk": "meta"})

// Varredura completa — caro, evite em tabelas grandes ou caminho quente
todos, err := produtos.Scan(ctx)
```

### Escape hatch

`client.Raw()` devolve o `*dynamodb.Client` nativo do SDK oficial — para
transações (`TransactWriteItems`), batch operations, streams e qualquer
coisa que este wrapper não cobre.

---

## 28. Core (fundação modular) — experimental

`kyrux/core` é uma camada **adicional** para registrar, ativar e orquestrar
módulos plugáveis (bancos, caches, e no futuro filas/storage/protocolos) de
forma uniforme. É experimental e **não substitui nada**: `bootstrap.Init`,
`Framework`, `fw.DB`/`fw.Cache`/`fw.Queue` e todo o resto continuam
funcionando exatamente como antes, sem precisar tocar em uma linha para
adotar (ou ignorar) este pacote.

Esta é a fundação (fase 1 de um plano maior — ver Referência Rápida abaixo
para o que ainda falta): `Module`, um Registry com self-registro via `init()`
(hot-plug: importar o pacote do adapter já disponibiliza o módulo, sem o
`core` nunca importar o adapter em si), um Container de DI simples (nomeado,
não por tipo — permite múltiplas instâncias do mesmo tipo, ex: dois bancos
Postgres coexistindo) e um Lifecycle que orquestra Init→Configure (na
ativação) e Start/Shutdown (em lote, este último em ordem reversa) —
publicando eventos de fase (`lifecycle.BeforeInit`, `AfterInit`,
`BeforeStart`, `AfterStart`, `BeforeShutdown`, `AfterShutdown`) no mesmo
`core/events.Bus` já existente, sempre de forma assíncrona/best-effort
(a orquestração de verdade — ordem, propagação de erro — é síncrona dentro
do próprio `core/lifecycle`, não pelo Bus).

Provado com **quinze adapters reais**, todos com teste de ponta a ponta
real contra infraestrutura de verdade (Postgres, Kafka, RabbitMQ, NATS,
MinIO, Mailpit, LocalStack, um OpenTelemetry Collector real — nunca mock
do próprio código):

| Categoria | Adapters | Sugar em `core.X` |
|---|---|---|
| Cache | `cachememory` | `core.Cache.Memory()` |
| Banco SQL | `sqlpostgres` | `core.Database.SQL.Postgres()` |
| API | `restapi`, `apigraphql`, `apigrpc`, `soapclient` (+ `core/soap` p/ servidor) | só REST e SOAPClient |
| Fila | `kafka`, `rabbitmq`, `nats` | nenhum |
| Storage | `s3` (S3 real ou MinIO, endpoint configurável) | nenhum |
| Auth | `oauth2` | `core.Auth.OAuth2()` |
| Mail | `smtp` (STARTTLS ou SMTPS/465, automático pela porta), `sendgrid`, `ses` | só SMTP |
| Observabilidade | `prometheus`, `opentelemetry` | nenhum |

Vários coexistem ao mesmo tempo no mesmo `Core` sem qualquer limitação de
uso conjunto (ver `TestCoreCacheEPostgresCoexistindo`,
`TestCoreAPIGRPCERESTCoexistindo`, `TestCoreAPISOAPClienteEServidorReais`).

O caminho STARTTLS do adapter `smtp` é testado via `core.Mail.SMTP` contra
um Mailpit real (`core_mail_smtp_test.go`); o caminho SMTPS/TLS implícito
(porta 465) é testado em `core/adapters/smtp/smtp_test.go` contra um
servidor TLS/SMTP real em loopback (handshake TLS e protocolo SMTP de
verdade — não é Mailpit, que não expõe SMTPS, mas também não é mock do
próprio código: é um peer real na outra ponta do socket).

### Ativar módulos

```go
import (
    "context"
    "kyrux/core"
    "kyrux/core/mail"
    _ "kyrux/core/adapters/cachememory" // hot-plug: só entra no binário se importado
    _ "github.com/lib/pq"               // driver Postgres — o Core nunca importa drivers
)

c := core.New()

cache, err := c.Cache.Memory()
db, err := c.Database.SQL.Postgres("principal", "postgres://user:pass@localhost/app?sslmode=disable")

// useTLS ativa STARTTLS fora da porta 465 (ex: 587). Na porta 465, o
// client já usa TLS implícito (SMTPS) sozinho — a flag é ignorada.
mailer, err := c.Mail.SMTP("principal", "smtp.exemplo.com.br", "465", "user@exemplo.com.br", "senha", false)
err = mailer.Send(context.Background(), mail.Message{From: "user@exemplo.com.br", To: []string{"destino@exemplo.com.br"}, Subject: "Olá", Text: "corpo"})

// core.API.REST reaproveita o core/router — devolve o Router pra registrar rotas
api, err := c.API.REST("127.0.0.1:8081")
api.Handle("GET /status", func(ctx *router.Context) {
    ctx.JSON(200, map[string]string{"status": "ok"})
})

// Start em lote (na ordem de ativação) e Shutdown em lote (ordem reversa)
if err := c.Run(); err != nil { /* ... */ }
defer c.Shutdown()

// Buscar uma conexão ativada em outro lugar da aplicação, pelo nome usado:
db2, ok := c.Database.Get("principal")
```

### Hot-plug para módulos sem parâmetros

Qualquer módulo parametrizado é construído diretamente pelo seu próprio
pacote e ativado com `core.UseModule`; módulos sem parâmetros (config lida
de variáveis de ambiente dentro do próprio adapter, por exemplo) podem se
autorregistrar por nome e ser ativados sem o `core` conhecer o adapter:

```go
// No pacote do seu adapter:
func init() {
    registry.Register("meu.modulo", func() registry.Module { return &MeuAdapter{} })
}

// Em qualquer lugar da aplicação, depois de importar (mesmo que só com _) o pacote acima:
valor, err := core.Use[*MeuTipo](c, "meu.modulo", "chave-no-container")
```

### Adapters pesados — sem sugar em `core.X`, ative com `core.UseModule`

`core.API.REST()`, `core.Database.SQL.Postgres()`, `core.Auth.OAuth2()`,
`core.Mail.SMTP()` e `core.API.SOAPClient()` existem como métodos porque
suas dependências já são obrigatórias do framework (`core/router`,
`core/database`) ou puramente stdlib (`golang.org/x/oauth2`, `net/smtp`,
`encoding/xml`) — importar de dentro de `kyrux/core` não pesa nada a mais.

Todo o resto — GraphQL, gRPC, Kafka, RabbitMQ, NATS, S3, SendGrid, SES,
Prometheus, OpenTelemetry — **não** tem esse privilégio: são dependências
reais que a maioria dos projetos Kyrux nunca vai usar, então `kyrux/core`
nunca as importa (confirmado via `go list -deps kyrux/core` no CI mental
de cada um). Importe o adapter você mesmo e ative com o `core.UseModule`
genérico, exatamente como qualquer client NoSQL:

```go
import (
    "kyrux/core"
    "kyrux/core/adapters/kafka"
)

client, err := core.UseModule[*kafka.Client](c, kafka.New("localhost:9092"), "queue.kafka.principal")
w := client.Producer("pedidos")
w.WriteMessages(ctx, kafkago.Message{Value: []byte("...")})
r := client.Consumer("pedidos", "meu-grupo")
```

```go
import (
    "kyrux/core"
    "kyrux/core/adapters/s3"
)

// endpoint vazio = AWS S3 real; endpoint não-vazio (ex: MinIO) exige accessKey/secretKey
storage, err := core.UseModule[*s3.Client](c, s3adapter.New("principal", "us-east-1", "http://localhost:9000", "minioadmin", "minioadmin"), "storage.s3.principal")
bucket := storage.Bucket("meu-bucket")
err = bucket.Put(ctx, "foto.jpg", arquivo, "image/jpeg")
```

```go
import (
    "kyrux/core"
    "kyrux/core/adapters/sendgrid"
    "kyrux/core/mail"
)

client, err := core.UseModule[*sendgrid.Client](c, sendgrid.New("principal", "SG.xxx"), "mail.sendgrid.principal")
err = client.Send(ctx, mail.Message{From: "...", To: []string{"..."}, Subject: "...", Text: "..."})
// core/adapters/ses tem a mesma assinatura Send(ctx, mail.Message) — trocar de provedor
// (SMTP/SendGrid/SES) não exige reescrever quem monta o e-mail (mail.Sender)
```

```go
import (
    "kyrux/core"
    "kyrux/core/adapters/prometheus"
)

metrics, err := core.UseModule[*prometheus.Client](c, prometheusadapter.New("principal"), "observability.prometheus.principal")
metrics.MustRegister(meuContador)
api, _ := c.API.REST(addr)
api.HandlePrefix("/metrics", metrics.Handler())
```

RabbitMQ (`core/adapters/rabbitmq`), NATS (`core/adapters/nats`), gRPC
(`core/adapters/apigrpc`), GraphQL (`core/adapters/apigraphql`) e
OpenTelemetry (`core/adapters/opentelemetry`) seguem o mesmo padrão —
`New(...)` + `core.UseModule` — ver o comentário de pacote de cada um para
os detalhes da assinatura.

### SOAP — cliente e servidor sem nenhuma lib de terceiros

Não existe uma biblioteca SOAP amplamente adotada e mantida em Go — como o
protocolo em si é só XML bem definido sobre HTTP, `core/soap` implementa
os dois lados direto sobre `encoding/xml`/`net/http` da stdlib (custo
adicional zero). Uso típico: integração com webservices legados/governo
(SEFAZ/NFe, INSS, etc.).

Cliente, com sugar (`core.API.SOAPClient`, custo zero):

```go
client, err := c.API.SOAPClient("sefaz", "https://homologacao.sefaz.sp.gov.br/ws/...")
var resp MinhaResposta
err = client.Call(ctx, "ConsultarNFe", MinhaRequisicao{...}, &resp)
// erro do servidor (<soap:Fault>) chega como *soap.Fault
```

Servidor — sem sugar dedicada; é um `http.Handler` comum, monte no router
de `core.API.REST` do mesmo jeito que o `/metrics` do Prometheus:

```go
server := soap.NewServer()
server.Handle("MinhaOperacaoRequest", func(ctx context.Context, requestXML []byte) ([]byte, error) {
    var req MinhaOperacaoRequest
    xml.Unmarshal(requestXML, &req)
    return xml.Marshal(MinhaOperacaoResponse{...})
})

api, _ := c.API.REST(addr)
api.HandlePrefix("/soap", server)
```

### O que ainda não existe (roadmap, não implementado)

Ficam para fases seguintes: uma reorganização completa e opcional de
`core/` (namespaces adicionais tipo `core.Queue.Kafka()` como sugar, se um
dia fizer sentido relaxar a disciplina de peso pra conveniência), suporte
a múltiplos exporters OpenTelemetry prontos (hoje só OTLP/HTTP — Jaeger
nativo, Zipkin, etc. exigiriam adapters próprios), e qualquer protocolo
novo que surgir (ex: MQTT, WebRTC) — cada um, quando vier, segue a mesma
regra: só pesa no binário de quem importar.

---

## 29. Mail (fw.Mail)

Serviço de e-mail de primeira classe, igual `fw.DB`/`fw.Cache`/`fw.Queue`
— conectado automaticamente no boot a partir de `MAIL_*` no `.env` (ver
seção 3), disponível em qualquer app sem nenhuma configuração extra.

### Ativar

```env
MAIL_ENABLED=true
MAIL_HOST=smtp.exemplo.com.br
MAIL_PORT=465            # 465 = TLS implícito (SMTPS) automático; outra porta = STARTTLS
MAIL_USER=no-reply@exemplo.com.br
MAIL_PASSWORD=sua-senha
```

`fw.Mail` fica `nil` se `MAIL_ENABLED=false`, se `MAIL_HOST`/`MAIL_USER`
estiverem vazios, ou se o servidor SMTP estiver inacessível no boot — ao
contrário de Cache/Queue (que sempre caem para um fallback em memória),
não existe "envio de e-mail" que funcione sem servidor de verdade. Sempre
cheque `fw.Mail != nil` antes de usar.

### Enviar

```go
import "kyrux/core/mail"

err := fw.Mail.Send(ctx.Request.Context(), mail.Message{
    From:    "contato@meuapp.com",
    ReplyTo: "visitante@example.com", // opcional
    To:      []string{"destino@example.com"},
    Cc:      []string{"copia@example.com"},  // opcional
    Bcc:     []string{"oculta@example.com"}, // opcional
    Subject: "Assunto",
    Text:    "Corpo em texto puro",
    HTML:    "<p>Corpo em HTML</p>", // opcional — se vazio, envia só Text
    Attachments: []mail.Attachment{
        {Filename: "nota.pdf", Content: pdfBytes, ContentType: "application/pdf"},
    },
})
```

### Assíncrono via Queue

Se `QUEUE_ENABLED=true`, `fw.Mail.Send()` **enfileira** a mensagem em
`fw.Queue` e devolve o controle quase na hora — a entrega de verdade
acontece depois, num worker (pool, retry com backoff automático em falha
transitória, drenagem no shutdown). Sem isso, a requisição HTTP ficaria
presa esperando a ida e volta de uma sessão SMTP, que pode levar segundos.

Sem `QUEUE_ENABLED=true`, `Send` é síncrono — o erro devolvido já é o de
entrega de verdade. Com fila, o erro devolvido por `Send` é só de
**enfileirar** (fila cheia, fila encerrada); falhas de entrega acontecem
assíncronas e só aparecem nos logs e no retry automático da fila, nunca de
volta pro caller.

Implementado em `core/mail.Queued` (`core/mail/queued.go`) — reaproveita
`core/queue` sem nenhuma dependência nova; funciona também com
`QUEUE_DRIVER=redis` (payload serializado em JSON entre réplicas).

### Trocar de provedor

`fw.Mail` é montado sobre `core/adapters/smtp` no bootstrap. Pra
SendGrid/SES (sem sugar automático — SDK de terceiros), monte você mesmo
com `core.UseModule` e `mail.NewQueued(sender, fw.Queue)` — ver seção 28.

---

## 30. Captcha (core/security/captcha)

Desafio visual simples (código numérico distorcido, renderizado como PNG)
pra formulários públicos — sem depender de nenhum serviço externo (Google
reCAPTCHA, hCaptcha, etc). Zero configuração de conta, zero chave de API,
zero chamada de rede pro navegador do visitante bloquear ou atrasar.

### Uso pronto — `captcha.Store`

```go
import "kyrux/core/security/captcha"

captchaStore := captcha.NewStore(fw.Sessions)

router.Path("GET", "/captcha/image", captchaStore.ImageHandler(), "captcha_image"),

// no handler de POST do formulário:
if !captchaStore.Verify(ctx, ctx.Request.FormValue("captcha_answer")) {
    // código incorreto — mostre erro e não processe o formulário
}
```

No template:

```html
<img src="{{ url "captcha_image" }}" alt="Código de verificação" id="captchaImg">
<input type="text" name="captcha_answer" maxlength="5" inputmode="numeric" required>
```

`ImageHandler` gera um código novo a cada requisição (útil pra um "gerar
outro" no front — troque o `src` da imagem com um cache-buster, ex:
`?t=Date.now()`) e guarda na sessão do visitante, substituindo o anterior.
`Verify` confere a resposta contra o código da sessão e o consome (uso
único) independente do resultado — sempre precisa de uma imagem nova pra
uma nova tentativa.

### Primitivas de baixo nível

Pra guardar o código em outro lugar que não sessão (ex: banco, num fluxo
sem cookie):

```go
code, err := captcha.New()      // código aleatório de captcha.CodeLength dígitos
png, err := captcha.PNG(code)   // imagem PNG distorcida do código
```

### Como funciona a imagem

Fonte bitmap 5x7 própria (só dígitos 0-9, sem ambiguidade visual tipo 0/O
ou 1/I/l), desenhada com `image`/`image/png` da stdlib — sem
`golang.org/x/image` nem nenhuma outra dependência de terceiros. Ruído de
fundo (linhas e pontos aleatórios) e leve inclinação por caractere
dificultam OCR automatizado simples.

---

## Referência Rápida

### Context — todos os métodos

```go
// Path params
ctx.Param("nome")              // string
ctx.ParamInt("id")             // (int, bool)

// Query string
ctx.Query("q")                 // string
ctx.QueryDefault("order","asc")// string com fallback
ctx.QueryInt("page", 1)        // int com fallback
ctx.QueryAll("tag")            // []string

// Respostas
ctx.JSON(status, v)
ctx.HTML(status, html)
ctx.Redirect(url, status)
ctx.Error(code)

// Dados internos
ctx.Set("chave", valor)
ctx.Get("chave")               // (any, bool)

// Acesso direto
ctx.Request                    // *http.Request
ctx.Writer                     // http.ResponseWriter
ctx.Params                     // map[string]string
```

### ORM — todos os métodos

```go
// Leitura
orm.FromDB[T](db).All()                          // ([]T, error)
orm.FromDB[T](db).Each(func(t *T) error {...})   // streaming, memória O(1)
orm.FromDB[T](db).First()                        // (*T, error) — sql.ErrNoRows se vazio
orm.FromDB[T](db).Last()                         // (*T, error) — ordenação invertida
orm.FromDB[T](db).Exists()                       // (bool, error)
orm.FromDB[T](db).Count()                        // (int64, error)
orm.FromDB[T](db).Sum("col")                     // (float64, error) — também Avg/Min/Max

// Filtros tipados (retornam *Query[T]) — preferidos: coluna validada
.WhereEq("col", val)          // col = ?      (também OrWhereEq)
.WhereNe("col", val)          // col <> ?
.WhereGt/.WhereGte/.WhereLt/.WhereLte("col", val)  // col >, >=, <, <= ?
.WhereLike("col", "%termo%")  // col LIKE ?   (também OrWhereLike)
.WhereNull("col") / .WhereNotNull("col")

// Filtros em SQL livre — nunca concatene valores na string, só em args
.Where("col = ?", val)        // idêntico a WhereSQL
.OrWhere("col = ?", val)      // idêntico a OrWhereSQL
.WhereIn("id", ids)           // expande slice em placeholders (máx. 5000 valores)
.Join("users", "users.id = posts.user_id")     // filtro por relação
.LeftJoin("users", "users.id = posts.user_id")
.Search("conteudo", "termo")  // full-text (campo kyrux:"fts") — já ordena por relevância
.OrderBy("col DESC", "id")    // múltiplas colunas — ou orm.Desc("col")/orm.Asc("col")
.Select("id", "titulo")       // restringe colunas do SELECT (padrão: *)
.Distinct()
.Limit(n)
.Offset(n)

// Relações (carregamento sem N+1)
orm.Prefetch[Comentario](db, "post_id", ids, func(c *Comentario) int64 { return c.PostID })
// → (map[int64][]Comentario, error)

// Paginação
orm.FromDB[T](db).Where(...).OrderBy("id DESC").Paginate(page, 20)     // pageSize máx. 1000
orm.FromDB[T](db).OrderBy("id DESC").PaginateNoCount(page, 20)         // sem COUNT(*)
// → (Page[T], error)
// Page[T]: Items, Total, Page, PageSize, TotalPages, HasNext, HasPrev

orm.FromDB[T](db).PaginateAfter("id", cursor, false, 20) // keyset — custo não cresce com a página
// → (KeysetPage[T], error) — KeysetPage[T]: Items, NextCursor, HasNext

// Escrita
orm.Create(db, &model)                         // error — preenche PK
orm.CreateAll(db, []*T{...})                   // bulk: um INSERT multi-VALUES
orm.FromDB[T](db).Where(...).Update(map[string]any{...}) // error
orm.FromDB[T](db).Where(...).Delete()            // error
orm.FromDB[T](db).Where(...).GetOrCreate(&defaults)      // (*T, created, error) — recupera de corrida via UNIQUE
orm.FromDB[T](db).Where(...).UpdateOrCreate(values, &defaults) // (created, error) — idem

// Transações (atômico — commit/rollback automático)
fw.DB.Use().Transaction(func(tx *database.Tx) error {
    orm.CreateTx(tx, &model)
    return orm.FromTx[T](tx).Where(...).Update(...)
})

// Multi-tenant
db := fw.DB.Use().WithSchema("tenant_abc")
orm.FromDB[T](db).All()  // → SELECT * FROM tenant_abc.tabela
```

### ORM — tags do model

| Tag | Efeito |
|---|---|
| `kyrux:"pk"` | Chave primária — ignorado no INSERT, preenchido após criação |
| `kyrux:"column:nome"` | Override do nome da coluna SQL |
| `kyrux:"size:N"` | VARCHAR(N) no makemigrations |
| `kyrux:"unique"` | CREATE UNIQUE INDEX no makemigrations (apenas migration) |
| `kyrux:"default:valor"` | Valor SQL literal no INSERT se campo for zero Go |
| `kyrux:"hash"` | Hash Argon2id+pepper automático na escrita; nunca revertido |
| `kyrux:"encrypt"` | AES-256-GCM: cifra na escrita, decifra na leitura |
| `kyrux:"login"` | Exclusivo do `auth.User` — define o campo de login; imutável após migrate |
| `kyrux:"autonow"` | `CURRENT_TIMESTAMP` automático em todo Update; preenche no Create se zerado |
| `kyrux:"fk:tabela"` | `REFERENCES tabela(id)` + índice na migration; vira `<select>` no admin |
| `kyrux:"fklabel:coluna"` | Rótulo do `<select>` de FK no admin (padrão: o id) |
| `kyrux:"fts"` | Full-text nativo via `Query.Search` — GIN (Postgres), FULLTEXT (MySQL) ou FTS5 (SQLite) |
| `kyrux:"image"` | Upload no admin — salva em `medias/<app>/<tabela>/`, exige `admin.App("nome")` |

### Auth — todos os métodos

```go
// Model
user.SetPassword("senha")          // hash Argon2id + pepper
user.CheckPassword("senha")        // bool
user.FullName()                    // "Nome Sobrenome"

// Campo de login (determinado pela tag kyrux:"login" no model User)
auth.LoginFieldName()                              // string — "Username" ou "Email"

// SSR (sessão)
auth.Login(db, store, w, r, loginValue, password)  // (*session.Session, error)
auth.Logout(store, r, w)                           // remove sessão + expira cookie
auth.GetUser(db, store, r)                         // (*User, error)
auth.NextURL(r, fallback)                          // string — lê ?next=, valida open redirect

// JWT
fw.Auth.GenerateToken(userID, ttl)  // (string, error)
fw.Auth.ValidateToken(token)        // (*Claims, error)

// Erros SSR
auth.ErrUserNotFound
auth.ErrWrongPassword
auth.ErrInactiveUser
auth.ErrAuthDisabled  // retornado quando DB_ENABLED=false
```

### Realtime — todos os métodos

```go
fw.Realtime.Replace("target", html)   // innerHTML = html
fw.Realtime.Append("target", html)    // adiciona ao final
fw.Realtime.Prepend("target", html)   // adiciona ao início
fw.Realtime.Remove("target")          // remove o elemento
fw.Realtime.Broadcast("evento", data) // publica no EventBus
```

### EventBus — todos os métodos

```go
fw.Events.Subscribe("evento", handler)   // assinar
fw.Events.Publish("evento", payload)     // publicar
fw.Events.Unsubscribe("evento")          // cancelar
```

### DB Manager — todos os métodos

```go
fw.DB.Add("nome", "driver", "dsn")          // adicionar conexão
fw.DB.Use()                                 // conexão "default"
fw.DB.Use("nome")                           // conexão nomeada
fw.DB.Use().WithSchema("schema")            // cópia com schema (multi-tenant)
fw.DB.Use().Transaction(func(tx) error { ... })
fw.DB.Close()                               // encerrar todas
```

### Render — funções globais e processors

```go
// Funções customizadas para templates (disponíveis globalmente)
render.AddFunc("nome", funcao)
render.AddFunc("formatarData", func(t time.Time) string {
    return t.Format("02/01/2006")
})

// ContextProcessors — variáveis injetadas em todos os templates de todos os apps
render.AddDefaultProcessor(func(ctx *router.Context) map[string]any {
    return map[string]any{"ano": time.Now().Year()}
})

// Renderizar fragmento para string (usado com Realtime)
html, err := render.Partial("appName", "partials/lista.html", data)
```

### Crypton — utilitários de segurança

```go
// Setup (chamado pelo bootstrap automaticamente)
crypton.SetPepper(pepper)
crypton.SetEncryptionKey(key)

// Senhas (Argon2id — formato PHC)
crypton.HashPassword("senha")                  // (string, error) — $argon2id$...
crypton.CheckPassword("senha", hash)           // bool — comparação em tempo-constante

// Criptografia simétrica (AES-256-GCM)
crypton.Encrypt("dado sensível")               // (string, error) — $enc$<base64>
crypton.Decrypt("$enc$<base64>")              // (string, error)

// Assinatura HMAC-SHA256
crypton.Sign("payload", "secret")             // (string, error) — <b64>.<sig>
crypton.Verify("token", "secret")             // (string, error) — payload ou erro

// Aleatoriedade criptograficamente segura
crypton.RandomBytes(32)                        // ([]byte, error)
```

### Errors — customização de páginas de erro

```go
import kyerrors "kyrux/core/errors"

// Registrar handler customizado para código HTTP
kyerrors.Set(404, func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(404)
    json.NewEncoder(w).Encode(map[string]string{"error": "não encontrado"})
})

kyerrors.Set(500, func(w http.ResponseWriter, r *http.Request) {
    // renderizar template personalizado de erro 500
})
```

Handlers registrados têm prioridade sobre o comportamento padrão em qualquer ambiente.

### Session — API de baixo nível

```go
import "kyrux/core/security/session"

// Criar sessão manualmente
sess, err := fw.Sessions.New()
sess.Set("chave", valor)
session.SetCookie(ctx.Writer, sess.ID, ctx.Request.TLS != nil)

// Ler sessão do request
sess, ok := session.FromRequest(ctx.Request, fw.Sessions)
if !ok { /* sem sessão */ }

// Acessar valores (seguro para requisições paralelas)
val, ok := sess.Get("chave")

// Encerrar sessão
fw.Sessions.Delete(sess.ID)
```

---

Kyrux — *execução no momento certo.*
Desenvolvido por [Müller Nocciolli](https://www.nocciolli.com.br) · Licença MIT com atribuição obrigatória.
