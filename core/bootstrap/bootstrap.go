package bootstrap

import (
	"context"
	"fmt"
	"kyrux/core/bootstrap/assets"
	"kyrux/core/bootstrap/welcome"
	"kyrux/core/cache"
	"kyrux/core/database"
	kyerrors "kyrux/core/errors"
	"kyrux/core/environment"
	"kyrux/core/events"
	"kyrux/core/orm"
	"kyrux/core/hotreload"
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
	}

	addr := cfg.Server.Host + ":" + cfg.Server.Port
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

	dbm := orm.LoadDatabases()
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
		f.Cache = cache.New()
		log.Println("bootstrap: cache enabled")
	} else {
		log.Println("bootstrap: cache disabled (CACHE_ENABLED=false)")
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
	welcome.RegisterIfNeeded(r)

	r.Internal("GET /kyrux/websocket/ws/", func(ctx *router.Context) {
		hub.ServeHTTP(ctx.Writer, ctx.Request)
	})

	static := render.MultiStaticHandler("apps")
	r.HandlePrefix("GET /statics/", http.StripPrefix("/statics/", static))

	if cfg.App.Debug {
		lr := hotreload.NewHub()
		lr.Watch("apps", "statics")
		r.HandlePrefix("GET /__kyrux_reload__", lr)
		log.Println("bootstrap: hotreload ativo")

		r.Internal("GET /kyrux/debug/", secmiddleware.LocalhostOnly(kydebug.Handler(cfg.App.Name, cfg.App.Version, cfg.App.Env, addr, cfg.Server.Workers, r.Routes, f.DB, f.Cache)))
		log.Printf("bootstrap: debug em http://%s/kyrux/debug/\n", addr)
	}

	return f, nil
}

func (f *Framework) Run() error {
	runtime.GOMAXPROCS(f.Settings.Server.Workers)

	if gc := environment.Get("RUNTIME_GOGC"); gc != "" {
		if n, err := strconv.Atoi(gc); err == nil {
			debug.SetGCPercent(n)
			log.Printf("bootstrap: GOGC=%d\n", n)
		}
	}

	addr := f.Settings.Server.Host + ":" + f.Settings.Server.Port
	fmt.Printf("Kyrux running on http://%s (workers: %d)\n", addr, f.Settings.Server.Workers)

	writeTimeout := 10 * time.Second
	if f.Settings.App.Debug {
		writeTimeout = 0 // SSE (hotreload) precisa de conexões de longa duração
	}

	srv := &http.Server{
		Addr:           addr,
		Handler:        f.Router,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   writeTimeout,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
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
		fmt.Printf("[%s] see you again.\n", time.Now().Format("15:04:05"))
		close(done)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	<-done
	return nil
}
