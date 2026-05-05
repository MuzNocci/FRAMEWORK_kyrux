# KYRUX FRAMEWORK
Criado e desenvolvido por Müller Nocciolli.
Contato: muller.nocciolli@gmail.com
Site: www.nocciolli.com.br
Documentação: www.kyrux.com.br/docs/


## VISÃO GERAL
Framework web em Go baseado em SSR, EventBus e Realtime invisível.


## PRINCÍPIOS
- SSR-first
- Event-driven architecture
- Realtime automático e invisível
- Security centralizada
- Dev foca apenas na lógica


## CORE MODULES

### Bootstrap
Inicializa o framework: env, settings, db, cache, router, realtime, csrf, security headers.

### Environment
Leitura de `.env` com suporte a comentários inline. Variáveis do OS têm prioridade.

### Settings
Configurações tipadas lidas do ambiente. Debug ligado automaticamente com `APP_ENV=development`.

### Router
Multiplexador HTTP com tipos de path (`<id:int>`, `<slug:str>`, `<uid:uuid>`, `<resto:path>`),
resolução de URLs por nome, middleware global e por rota, e interceptação de 404/405.

### Render
Renderização SSR com herança de templates Django-like, hot reload em dev, sync.Pool de buffers,
ContextProcessors e injeção automática de WebSocket.

### Templates
Herança via `{% extends %}`, blocos com `{% block %}`, inclusão com `{% include %}`.
Funções globais: `{{ AppName }}`, `{{ Version }}`, `{{ Env }}`, `{{ Addr }}`, `{{ url "nome" }}`, `{{ csrf_token }}`.

### Middleware
Recovery (panic), AllowedHosts, CORS, SecureHeaders, RequireLogin (SSR), RequireAuth (JWT), compressão gzip.

### Security / CSRF
CSRF automático em POST/PUT/PATCH/DELETE via cookie + field hidden ou header `X-CSRF-Token`.

### Security / Session
Sessões em memória com TTL, GC automático, cookie `HttpOnly + SameSite=Strict`.

### Security / Auth
Model `auth.User` com hash Argon2id+pepper, Login/Logout por sessão, JWT para APIs.
Campo de login definido via tag `kyrux:"login"` no model — imutável após o primeiro migrate.
`Email *string` — opcional quando não é o campo de login; permite `NULL` sem violar índice `UNIQUE`.

### Security / Crypton
Argon2id para senhas, AES-256-GCM para campos sensíveis, HMAC-SHA256 para assinaturas.

### ORM
Query builder fluente com generics, hash/encrypt automático, paginação, multi-tenant por schema.

### Database
Wrapper de `database/sql` com pool configurado, multi-conexão nomeada, transações e schema por conexão.

### Migrations
Arquivos `.sql` numerados em `database/migrations/`. Rastreamento via tabela `kyrux_migrations`.

### Cache
Cache em memória com TTL e GC automático.

### EventBus
Pub/sub assíncrono em goroutines separadas.

### Realtime
WebSocket invisível — atualiza DOM sem JS manual via `fw.Realtime.Replace/Append/Prepend/Remove`.

### Errors
Páginas de erro customizáveis. Debug page com stack trace em desenvolvimento.

### CLI
Scaffolding, migrations e gerenciamento de usuários via `go run main.go <comando>`.


## COMANDOS CLI

```bash
go run main.go startapp <nome>      # cria estrutura de app em apps/<nome>/
go run main.go removeapp <nome>     # remove app e desfaz registro (com confirmação)
go run main.go makemigrations       # gera SQL a partir dos models (apps/*/models/ + auth)
go run main.go migrate              # aplica migrations pendentes em database/migrations/
go run main.go createsuperuser      # cria usuário is_admin + is_staff interativamente
go run main.go createuser           # cria usuário comum interativamente
go run main.go removemigration 0003        # remove migration do disco
go run main.go removemigration 0003 all    # remove do disco + da tabela kyrux_migrations
```


## VARIÁVEIS DE AMBIENTE (.env)

```env
# ── Ambiente ──────────────────────────────────────────────────────
APP_ENV=development          # development | production

# ── Servidor ──────────────────────────────────────────────────────
SERVER_HOST=0.0.0.0
SERVER_PORT=8000
SERVER_WORKERS=4             # omitir → runtime.NumCPU()

# ── Hosts permitidos (ignorado em development) ────────────────────
ALLOWED_HOSTS=meusite.com.br,www.meusite.com.br

# ── Banco de dados ────────────────────────────────────────────────
DB_ENABLED=true
DB_DRIVER=postgres           # postgres | pgx | mysql | sqlite | sqlserver | oracle
DB_DSN=postgres://user:pass@localhost:5432/db?sslmode=disable

# ── Cache ─────────────────────────────────────────────────────────
CACHE_ENABLED=false
CACHE_DRIVER=memory
CACHE_ADDR=localhost:6379

# ── Segurança (obrigatórios em production) ────────────────────────
SECRET_KEY=sua-chave-secreta-forte-aqui     # mínimo 32 chars
SESSION_TTL=3600                            # segundos

# Pepper para hash Argon2id — nunca armazenar no banco
# Gere com: openssl rand -base64 32
PASSWORD_PEPPER=seu-pepper-forte-aqui

# Chave AES-256-GCM para campos kyrux:"encrypt" — nunca armazenar no banco
# Gere com: openssl rand -base64 32
FIELD_ENCRYPTION_KEY=sua-chave-de-criptografia-forte-aqui

# ── Runtime (opcional) ────────────────────────────────────────────
RUNTIME_GOGC=75              # GC percentage (padrão Go: 100)
```

> Em produção: `SECRET_KEY`, `PASSWORD_PEPPER`, `FIELD_ENCRYPTION_KEY` e `ALLOWED_HOSTS` são **obrigatórios**
> — o servidor recusa iniciar sem eles.


## TAGS DOS MODELS (kyrux)

```go
type Produto struct {
    ID        int64     `kyrux:"pk"`
    Nome      string    `kyrux:"size:200"`
    Slug      string    `kyrux:"size:200,unique"`
    Preco     float64   `kyrux:"default:0"`
    Ativo     bool      `kyrux:"default:true"`
    CriadoEm time.Time `kyrux:"column:criado_em,default:NOW()"`
}

type Cliente struct {
    ID   int64  `kyrux:"pk"`
    CPF  string `kyrux:"size:14,encrypt"` // AES-256-GCM: cifrado no banco, decifrado na leitura
    Pin  string `kyrux:"hash"`            // Argon2id: hash na escrita, nunca revertido
}
```

| Tag | Descrição |
|---|---|
| `pk` | Chave primária. Ignorado no INSERT, preenchido após criação. |
| `column:nome` | Override do nome da coluna (padrão: snake_case do campo). |
| `size:N` | VARCHAR(N) na migration. |
| `unique` | CREATE UNIQUE INDEX na migration. |
| `default:valor` | Valor SQL default quando o campo for zero (`NOW()`, `true`, `0`, `''`, etc.). |
| `hash` | Auto-hash Argon2id+pepper na escrita (Create/Update). Nunca revertido. |
| `encrypt` | Auto-cifra AES-256-GCM na escrita, decifra na leitura. Requer `FIELD_ENCRYPTION_KEY`. |
| `login` | Marca o campo de login do `auth.User`. Apenas um campo. Imutável após o primeiro migrate. |


## DRIVERS DE BANCO SUPORTADOS

| Driver | Módulo Go | Observação |
|---|---|---|
| `postgres` | `github.com/lib/pq` | |
| `pgx` | `github.com/jackc/pgx/v5/stdlib` | Melhor performance |
| `mysql` | `github.com/go-sql-driver/mysql` | Também MariaDB |
| `sqlite` | `modernc.org/sqlite` | Sem CGO |
| `sqlite3` | `github.com/mattn/go-sqlite3` | Requer CGO |
| `sqlserver` | `github.com/microsoft/go-mssqldb` | |
| `oracle` | `github.com/sijms/go-ora/v2` | Puro Go |

Importe o driver com blank identifier em `main.go`:
```go
import _ "github.com/lib/pq"
```


## FLUXO DO SISTEMA
```
Request → Recovery → AllowedHosts → CSRF → Middleware da rota → Router → View
  → ORM / SQL raw / Cache / EventBus / Realtime
  → Render (SSR + injeção WebSocket + hot reload em dev)
  → Response
```

## FLUXO DE DESENVOLVIMENTO
```
.go alterado       → Air detecta → recompila → reinicia servidor
.html/.css/.js     → hotreload detecta → SSE → browser recarrega sem reload manual
```


## PERFORMANCE (ab · 100 conexões · keep-alive · i5-1235U · Go 1.26.2)

```
GET /ping/         (estático)    127.474 req/s    0,784 ms    0 erros
GET /usuarios/42/  (path param)  126.454 req/s    0,791 ms    0 erros
GET /busca/?q=...  (query str)   109.302 req/s    0,915 ms    0 erros
500 conexões       (pico)         99.110 req/s    5,045 ms    0 erros
```

Percentis (P50/P90/P95/P99/P100): 0,89 / 1,34 / 1,72 / 2,44 / 6,36 ms.
Zero falhas em 200.000 requisições totais.


## CONCEITO CHAVE
Kyrux representa execução no "momento certo" (Kairos),
onde eventos dirigem a atualização do sistema em tempo real.


## LICENÇA
MIT License com cláusula de atribuição obrigatória.
Qualquer projeto que utilize o Kyrux deve incluir créditos visíveis ao framework.
Veja o arquivo LICENSE para detalhes.
