# KYRUX FRAMEWORK
Criado e desenvolvido por Müller Nocciolli.
Contato: muller.nocciolli@gmail.com
Site: www.nocciolli.com.br
Documentação: www.kyrux.com.br/docs/


## VISÃO GERAL:
Framework web em Go baseado em SSR, EventBus e Realtime invisível.


## PRINCÍPIOS:
- SSR-first
- Event-driven architecture
- Realtime automático e invisível
- Security centralizada
- Dev foca apenas na lógica


## CORE MODULES:

### Bootstrap:
Inicializa todo o framework (env, db, cache, router, realtime, csrf, allowed hosts)

### Environment:
Leitura de .env e variáveis do sistema com suporte a comentários inline

### Settings:
Configuração tipada global. Debug ligado automaticamente quando APP_ENV=development

### Router:
Mapeamento de URLs para views com resolução de nomes e rastreamento de rotas registradas

### Render:
Renderização SSR com herança de templates, sync.Pool de buffers e injeção automática de realtime

### Templates:
Herança e composição Django-like com extends, block, endblock e include

### Middleware:
Compressão gzip, CORS, recovery e cadeia de middlewares por rota

### Security:
CSRF automático, Allowed Hosts, autenticação JWT, sessão e criptografia

### EventBus:
Sistema interno de eventos desacoplados

### Realtime:
WebSocket invisível com atualização de DOM sem JS manual

### ORM:
ORM leve e fluente com generics e reflection cacheada. Query builder tipado, SQL explícito com placeholders, suporte a multi-tenant via schema e compatível com todos os drivers SQL

### CLI:
Criação e remoção de apps via linha de comando


## COMANDOS:

### Iniciar o servidor:
```bash
go run main.go
```
O modo é detectado automaticamente pelo APP_ENV:
- `development` → inicia com Air (live reload). Requer Air instalado.
- `production` → inicia o servidor diretamente.

### Instalar o Air (uma vez):
```bash
go install github.com/air-verse/air@latest
```

### Criar um novo app:
```bash
go run main.go startapp <nome>
```

### Remover um app existente:
```bash
go run main.go removeapp <nome>
```


## VARIÁVEIS DE AMBIENTE (.env):

```env
# Environment
# development → debug, hotreload e pprof ativados automaticamente
# production  → modo otimizado, debug desligado
APP_ENV=development

# Server
SERVER_HOST=0.0.0.0
SERVER_PORT=8000
SERVER_WORKERS=4  # omitir para usar todos os CPUs disponíveis

# Database
DB_ENABLED=true
DB_DRIVER=postgres
DB_DSN=postgres://user:password@localhost:5432/kyrux?sslmode=disable

# Cache
CACHE_ENABLED=false
CACHE_DRIVER=memory
CACHE_ADDR=localhost:6379

# Security
SECRET_KEY=your-strong-random-secret-key-here
SESSION_TTL=3600
ALLOWED_HOSTS=localhost,127.0.0.1  # omitir em development (ignorado automaticamente)
```


## TEMPLATES:

### Variáveis do framework (sem ponto):
```html
{{ AppName }}    {{ Version }}    {{ Env }}
{{ Addr }}       {{ GoVersion }}  {{ url "nome_da_rota" }}
```

### Variáveis da view (com ponto):
```html
{{ .titulo }}    {{ .usuario }}    {{ .produtos }}
```

A convenção é visual: sem ponto = framework, com ponto = seus dados.

### Herança de templates:

**base.html:**
```html
<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <title>{% block "title" %}{{ AppName }}{% endblock "title" %}</title>
</head>
<body>
  {% block "content" %}{% endblock "content" %}
</body>
</html>
```

**página.html:**
```html
{% extends "base.html" %}

{% block "title" %}Minha Página{% endblock "title" %}

{% block "content" %}
  {% include "partials/header.html" %}
  <h1>{{ .titulo }}</h1>
{% endblock "content" %}
```

### CSRF em formulários:
```html
<form method="POST" action="{{ url "criar_usuario" }}">
  {{ csrf_token }}
  <input type="text" name="nome">
  <button type="submit">Salvar</button>
</form>
```
O CSRF é validado automaticamente em POST, PUT, PATCH e DELETE.
Para AJAX, envie o token no header `X-CSRF-Token`.


## REALTIME (DOM sem JS):

O Kyrux injeta automaticamente o WebSocket em toda página.
O desenvolvedor usa apenas atributos HTML e funções Go.

### Template:
```html
<div kyrux-target="lista-usuarios">
  {% include "partials/lista.html" %}
</div>
```

### View:
```go
func CriarUsuario(ctx *router.Context) {
    // ... salva no banco ...

    html, _ := render.Partial("users", "partials/lista.html", map[string]any{
        "usuarios": usuarios,
    })

    fw.Realtime.Replace("lista-usuarios", html)  // substitui o conteúdo
    fw.Realtime.Append("lista-usuarios", html)   // adiciona ao final
    fw.Realtime.Prepend("lista-usuarios", html)  // adiciona ao início
    fw.Realtime.Remove("lista-usuarios")         // remove o elemento
}
```

Zero JS escrito pelo desenvolvedor. O framework cuida de tudo.


## BANCO DE DADOS:

### Múltiplas conexões:
O Kyrux não importa nenhum driver — isso é responsabilidade do desenvolvedor.
Adicione o driver no `go.mod` e importe com blank identifier:

```go
import _ "github.com/lib/pq"          // PostgreSQL
import _ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL (pgx)
import _ "github.com/go-sql-driver/mysql"  // MySQL / MariaDB
import _ "modernc.org/sqlite"          // SQLite (sem CGO)
```

### Conexão padrão (via .env):
```env
DB_ENABLED=true
DB_DRIVER=postgres
DB_DSN=postgres://user:password@localhost:5432/kyrux?sslmode=disable
```

### Múltiplos bancos:
```go
fw.DB.Add("analytics", "postgres", os.Getenv("ANALYTICS_DSN"))
fw.DB.Add("legacy",    "mysql",    os.Getenv("LEGACY_DSN"))
```

### Uso (SQL raw):
```go
db := fw.DB.Use()             // conexão "default"
db := fw.DB.Use("analytics")  // conexão nomeada

rows, err := db.Query("SELECT id, nome FROM usuarios")
```

### ORM:
```go
import "kyrux/core/orm"

// SELECT
users, err := orm.From[User](db).
    Where("active = ?", true).
    OrderBy("id DESC").
    Limit(20).
    All()

// INSERT
user := User{Name: "Maria"}
orm.Create(db, &user)       // user.ID preenchido após inserção

// UPDATE
orm.From[User](db).Where("id = ?", 1).Update(map[string]any{"name": "Carlos"})

// DELETE
orm.From[User](db).Where("id = ?", 1).Delete()

// Multi-tenant
db := fw.DB.Use().WithSchema("tenant_abc")
orm.From[User](db).All()    // → SELECT * FROM tenant_abc.users
```

### Drivers suportados:
| Driver       | Módulo Go                              | Observação          |
|-------------|----------------------------------------|---------------------|
| postgres     | github.com/lib/pq                      |                     |
| pgx          | github.com/jackc/pgx/v5/stdlib         | melhor performance  |
| mysql        | github.com/go-sql-driver/mysql         | também MariaDB      |
| sqlite       | modernc.org/sqlite                     | sem CGO             |
| sqlite3      | github.com/mattn/go-sqlite3            | requer CGO          |
| sqlserver    | github.com/microsoft/go-mssqldb        |                     |
| oracle       | github.com/sijms/go-ora/v2             | puro Go, sem CGO    |

Clientes nativos (MongoDB, Redis, Cassandra, DynamoDB) não usam `database/sql` — consulte `core/database/drivers.go` para referência completa.


## URLS:

### Definição:
```go
var URLPatterns = []router.URLPattern{
    router.Path("GET",  "/",        views.HomeView,   "home"),
    router.Path("POST", "/usuarios", views.CriarUsuario, "criar_usuario"),
}
```

### Uso no template:
```html
<a href="{{ url "home" }}">Início</a>
<form action="{{ url "criar_usuario" }}">...</form>
```


## FLUXO DO SISTEMA:
Request → Allowed Hosts → CSRF → Middleware → Router → View → Service → DB
→ EventBus → Realtime → DOM atualizado automaticamente


## FLUXO DE DESENVOLVIMENTO:
Arquivo .go alterado    → Air detecta → Recompila → Reinicia servidor
Arquivo .html/.css/.js  → hotreload detecta → SSE → Browser recarrega


## DEBUG DASHBOARD (apenas em development):
Disponível automaticamente em `http://localhost:8000/kyrux/debug/`
Exibe informações da aplicação, runtime Go (goroutines, heap, GC) e todas as rotas registradas.


## PERFORMANCE:

Benchmarks medidos com `ab` (HTTP real, TCP, keep-alive) e suite `testing.B` do Go.
Hardware: Intel Core i5-1235U · Go 1.26.2 · Linux · **SERVER_WORKERS=4** (conforme `.env`).

### HTTP real — ab (keep-alive, 100 conexões simultâneas)

```
GET /ping/          (estático)    127.474 req/s    0,784 ms/req    0 erros
GET /usuarios/42/   (path param)  126.454 req/s    0,791 ms/req    0 erros
GET /busca/?q=...   (query str)   109.302 req/s    0,915 ms/req    0 erros
500 conexões        (pico)         99.110 req/s    5,045 ms/req    0 erros
```

Zero falhas em 200.000 requisições totais.

### Latência — percentis (100 conexões, 50k req)

```
P50    0,89 ms
P90    1,34 ms
P95    1,72 ms
P99    2,44 ms
P100   6,36 ms   (pior caso absoluto)
```

### Benchmarks Go nativos — testing.B (GOMAXPROCS=4, sem TCP overhead)

```
Rota estática      1104 ns/op      ~906.000 req/s    15 allocs
Path param         1313 ns/op      ~762.000 req/s    18 allocs
Query string       1715 ns/op      ~583.000 req/s    22 allocs
1 middleware        878 ns/op    ~1.139.000 req/s    12 allocs
3 middlewares       908 ns/op    ~1.101.000 req/s    12 allocs
Estático paral.     923 ns/op    ~4.334.000 req/s    15 allocs
Path param par.     697 ns/op    ~5.743.000 req/s    18 allocs
```

> Adicionar 3 middlewares custa menos de 30 ns em relação a 1 — o chain é compilado antes do request.
> Respostas JSON incluem `Content-Length` — sem chunked transfer encoding no HTTP/1.1.
> Medido em localhost com GOMAXPROCS=4, respeitando SERVER_WORKERS do `.env`.


## CONCEITO CHAVE:
Kyrux representa execução no "momento certo" (Kairos),
onde eventos dirigem a atualização do sistema em tempo real.


## LICENÇA:
MIT License com cláusula de atribuição obrigatória.
Qualquer projeto que utilize o Kyrux deve incluir créditos visíveis ao framework.
Veja o arquivo LICENSE para detalhes.
