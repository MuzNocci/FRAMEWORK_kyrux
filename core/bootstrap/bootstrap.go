package bootstrap

import (
	"context"
	"fmt"
	"kyrux/core/admin"
	"kyrux/core/bootstrap/assets"
	"kyrux/core/bootstrap/welcome"
	"kyrux/core/cache"
	"kyrux/core/database"
	kyerrors "kyrux/core/errors"
	"kyrux/core/environment"
	"kyrux/core/events"
	"kyrux/core/orm"
	"kyrux/core/hotreload"
	"kyrux/core/queue"
	"kyrux/core/realtime"
	"kyrux/core/render"
	"kyrux/core/router"
	"kyrux/core/security/auth"
	"kyrux/core/security/csrf"
	"kyrux/core/security/crypton"
	secmiddleware "kyrux/core/security/middleware"
	"kyrux/core/security/session"
	"kyrux/core/settings"
	"log"
	kydebug "kyrux/core/bootstrap/debug"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Framework struct {
	Settings *settings.Settings
	Router   *router.Router
	Events   *events.Bus
	Realtime *realtime.Hub
	DB       *database.Manager
	Cache    *cache.Cache
	Queue    *queue.Queue
	Auth     *auth.Authenticator
	Sessions *session.Store
}

func Init(envPath string) (*Framework, error) {
	if err := environment.Load(envPath); err != nil {
		return nil, fmt.Errorf("bootstrap: env: %w", err)
	}

	cfg := settings.Load()

	crypton.SetPepper(cfg.Security.Pepper)
	crypton.SetEncryptionKey(cfg.Security.EncryptionKey)
	render.SetDebug(cfg.App.Debug)
	kyerrors.SetDebug(cfg.App.Debug)
	kyerrors.SetApp(cfg.App.Name, cfg.App.Version)
	csrf.SetSecure(!cfg.App.Debug)
	csrf.SetSecret(cfg.Security.SecretKey)
	session.SetSecureDefault(!cfg.App.Debug)
	secmiddleware.SetTrustedProxyHeader(cfg.Security.TrustedProxy)

	if !cfg.App.Debug {
		if cfg.Security.SecretKey == "change-me" {
			log.Fatal("bootstrap: SECRET_KEY não definida — defina uma chave forte no .env antes de rodar em produção")
		}
		if len(cfg.Security.SecretKey) < 32 {
			log.Fatal("bootstrap: SECRET_KEY muito curta — use no mínimo 32 caracteres em produção")
		}
		if cfg.Security.Pepper == "" || cfg.Security.Pepper == "your-strong-random-pepper-here" {
			log.Fatal("bootstrap: PASSWORD_PEPPER não definida — defina um pepper forte no .env antes de rodar em produção")
		}
		// FIELD_ENCRYPTION_KEY é opcional (nem todo app usa kyrux:"encrypt"),
		// mas se ausente a criptografia fica desativada — avise, não derive
		// silenciosamente uma chave de string vazia (segredo público).
		if !crypton.HasEncryptionKey() {
			log.Println("bootstrap: AVISO — FIELD_ENCRYPTION_KEY não definida; campos kyrux:\"encrypt\" falharão ao gravar. Defina no .env se usar criptografia de campos.")
		} else if cfg.Security.EncryptionKey == "your-base64-32-byte-key-here" {
			log.Fatal("bootstrap: FIELD_ENCRYPTION_KEY ainda é o valor de exemplo — gere uma chave forte (openssl rand -base64 32) antes de rodar em produção")
		}
	}

	// addr aqui é só para EXIBIÇÃO (welcome page, debug dashboard, logs) — o
	// bind real do servidor (Run) usa Host:Port sem tradução, para continuar
	// escutando em todas as interfaces quando Host=0.0.0.0.
	addr := browsableAddr(cfg.Server.Host, cfg.Server.Port)
	render.RegisterAppFuncs(cfg.App.Name, cfg.App.Version, cfg.App.Env, addr)
	csrf.RegisterFuncs()

	bus := events.NewBus()
	hub := realtime.NewHub(bus)
	hub.SetAllowedOrigins(cfg.Security.AllowedHost)
	r := router.New()
	kyerrors.SetRouteListFunc(r.Routes)
	r.Use(secmiddleware.Recovery())
	r.Use(secmiddleware.MaxBodySize(32 << 20)) // 32 MB default
	if !cfg.App.Debug {
		r.Use(secmiddleware.SecureHeaders)
	}
	r.Use(secmiddleware.AllowedHosts(cfg.Security.AllowedHost, cfg.App.Debug))
	r.Use(csrf.Middleware)
	a := auth.New(cfg.Security.SecretKey)
	store := session.NewStore(time.Duration(cfg.Security.SessionTTL) * time.Second)

	dbm := orm.LoadDatabases(cfg.Databases)

	// Sem nenhum banco configurado, o admin não teria como autenticar
	// ninguém. Em development, o Kyrux cria um SQLite local para o admin
	// funcionar de imediato — é a ÚNICA situação em que o framework abre um
	// banco por conta própria. Em produção isso NUNCA acontece: escrever
	// silenciosamente num arquivo local efêmero em vez do banco pretendido
	// seria pior que simplesmente recusar montar o admin.
	usingFallbackDB := false
	if cfg.Admin.Enabled && !orm.HasConnections() {
		if cfg.App.Debug {
			const fallbackPath = "database/kyrux.sqlite3"
			fdb, err := database.OpenSQLiteFallback(fallbackPath)
			if err != nil {
				log.Printf("bootstrap: ADMIN_ENABLED=true sem banco configurado — falha ao criar SQLite de desenvolvimento em %s: %v\n", fallbackPath, err)
			} else {
				orm.Register("default", fdb)
				dbm.AddDB("default", fdb)
				usingFallbackDB = true
				if err := orm.EnsureSQLiteTable[auth.User](fdb); err != nil {
					log.Printf("bootstrap: falha ao preparar tabela de usuários no SQLite de desenvolvimento: %v\n", err)
				}
				log.Printf("bootstrap: nenhum banco configurado — criado SQLite de desenvolvimento para o admin em %s (apenas development; configure um banco real para produção)\n", fallbackPath)
			}
		} else {
			log.Println("bootstrap: ADMIN_ENABLED=true sem banco configurado — em produção o Kyrux não cria um SQLite automaticamente; configure um banco real no .env. O admin não será montado.")
		}
	}

	auth.SetDBEnabled(orm.HasConnections())

	f := &Framework{
		Settings: cfg,
		Router:   r,
		Events:   bus,
		Realtime: hub,
		DB:       dbm,
		Auth:     a,
		Sessions: store,
	}

	if cfg.Cache.Enabled {
		switch cfg.Cache.Driver {
		case "redis":
			rc, err := cache.NewRedis(cfg.Cache.Addr, cfg.Cache.Password)
			if err != nil {
				log.Printf("bootstrap: CACHE_DRIVER=redis mas falha ao conectar em %s: %v — usando cache em memória (não compartilhado entre réplicas)\n", cfg.Cache.Addr, err)
				f.Cache = cache.New()
			} else {
				f.Cache = rc
				log.Printf("bootstrap: cache enabled (redis @ %s)\n", cfg.Cache.Addr)
			}
		case "", "memory":
			f.Cache = cache.New()
			log.Println("bootstrap: cache enabled (memory)")
		default:
			log.Printf("bootstrap: CACHE_DRIVER=%q desconhecido — usando 'memory'\n", cfg.Cache.Driver)
			f.Cache = cache.New()
		}
	} else {
		log.Println("bootstrap: cache disabled (CACHE_ENABLED=false)")
	}

	if cfg.Queue.Enabled {
		switch cfg.Queue.Driver {
		case "redis":
			rq, err := queue.NewRedis(cfg.Queue.Addr, cfg.Queue.Password, cfg.Queue.Workers, 0)
			if err != nil {
				log.Printf("bootstrap: QUEUE_DRIVER=redis mas falha ao conectar em %s: %v — usando fila em memória (não compartilhada entre réplicas)\n", cfg.Queue.Addr, err)
				f.Queue = queue.New(cfg.Queue.Workers, 0)
			} else {
				f.Queue = rq
				log.Printf("bootstrap: queue enabled (redis @ %s, %d workers)\n", cfg.Queue.Addr, cfg.Queue.Workers)
			}
		case "", "memory":
			f.Queue = queue.New(cfg.Queue.Workers, 0)
			log.Printf("bootstrap: queue enabled (memory, %d workers)\n", cfg.Queue.Workers)
		default:
			log.Printf("bootstrap: QUEUE_DRIVER=%q desconhecido — usando 'memory'\n", cfg.Queue.Driver)
			f.Queue = queue.New(cfg.Queue.Workers, 0)
		}
	} else {
		log.Println("bootstrap: queue disabled (QUEUE_ENABLED=false)")
	}

	for _, appName := range cfg.InstalledApps {
		if fn, ok := registry[appName]; ok {
			fn(r, f)
			log.Printf("bootstrap: app '%s' registrado\n", appName)
		} else {
			log.Printf("bootstrap: app '%s' listado em InstalledApps mas não importado\n", appName)
		}
	}

	assets.Register(r)
	welcome.RegisterIfNeeded(r, cfg.App.Name, cfg.App.Version, cfg.App.Env, addr)

	// Monta /admin/ apenas se ao menos um model foi registrado via
	// admin.Register (feito pelos apps acima) E ADMIN_ENABLED=true —
	// os dois portões precisam estar abertos (opt-in em código + config).
	if cfg.Admin.Enabled {
		if usingFallbackDB {
			// Só agora (depois do loop acima) os apps já registraram seus
			// models no admin — é o primeiro momento em que dá pra criar
			// as tabelas deles no SQLite de desenvolvimento.
			if err := admin.EnsureAllTables(dbm.Use()); err != nil {
				log.Printf("bootstrap: erro ao preparar tabelas dos models do admin no SQLite de desenvolvimento: %v\n", err)
			}
		}
		// ADMIN_SUPERUSER_USERNAME/PASSWORD são opcionais — só criam a conta
		// na PRIMEIRA vez (nunca redefinem senha de uma conta já existente),
		// evitando reset silencioso a cada reinício em dev com hot reload.
		if dbm.Use() != nil && cfg.Admin.SuperuserUsername != "" && cfg.Admin.SuperuserPassword != "" {
			created, err := auth.EnsureSuperuser(dbm.Use(), cfg.Admin.SuperuserUsername, cfg.Admin.SuperuserPassword)
			if err != nil {
				log.Printf("bootstrap: falha ao garantir superusuário via ADMIN_SUPERUSER_USERNAME/PASSWORD: %v\n", err)
			} else if created {
				log.Printf("bootstrap: superusuário '%s' criado a partir do .env (ADMIN_SUPERUSER_USERNAME/PASSWORD)\n", cfg.Admin.SuperuserUsername)
			}
		}
		admin.Mount(r, dbm, store, cfg.Admin.Path, cfg.App.Name, cfg.App.Version)
	}

	r.Internal("GET /kyrux/websocket/ws/", func(ctx *router.Context) {
		hub.ServeHTTP(ctx.Writer, ctx.Request)
	})

	// Cliente Realtime como arquivo externo — inline violaria a CSP
	// `script-src 'self'` enviada por SecureHeaders em produção.
	r.Internal("GET /kyrux/js/live.js", func(ctx *router.Context) {
		h := ctx.Writer.Header()
		h.Set("Content-Type", "application/javascript; charset=utf-8")
		if cfg.App.Debug {
			h.Set("Cache-Control", "no-store")
		} else {
			h.Set("Cache-Control", "public, max-age=86400")
		}
		ctx.Writer.Write(render.LiveScriptJS())
	})

	static := render.MultiStaticHandler("apps")
	r.HandlePrefix("GET /statics/", http.StripPrefix("/statics/", static))

	if cfg.App.Debug {
		lr := hotreload.NewHub()
		lr.Watch("apps")
		r.HandlePrefix("GET /__kyrux_reload__", lr)
		log.Println("bootstrap: hotreload ativo")

		r.Internal("GET /kyrux/debug/", secmiddleware.LocalhostOnly(kydebug.Handler(cfg.App.Name, cfg.App.Version, cfg.App.Env, addr, cfg.Server.Workers, r.Routes, f.DB, f.Cache, f.Queue)))
		log.Printf("bootstrap: debug em http://%s/kyrux/debug/\n", addr)
	}

	return f, nil
}

// browsableAddr traduz o endereço de bind para um endereço que o navegador
// consiga realmente acessar. 0.0.0.0 (e variantes "escute em tudo") são
// válidos para o servidor ESCUTAR, mas não são um destino ao qual um
// navegador consiga se conectar — copiar essa URL do log resulta em
// ERR_CONNECTION_REFUSED. Aqui trocamos apenas o que é EXIBIDO ao usuário;
// o bind real do http.Server continua usando Host:Port sem alteração.
func browsableAddr(host, port string) string {
	if host == "0.0.0.0" || host == "" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return host + ":" + port
}

// parseMemLimit converte "512MiB", "2GiB", "268435456" (bytes) em int64.
// Os sufixos maiores são testados primeiro — "KiB" também termina em "B".
func parseMemLimit(s string) (int64, error) {
	s = strings.TrimSpace(s)
	mult := int64(1)
	suffixes := []struct {
		suffix string
		mult   int64
	}{
		{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"B", 1},
	}
	for _, sf := range suffixes {
		if strings.HasSuffix(s, sf.suffix) {
			mult = sf.mult
			s = strings.TrimSuffix(s, sf.suffix)
			break
		}
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("valor inválido")
	}
	return n * mult, nil
}

func (f *Framework) Run() error {
	// GOMAXPROCS só é alterado quando SERVER_WORKERS foi definido
	// explicitamente — o default do runtime Go (todos os CPUs) é o mais
	// rápido; limitar abaixo de NumCPU REDUZ o throughput.
	if environment.Get("SERVER_WORKERS") != "" {
		runtime.GOMAXPROCS(f.Settings.Server.Workers)
	}

	if gc := environment.Get("RUNTIME_GOGC"); gc != "" {
		if n, err := strconv.Atoi(gc); err == nil {
			debug.SetGCPercent(n)
			log.Printf("bootstrap: GOGC=%d\n", n)
		}
	}

	// GOMEMLIMIT dá ao GC um teto de memória — reduz ciclos sob pressão e
	// evita OOM em containers. Aceita bytes ou sufixos KiB/MiB/GiB.
	if ml := environment.Get("RUNTIME_GOMEMLIMIT"); ml != "" {
		if n, err := parseMemLimit(ml); err == nil {
			debug.SetMemoryLimit(n)
			log.Printf("bootstrap: GOMEMLIMIT=%s\n", ml)
		} else {
			log.Printf("bootstrap: RUNTIME_GOMEMLIMIT inválido (%q): %v\n", ml, err)
		}
	}

	addr := f.Settings.Server.Host + ":" + f.Settings.Server.Port
	fmt.Printf("Kyrux running on http://%s (workers: %d)\n", browsableAddr(f.Settings.Server.Host, f.Settings.Server.Port), f.Settings.Server.Workers)

	writeTimeout := 10 * time.Second
	if f.Settings.App.Debug {
		writeTimeout = 0 // SSE (hotreload) precisa de conexões de longa duração
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: f.Router,
		// ReadHeaderTimeout limita o tempo de envio dos cabeçalhos —
		// defesa direta contra Slowloris (headers enviados byte a byte).
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	quit := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-quit
		fmt.Printf("\n[%s] cleaning...\n", time.Now().Format("15:04:05"))
		fmt.Printf("signal: %s\n", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("bootstrap: shutdown: %v\n", err)
		}
		// Drena a fila de tarefas antes de encerrar — trabalho enfileirado
		// não se perde num deploy.
		if f.Queue != nil {
			f.Queue.Close()
		}
		if f.Cache != nil {
			f.Cache.Close()
		}
		fmt.Printf("[%s] see you again.\n", time.Now().Format("15:04:05"))
		close(done)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	<-done
	return nil
}
