# KYRUX — MANUAL DE USO

Framework web em Go baseado em SSR, EventBus e Realtime invisível.
Criado por Müller Nocciolli · [www.kyrux.com.br/docs](https://www.kyrux.com.br/docs/)

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
11. [ORM](#11-orm)
12. [Cache](#12-cache)
13. [Sessões](#13-sessões)
14. [Autenticação JWT](#14-autenticação-jwt)
15. [EventBus](#15-eventbus)
16. [Realtime (DOM sem JS)](#16-realtime-dom-sem-js)
17. [Páginas de Erro](#17-páginas-de-erro)
18. [Debug Dashboard](#18-debug-dashboard)
19. [Fluxo do Sistema](#19-fluxo-do-sistema)
20. [Performance](#20-performance)

---

## 1. Início Rápido

### Pré-requisitos

```bash
go 1.22+
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
SERVER_HOST=0.0.0.0
SERVER_PORT=8000
SERVER_WORKERS=4        # omitir para usar todos os CPUs disponíveis

# ── Hosts permitidos ──────────────────────────────────────────────
# Ignorado em development. Obrigatório em production.
ALLOWED_HOSTS=meusite.com.br,www.meusite.com.br

# ── Banco de dados ────────────────────────────────────────────────
DB_ENABLED=true
DB_DRIVER=postgres
DB_DSN=postgres://user:password@localhost:5432/meudb?sslmode=disable

# ── Cache ─────────────────────────────────────────────────────────
CACHE_ENABLED=false
CACHE_DRIVER=memory
CACHE_ADDR=localhost:6379

# ── Segurança ─────────────────────────────────────────────────────
SECRET_KEY=sua-chave-secreta-forte-aqui
SESSION_TTL=3600        # duração da sessão em segundos
```

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
- Pastas `statics/css/` e `statics/js/`
- Registra em `InstalledApps` e adiciona o import automaticamente

### Remover um app

```bash
go run main.go removeapp <nome>
```

Remove a pasta e desfaz o registro. Pede confirmação antes.

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
| `{{ csrf_token }}`| Framework      | Input hidden de segurança  |

### Herança de templates

**`apps/blog/templates/base.html`:**
```html
<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <title>{% block "title" %}{{ AppName }}{% endblock "title" %}</title>
  <link rel="stylesheet" href="/static/blog/css/style.css">
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
  <script src="/static/blog/js/post.js"></script>
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

### Arquivos estáticos

```html
<link rel="stylesheet" href="/static/blog/css/style.css">
<script src="/static/blog/js/app.js"></script>
<img src="/static/blog/img/logo.png">
```

Os arquivos ficam em `apps/blog/statics/` e são servidos automaticamente em `/static/blog/`.

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

```javascript
// Leia o token do cookie
function getCookie(name) {
    const value = `; ${document.cookie}`;
    const parts = value.split(`; ${name}=`);
    if (parts.length === 2) return parts.pop().split(';').shift();
}

fetch('/api/posts/', {
    method: 'POST',
    headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCookie('kyrux_csrf'),
    },
    body: JSON.stringify({ titulo: 'Meu Post' }),
});
```

---

## 9. Middleware

### Middlewares globais (aplicados a todas as rotas)

Registrados no `bootstrap` — já ativos por padrão:

| Middleware        | Descrição                                              |
|-------------------|--------------------------------------------------------|
| `Recovery()`      | Captura panics — mostra debug page (dev) ou 500 (prod) |
| `AllowedHosts()`  | Bloqueia hosts não autorizados (ignorado em dev)       |
| `csrf.Middleware` | Valida token CSRF em métodos não seguros               |

### Middlewares opcionais

```go
import (
    "kyrux/core/middleware"
    secmiddleware "kyrux/core/security/middleware"
)

// Compressão gzip (global)
r.Use(middleware.Compress)

// CORS
r.Use(secmiddleware.CORS([]string{
    "https://meusite.com.br",
    "https://app.meusite.com.br",
}))

// Exigir autenticação JWT (por rota ou global)
r.Use(secmiddleware.RequireAuth(fw.Auth))
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
    protegido := secmiddleware.RequireAuth(fw.Auth)

    r.Handle("GET /dashboard/", protegido(func(ctx *router.Context) {
        render.For("painel").Render(ctx, "dashboard.html", nil)
    }))
}
```

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

```env
DB_ENABLED=true
DB_DRIVER=postgres
DB_DSN=postgres://user:password@localhost:5432/meudb?sslmode=disable
```

### Conexão padrão

```go
db := fw.DB.Use()          // conexão "default" (configurada no .env)

rows, err := db.Query("SELECT id, titulo FROM posts WHERE publicado = $1", true)
defer rows.Close()

for rows.Next() {
    var id int
    var titulo string
    rows.Scan(&id, &titulo)
}
```

### Múltiplas conexões

```go
// Adicionar conexão secundária (em qualquer lugar após o bootstrap)
fw.DB.Add("analytics", "postgres", os.Getenv("ANALYTICS_DSN"))
fw.DB.Add("legado",    "mysql",    os.Getenv("LEGADO_DSN"))

// Usar
db := fw.DB.Use("analytics")
db := fw.DB.Use("legado")
```

### Transações

```go
err := fw.DB.Use().Transaction(func(tx *sql.Tx) error {
    _, err := tx.Exec("INSERT INTO posts (titulo) VALUES ($1)", "Novo post")
    if err != nil {
        return err // rollback automático
    }
    _, err = tx.Exec("UPDATE stats SET total = total + 1")
    return err // commit se nil
})
```

### Multi-tenant com schema

```go
// Retorna uma cópia da conexão com o schema definido — a original não é alterada.
db := fw.DB.Use().WithSchema("tenant_abc")
```

Todas as queries executadas com essa conexão usarão `tenant_abc.<tabela>` automaticamente.
Veja a seção [ORM](#11-orm) para uso completo com multi-tenant.

### Drivers suportados

| Driver      | Módulo Go                          | Observação          |
|-------------|-------------------------------------|---------------------|
| `postgres`  | `github.com/lib/pq`                 |                     |
| `pgx`       | `github.com/jackc/pgx/v5/stdlib`    | Melhor performance  |
| `mysql`     | `github.com/go-sql-driver/mysql`    | Também MariaDB      |
| `sqlite`    | `modernc.org/sqlite`                | Sem CGO             |
| `sqlite3`   | `github.com/mattn/go-sqlite3`       | Requer CGO          |
| `sqlserver` | `github.com/microsoft/go-mssqldb`   |                     |
| `oracle`    | `github.com/sijms/go-ora/v2`        | Puro Go, sem CGO    |

> Para MongoDB, Redis, Cassandra e DynamoDB use os clients nativos. Consulte `core/database/drivers.go`.

---

## 11. ORM

ORM leve e fluente integrado ao framework. Usa generics, reflection cacheada e SQL explícito com placeholders — sem magia, sem surpresas.

```go
import "kyrux/core/orm"
```

### Definição de model

Um model é qualquer struct Go com campos exportados. Use a tag `kyrux` para configurar comportamento:

```go
type Post struct {
    ID        int64     `kyrux:"pk"`
    Titulo    string
    Slug      string
    Publicado bool
    CriadoEm time.Time `kyrux:"column:criado_em"`
}
```

#### Tags disponíveis

| Tag | Descrição |
|---|---|
| `kyrux:"pk"` | Marca como chave primária. Ignorado no INSERT, preenchido de volta após criação. |
| `kyrux:"column:nome"` | Override do nome da coluna SQL. Por padrão é `snake_case` do nome do campo. |

#### Nome da tabela

Gerado automaticamente a partir do nome do struct em `snake_case` plural:

| Struct | Tabela |
|---|---|
| `User` | `users` |
| `Post` | `posts` |
| `Category` | `categories` |
| `UserProfile` | `user_profiles` |
| `Address` | `addresses` |

### Leitura

#### All — buscar todos

```go
db := fw.DB.Use()

posts, err := orm.From[Post](db).All()

// Com filtros
posts, err := orm.From[Post](db).
    Where("publicado = ?", true).
    OrderBy("criado_em DESC").
    Limit(10).
    Offset(20).  // página 3
    All()
```

#### First — buscar o primeiro

Retorna `sql.ErrNoRows` se nenhuma linha for encontrada.

```go
post, err := orm.From[Post](db).
    Where("slug = ?", slug).
    First()

if errors.Is(err, sql.ErrNoRows) {
    ctx.Error(404)
    return
}
```

#### Count — contar linhas

```go
total, err := orm.From[Post](db).Count()

publicados, err := orm.From[Post](db).
    Where("publicado = ?", true).
    Count()
```

#### Métodos de filtro encadeáveis

| Método | Descrição |
|---|---|
| `Where(cond string, args ...any)` | Adiciona condição `AND`. Múltiplos `Where` são combinados com `AND`. |
| `OrderBy(col string)` | Define `ORDER BY`. Ex: `"criado_em DESC"`. |
| `Limit(n int)` | Máximo de linhas retornadas. |
| `Offset(n int)` | Linhas a pular — use com `Limit` para paginação. |

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

### Atualização

Exige ao menos um `Where` para evitar updates acidentais em toda a tabela.

```go
err := orm.From[Post](db).
    Where("id = ?", 1).
    Update(map[string]any{
        "titulo":    "Título atualizado",
        "publicado": true,
    })
```

### Deleção

Exige ao menos um `Where` para evitar deleções acidentais em toda a tabela.

```go
err := orm.From[Post](db).
    Where("id = ?", 1).
    Delete()
```

### Multi-tenant com schema

```go
// Middleware de tenant define o schema
db := fw.DB.Use().WithSchema("tenant_" + tenantID)

// Todas as queries usam o schema automaticamente
posts, _ := orm.From[Post](db).Where("publicado = ?", true).All()
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
    return orm.From[Post](db).
        Where("publicado = ?", true).
        OrderBy("id DESC").
        All()
}

func BuscarPorID(db *database.DB, id int64) (*Post, error) {
    return orm.From[Post](db).
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

## 12. Cache

Cache em memória com TTL. Ativado via `CACHE_ENABLED=true`.

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

## 13. Sessões

Sessões em memória server-side com TTL configurável via `SESSION_TTL`.

### Criar sessão

```go
func LoginView(ctx *router.Context) {
    // ... validar credenciais ...

    sess, err := fw.Sessions.New()
    if err != nil {
        ctx.Error(500)
        return
    }

    sess.Values["usuario_id"] = usuario.ID
    sess.Values["nome"] = usuario.Nome

    session.SetCookie(ctx.Writer, sess.ID, true) // true = Secure (HTTPS em produção)

    ctx.Redirect("/dashboard/", http.StatusFound)
}
```

### Ler sessão

```go
func DashboardView(ctx *router.Context) {
    sess, ok := session.FromRequest(ctx.Request, fw.Sessions)
    if !ok {
        ctx.Redirect("/login/", http.StatusFound)
        return
    }

    usuarioID := sess.Values["usuario_id"].(int)
    render.For("painel").Render(ctx, "dashboard.html", map[string]any{
        "usuario_id": usuarioID,
    })
}
```

### Encerrar sessão (logout)

```go
func LogoutView(ctx *router.Context) {
    if c, err := ctx.Request.Cookie(session.CookieName()); err == nil {
        fw.Sessions.Delete(c.Value)
    }
    http.SetCookie(ctx.Writer, &http.Cookie{
        Name:    session.CookieName(),
        Value:   "",
        MaxAge:  -1,
        Path:    "/",
    })
    ctx.Redirect("/login/", http.StatusFound)
}
```

---

## 14. Autenticação JWT

Para APIs e autenticação stateless.

### Gerar token

```go
func LoginAPIView(ctx *router.Context) {
    // ... validar usuário ...

    token, err := fw.Auth.GenerateToken(
        strconv.Itoa(usuario.ID),
        24*time.Hour,
    )
    if err != nil {
        ctx.Error(500)
        return
    }

    ctx.JSON(200, map[string]string{"token": token})
}
```

### Validar token manualmente

```go
claims, err := fw.Auth.ValidateToken(token)
if err != nil {
    ctx.JSON(401, map[string]string{"error": "não autorizado"})
    return
}
usuarioID := claims.UserID
expira := claims.ExpiresAt
```

### Middleware `RequireAuth`

Protege rotas automaticamente. O token deve vir no header `Authorization: Bearer <token>`.

```go
import secmiddleware "kyrux/core/security/middleware"

// Proteger rota individual
r.Handle("GET /api/perfil/", secmiddleware.RequireAuth(fw.Auth)(func(ctx *router.Context) {
    v, _ := ctx.Get("claims")
    claims := v.(*auth.Claims)
    ctx.JSON(200, map[string]string{"user_id": claims.UserID})
}))

// Ou aplicar globalmente para todas as rotas do app
r.Use(secmiddleware.RequireAuth(fw.Auth))
```

---

## 15. EventBus

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

---

## 16. Realtime (DOM sem JS)

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

## 17. Páginas de Erro

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

## 18. Debug Dashboard

Disponível automaticamente em `APP_ENV=development`:

```
http://localhost:8000/kyrux/debug/
```

Exibe:

| Seção       | Informações                                              |
|-------------|----------------------------------------------------------|
| Aplicação   | Nome, versão, ambiente, endereço, uptime                 |
| Runtime     | Go version, OS/arch, workers, goroutines, heap, GC cycles |
| Rotas       | Todas as rotas registradas com método e path             |

---

## 19. Fluxo do Sistema

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

## 20. Performance

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
orm.From[T](db).All()                          // ([]T, error)
orm.From[T](db).First()                        // (*T, error) — sql.ErrNoRows se vazio
orm.From[T](db).Count()                        // (int64, error)

// Filtros encadeáveis (retornam *Query[T])
.Where("col = ?", val)
.OrderBy("col DESC")
.Limit(n)
.Offset(n)

// Escrita
orm.Create(db, &model)                         // error — preenche PK
orm.From[T](db).Where(...).Update(map[string]any{...}) // error
orm.From[T](db).Where(...).Delete()            // error

// Multi-tenant
db := fw.DB.Use().WithSchema("tenant_abc")
orm.From[T](db).All()  // → SELECT * FROM tenant_abc.tabela
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

---

Kyrux — *execução no momento certo.*
Desenvolvido por [Müller Nocciolli](https://www.nocciolli.com.br) · Licença MIT com atribuição obrigatória.
