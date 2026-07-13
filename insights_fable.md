# Kyrux Framework — Insights de Desempenho e Segurança

> Análise gerada por Claude (Fable 5) em 2026-07-13.
> Escopo: todo o `core/`, `main.go`, documentação (README.md, USE.md) e configuração (.env, .env.example).
> Prioridades da análise: **desempenho** e **segurança**, preservando a facilidade de uso Django-like.
>
> **STATUS (2026-07-13): todos os itens implementados** — C1–C4, S1–S11 (incl.
> observações menores: `csrf.Exempt`, página 400, env sem `os.Setenv`) e P1–P8.
> Únicas exceções conscientes: o hack `gid()` do P6 foi mantido (refatorar o
> contexto por goroutine tem risco alto e ganho de ~1 µs) e o backend externo
> de sessões do S11 ficou documentado como limitação, não implementado.
> A doc de AJAX+CSRF do USE.md também foi corrigida (enviava o cookie bruto
> em vez do token assinado — fluxo sempre resultava em 403).

---

## Sumário executivo

O Kyrux está bem acima da média para um framework em estágio Alpha: CSRF com HMAC e comparação em tempo constante, Argon2id + pepper, AES-256-GCM em campos, prevenção de open redirect no `?next=`, proteção contra session fixation, `sync.Pool` em todos os hot paths do router/render, e validação de identificadores no ORM contra SQL injection. A base é sólida.

Foram encontrados, porém, **4 problemas críticos** (2 que quebram o sistema em produção e 2 de segurança/recurso), além de um conjunto de melhorias de segurança e desempenho de médio impacto. Nenhuma das correções sugeridas altera a API pública usada pelo desenvolvedor — a facilidade de uso fica intacta.

---

## 🔴 Críticos

### C1. Configuração de banco de dados em 3 formatos divergentes — servidor não sobe

Existem **três formatos de configuração de banco convivendo no projeto**, e o que o bootstrap realmente usa não é o que a documentação ensina:

| Onde | Formato |
|---|---|
| `README.md` / `USE.md` / `core/settings/settings.go:79` | Blocos `DB_NAME` / `DB_ENABLED` / `DB_DRIVER` / `DB_DSN` |
| `core/orm/config.go:25` (o que o bootstrap executa) | JSON na env var `DATABASES` |
| `.env` atual do projeto | Formato flat antigo (`DB_ENABLED`, `DB_DRIVER`, `DB_DSN` sem `DB_NAME`) |

O `bootstrap.Init` chama `orm.LoadDatabases()` ([bootstrap.go:93](core/bootstrap/bootstrap.go#L93)), que lê `os.Getenv("DATABASES")` e **faz `panic` se a variável não existir** ([config.go:25-27](core/orm/config.go#L25-L27)). Ou seja: um usuário que siga o README à risca (blocos `DB_NAME`) terá o servidor recusando iniciar. O `settings.Load()` até monta `cfg.Databases` corretamente a partir dos blocos, mas **esse resultado é ignorado pelo bootstrap**.

**Correção sugerida:** fazer `orm.LoadDatabases` receber `cfg.Databases` (do settings) em vez de ler `DATABASES` por conta própria, e remover o formato JSON — ou mantê-lo como fallback silencioso. Um formato só, documentado uma vez.

### C2. CSP de produção bloqueia o script Realtime injetado

Em produção, `SecureHeaders` envia `Content-Security-Policy: ... script-src 'self' ...` ([middleware.go:103](core/security/middleware/middleware.go#L103)). Mas o motor de render **injeta um `<script>` inline** (o `liveScript` do WebSocket) antes do `</body>` de toda página ([render.go:41-62](core/render/render.go#L41-L62), [render.go:270-289](core/render/render.go#L270-L289)).

`script-src 'self'` **bloqueia scripts inline** — resultado: o Realtime (o recurso-assinatura do framework) **para de funcionar silenciosamente em produção**, exatamente o ambiente onde o CSP é aplicado.

**Correções possíveis (em ordem de preferência):**
1. Servir o `liveScript` como arquivo estático interno (`/kyrux/js/live.js`) e injetar apenas `<script src=...>` — compatível com `'self'`, cacheável pelo browser e menor payload por página.
2. Calcular o hash SHA-256 do script uma vez no boot e adicionar `'sha256-<hash>'` ao `script-src`.
3. Nonce por request (mais caro e complexo).

### C3. `Update` com `hash`/`encrypt` falha aberto — pode gravar plaintext no banco

Em `Query.Update` ([query.go:182-194](core/orm/query.go#L182-L194)), se `crypton.HashPassword` ou `crypton.Encrypt` retornarem erro, o código **silenciosamente usa o valor original** — uma senha em texto claro ou um CPF sem cifrar vai direto para o banco:

```go
if hashed, err := crypton.HashPassword(s); err == nil {
    val = hashed
}   // ← em caso de erro, val continua sendo o plaintext
```

O `orm.Create` trata isso corretamente (retorna o erro — [exec.go:45-61](core/orm/exec.go#L45-L61)); o `Update` não. É o caso clássico de *fail-open* em código de segurança: deve ser *fail-closed* — retornar erro e abortar o UPDATE.

O mesmo padrão aparece em `scanRows` ([scanner.go:46-48](core/orm/scanner.go#L46-L48)): erro de decrypt é engolido e o ciphertext é devolvido ao caller como se fosse o dado. Aqui devolver o erro pode ser debatível, mas no mínimo deveria ser logado.

### C4. Vazamento de prepared statements no `PrepareCached`

Em [database.go:181-205](core/database/database.go#L181-L205), quando o cache atinge 512 entradas, o statement é **preparado, retornado e nunca fechado nem cacheado**:

```go
if len(db.stmts) < maxCachedStmts {
    db.stmts[query] = s
}   // ← acima de 512: stmt órfão, jamais fechado
return s, nil
```

Cada chamada além do limite cria um prepared statement **no servidor de banco** que só morre quando a conexão morre (lifetime de 30 min). O problema é **agravado pelo próprio ORM**: `LIMIT` e `OFFSET` são interpolados como literais no SQL ([query.go:341-345](core/orm/query.go#L341-L345), [query.go:379](core/orm/query.go#L379)), então `Paginate(1, 10)`, `Paginate(2, 10)`, `Paginate(3, 10)`… geram queries textualmente distintas — uma listagem paginada popular estoura as 512 entradas sozinha.

**Correções (as duas juntas):**
1. Emitir `LIMIT ? OFFSET ?` como placeholders e passar os valores em `args` — colapsa todas as páginas numa única query preparada (ganho direto de desempenho, também).
2. Trocar o mapa por um LRU: ao evictar, chamar `stmt.Close()`; nunca devolver statement fora do cache sem responsabilidade de fechamento.

---

## 🟠 Segurança — alta prioridade

### S1. Login sem rate limiting + Argon2id 64 MB = DoS barato

Cada tentativa de login roda Argon2id com **64 MB de memória** ([crypton.go:33-39](core/security/crypton/crypton.go#L33-L39)) — parâmetro correto pelo OWASP, mas sem nenhuma proteção de volume: ~100 requisições de login simultâneas alocam ~6,4 GB e saturam a CPU. Um atacante derruba o servidor com `ab -c 100` contra o form de login, sem credencial nenhuma.

**Correções:**
- Um **semáforo global** em volta de `HashPassword`/`CheckPassword` (ex.: `min(NumCPU, 4)` slots) — limita o pico de memória a um teto fixo. Simples, invisível para o dev.
- **Rate limit por IP** nas rotas de autenticação (token bucket em memória combina com a filosofia do framework). Poderia ser um middleware pronto: `middleware.RateLimit(n, window)` — hoje o framework não oferece nenhum.

### S2. Enumeração de usuários por timing no Login

Em [login.go:98-106](core/security/auth/login.go#L98-L106), quando o usuário **não existe**, a função retorna imediatamente (~1 ms de query); quando existe, roda Argon2id (~50-100 ms). A diferença de tempo revela quais usernames existem.

**Correção:** quando o usuário não for encontrado, comparar a senha contra um hash dummy fixo (gerado no boot) antes de retornar `ErrUserNotFound` — equaliza o tempo de resposta. Considerar também expor um erro único `ErrInvalidLogin` para as três falhas (não encontrado / inativo / senha errada), deixando os erros específicos para logging interno.

### S3. Cookie de sessão sem `Secure` atrás de proxy reverso

`session.SetCookie(w, sess.ID, r.TLS != nil)` ([login.go:121](core/security/auth/login.go#L121)) decide a flag `Secure` por `r.TLS`. No deploy típico (nginx/Caddy terminando TLS na frente), `r.TLS == nil` **sempre** — o cookie de sessão de produção viaja sem `Secure`.

**Correção:** seguir o mesmo padrão do CSRF (`csrf.SetSecure(!debug)` no bootstrap): um `session.SetSecureDefault(!cfg.App.Debug)` configurado uma vez, em vez de inferir por request. O CSRF já faz certo; a sessão ficou para trás.

### S4. Bypass do anti-open-redirect com backslash

`NextURL` ([login.go:159-165](core/security/auth/login.go#L159-L165)) bloqueia `//evil.com`, mas não `/\evil.com` — browsers normalizam `\` para `/`, transformando em `//evil.com` (redirect externo).

**Correção:** rejeitar também `/\`:
```go
if next == "" || !strings.HasPrefix(next, "/") ||
    strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
```

### S5. Panic em handler do EventBus derruba o processo inteiro

`Bus.Publish` dispara `go h(payload)` sem `recover` ([events.go:27-29](core/events/events.go#L27-L29)). Um panic dentro de qualquer subscriber **mata o servidor** — o middleware `Recovery` só cobre a goroutine do request. Como o EventBus é o coração da arquitetura ("event-driven"), esse é o caminho de crash mais provável em apps reais.

**Correção:**
```go
for _, h := range handlers {
    h := h
    go func() {
        defer func() {
            if r := recover(); r != nil {
                log.Printf("events: panic em handler de %q: %v", event, r)
            }
        }()
        h(payload)
    }()
}
```

### S6. Realtime faz broadcast para TODOS os clientes conectados

`sendDOM` itera todos os clientes do Hub ([realtime.go:28-36](core/realtime/realtime.go#L28-L36)). Se o dev fizer `fw.Realtime.Replace("saldo", htmlComSaldoDoUsuario)`, **todos os usuários conectados recebem o saldo** — vazamento de dados entre usuários é o acidente mais fácil de cometer com a API atual.

**Correção:** manter o broadcast (útil para dados públicos), mas adicionar variantes com escopo — ex.: `ReplaceFor(sessionID, target, html)` — associando o client à sessão no upgrade do WebSocket. Documentar explicitamente que as variantes atuais são broadcast global.

### S7. WebSocket: biblioteca em manutenção mínima e clientes caem a cada 60 s

- `golang.org/x/net/websocket` é oficialmente desaconselhado pelo próprio Go team ("lacks nearly all newer features") — sem ping/pong, sem compressão, sem controle fino de frames.
- `readPump` impõe deadline de 60 s ([client.go:56](core/realtime/client.go#L56)); browsers não enviam nada espontaneamente, então **toda conexão idle cai após 60 s** — e o `liveScript` injetado não tem `onclose`/reconexão ([render.go:41-62](core/render/render.go#L41-L62)). O "Realtime invisível" morre silenciosamente depois de 1 minuto de inatividade.

**Correção:** migrar para `github.com/coder/websocket` (sucessor do nhooyr.io, sem dependências) ou `gorilla/websocket`; implementar ping do servidor a cada ~30 s renovando o deadline; adicionar reconexão com backoff no `liveScript`.

### S8. `AllowedHosts` nunca casa hosts IPv6

O strip de porta usa `strings.LastIndex(host, ":")` ([middleware.go:31-33](core/security/middleware/middleware.go#L31-L33)): para `[::1]:8000` sobra `[::1]` (com colchetes) e para `[::1]` sem porta o corte destrói o endereço. O `isLocalhost` do mesmo arquivo já faz `strings.Trim(ip, "[]")` — o `AllowedHosts` ficou sem. Usar `net.SplitHostPort` com fallback resolve os dois casos.

### S9. `Session.Values` sem lock próprio — data race

Handlers leem/escrevem `sess.Values` diretamente (ex.: [login.go:118-119](core/security/auth/login.go#L118-L119)); o `RWMutex` do Store protege só os mapas do Store, não o conteúdo da sessão. Duas requests paralelas do mesmo usuário escrevendo em `Values` é um data race real (crash potencial com map concurrent write).

**Correção:** métodos `sess.Get(k)` / `sess.Set(k, v)` com mutex interno, mantendo `Values` como detalhe privado.

### S10. Directory listing habilitado em `/statics/`

`http.FileServer` sobre `multiStatic` ([static.go:45-55](core/render/static.go#L45-L55)) serve listagem de diretório quando não há `index.html` — enumeração de todos os assets do projeto. Correção padrão: no `Open`, se o arquivo for diretório, retornar `os.ErrNotExist` (ou verificar `index.html`).

### S11. Estado de auth/sessão só em memória — documentar o limite

Sessões ([session.go](core/security/session/session.go)), revogação de JWT ([auth.go:27](core/security/auth/auth.go#L27)) e cache vivem no processo. Consequências: deploy = logout geral; duas réplicas atrás de load balancer = sessões e revogações inconsistentes (um token revogado na instância A continua válido na B). Para o estágio atual é aceitável — mas deveria estar destacado no USE.md, e a interface `session.Store`/`cache` deveria ser extraível para permitir backend Redis futuramente sem quebrar API.

### Observações menores de segurança

- **CSRF global vs APIs JSON**: `csrf.Middleware` é aplicado a tudo ([bootstrap.go:89](core/bootstrap/bootstrap.go#L89)); clientes de API puros (Bearer token, sem cookies) recebem 403 em POST. Vale um mecanismo de exempt por rota/prefixo (ex.: `r.ExemptCSRF("/api/")`) — hoje o dev não tem saída documentada.
- **`environment.Load` faz `os.Setenv`** de tudo ([environment.go:61](core/environment/environment.go#L61)) — segredos vazam para o ambiente de processos filhos (Air, comandos exec). Considerar manter só no mapa interno.
- **Página de erro 400 do AllowedHosts** usa `http.Error` plano em vez de `kyerrors.Render` — inconsistência cosmética.
- O cookie CSRF com `HttpOnly: false` ([csrf.go:100](core/security/csrf/csrf.go#L100)) é seguro no design atual (o token submetido é o HMAC, que o JS não sabe calcular), mas merece um comentário explicando — parece bug à primeira vista.

---

## 🟡 Desempenho

### P1. `LIMIT`/`OFFSET` como placeholders (ligado ao C4)

Além de corrigir o vazamento de statements, parametrizar `LIMIT`/`OFFSET` faz o cache de prepared statements realmente funcionar para paginação — hoje cada página é um parse+plan novo no banco. É provavelmente o maior ganho de desempenho disponível no caminho ORM.

### P2. `Paginate` sempre executa `COUNT(*)`

[query.go:266](core/orm/query.go#L266) roda um `COUNT(*)` (custoso em tabelas grandes no PostgreSQL) em toda chamada. Sugestões: método `PaginateNoCount` (ou opção) que busca `pageSize+1` linhas e infere `HasNext`; para admin/dashboards o count exato importa, para feeds infinitos não.

### P3. `Vary: Accept-Encoding` ausente no middleware de compressão

[compress.go](core/compress/compress.go) não envia `Vary: Accept-Encoding` — um proxy/CDN pode cachear a versão gzip e entregá-la a um cliente que não aceita gzip (corretude), ou cachear a versão plana para todos (desperdício). Também vale: pular compressão para respostas já comprimidas (imagens, woff2) e corpos pequenos (< ~1 KB, onde gzip aumenta o payload).

### P4. Cache sem limite de tamanho e com `time.Tick`

[cache.go](core/cache/cache.go): sem número máximo de entradas, um padrão de chave por-usuário/por-query cresce sem teto (DoS de memória lento). O `time.Tick` no GC nunca é liberado e o intervalo fixo de 1 min é desconectado dos TTLs. Sugestões: limite máximo com eviction (aleatória já basta), `time.NewTicker` + canal de shutdown, e `Len()` exposto no debug dashboard (já existe — bom).

### P5. `GOMAXPROCS(workers)` — conceito que pode piorar o desempenho

[bootstrap.go:146](core/bootstrap/bootstrap.go#L146) mapeia `SERVER_WORKERS` para `GOMAXPROCS`. Diferente do Gunicorn/uWSGI (de onde a analogia vem), limitar GOMAXPROCS abaixo do número de CPUs **reduz** o throughput em Go — o runtime já escala sozinho. O default do `.env.example` (`SERVER_WORKERS=4`) faz uma máquina de 12 threads usar 4. Sugestão: remover o conceito, ou renomear/documentar como "CPU limit" e recomendar omitir; em containers, considerar `automaxprocs` (respeita cgroup quota).

### P6. Micro-otimizações no hot path

- `secureHeadersMap` é iterado como map a cada request ([middleware.go:98-115](core/security/middleware/middleware.go#L98-L115)) — trocar por slice de pares `[7][2]string` (ordem estável, sem hash walk). Micro, mas é executado em 100% das requests de produção.
- `csrf.sign` recria o HMAC por request ([csrf.go:106](core/security/csrf/csrf.go#L106)) — inevitável (HMAC não é reutilizável concorrentemente), ok.
- O hack `gid()` via `runtime.Stack` ([funcs.go:13-20](core/render/funcs.go#L13-L20)) custa ~1-2 µs por render e quebra se um template func rodar em outra goroutine. Funciona, mas a alternativa robusta é injetar o valor do token no `merged` map do render (o processor de CSRF já poderia colocar `csrf_token` como dado) e eliminar o registro por goroutine.

### P7. HTTP/2 indisponível

`ListenAndServe` sem TLS ([bootstrap.go:189](core/bootstrap/bootstrap.go#L189)) = HTTP/1.1 apenas. Atrás de proxy isso é irrelevante (o proxy fala h2 com o cliente), mas vale documentar o deploy recomendado; se o Kyrux for exposto direto, oferecer `ListenAndServeTLS` configurável via `.env` (e aí o `r.TLS != nil` do S3 também passa a funcionar).

### P8. Pooling de `Context` — risco documentacional

O `Context` volta ao pool ao fim do request ([router.go:100-110](core/router/router.go#L100-L110)). Se um dev guardar `ctx` numa goroutine (`go func() { usar(ctx) }()`) — padrão comum vindo de Django/Celery — terá use-after-free lógico com dados de outro request. O USE.md deveria avisar em destaque ("nunca use ctx fora do handler"), e/ou oferecer `ctx.Copy()` como o Gin faz.

---

## 🟢 Pontos fortes (manter como estão)

- **CSRF**: double-submit com HMAC-SHA256 e `subtle.ConstantTimeCompare` — acima do padrão de frameworks pequenos.
- **Argon2id com parâmetros OWASP + pepper**, formato PHC parseado corretamente.
- **AES-256-GCM com nonce aleatório por operação** e prefixo idempotente `$enc$`.
- **Session fixation tratado** no login (sessão anônima destruída).
- **Open redirect no `?next=`** já considerado (só falta o caso S4).
- **ORM valida identificadores** (`Select`/`OrderBy`/colunas de `Update`) e exige `WHERE` em UPDATE/DELETE.
- **Pools por toda parte** (ctx, params, buffers, gzip, JSON encoder) — o benchmark de 127k req/s confirma que o hot path está limpo.
- **Debug dashboard restrito a localhost** e rotas internas fora da listagem pública.
- **Timeouts do http.Server configurados** (Read/Write/Idle/MaxHeaderBytes) — muita gente esquece.

---

## Priorização sugerida

| # | Item | Impacto | Esforço |
|---|------|---------|---------|
| 1 | C1 — unificar config de banco (hoje o boot quebra) | Crítico | Médio |
| 2 | C2 — CSP vs script Realtime inline | Crítico | Baixo |
| 3 | C3 — fail-open de hash/encrypt no Update | Crítico | Baixo |
| 4 | S5 — recover nos handlers do EventBus | Alto (crash) | Baixo |
| 5 | C4 + P1 — stmt leak + LIMIT/OFFSET parametrizado | Alto | Médio |
| 6 | S1 — semáforo Argon2 + middleware RateLimit | Alto | Médio |
| 7 | S3 — flag Secure da sessão via settings | Alto | Baixo |
| 8 | S4 — backslash no NextURL | Médio | Trivial |
| 9 | S7 — trocar lib WebSocket + ping/reconexão | Alto (recurso-chave) | Médio |
| 10 | S6 — Realtime com escopo por sessão | Médio | Médio |^^
| 11 | S2 — timing no login (hash dummy) | Médio | Baixo |
| 12 | S9 — lock em Session.Values | Médio | Baixo |
| 13 | S8, S10, P3, P4 | Médio | Baixo |
| 14 | P2, P5, P6, P7, P8 e docs | Baixo/Médio | Variado |

Os itens 1-4 e 7-8 cabem numa única rodada de correções sem tocar em nenhuma API pública; o restante pode ser distribuído nas próximas iterações do Alpha.
