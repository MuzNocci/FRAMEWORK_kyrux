package settings

import (
	"kyrux/core/environment"
	"runtime"
	"strconv"
	"strings"
)

type Settings struct {
	InstalledApps []string
	App           AppSettings
	Server        ServerSettings
	Database      DatabaseSettings   // primeiro banco (atalho para Databases[0])
	Databases     []DatabaseSettings // todos os bancos configurados
	Cache         CacheSettings
	Queue         QueueSettings
	Mail          MailSettings
	Admin         AdminSettings
	Security      SecuritySettings
}

type AppSettings struct {
	Name    string
	Version string
	Debug   bool
	Env     string

	// URL é a origem canônica do site (ex: "https://meusite.com.br", sem
	// barra final) — usada pelo app embutido "meta" (apps/meta) pra montar
	// URLs absolutas em robots.txt, sitemap.xml e security.txt. Vazia até
	// configurar APP_URL no .env; nesse caso esses arquivos saem sem
	// domínio (Sitemap/Canonical omitidos), o que ainda é válido mas
	// incompleto — configure antes de ir pra produção.
	URL string
}

type ServerSettings struct {
	Host    string
	Port    string
	Workers int
}

type DatabaseSettings struct {
	Name    string
	Enabled bool
	Driver  string
	DSN     string
}

type CacheSettings struct {
	Enabled  bool
	Driver   string
	Addr     string
	Password string // usado apenas com CACHE_DRIVER=redis; vazio = sem AUTH
}

type QueueSettings struct {
	Enabled  bool
	Driver   string // memory | redis
	Addr     string // endereço do broker quando driver != memory
	Password string // usado apenas com QUEUE_DRIVER=redis; vazio = sem AUTH
	Workers  int
}

type MailSettings struct {
	Enabled  bool
	Host     string
	Port     string // padrão "587" (STARTTLS); "465" usa TLS implícito automaticamente
	User     string
	Password string
}

type AdminSettings struct {
	Enabled bool
	Path    string // ex: "/admin/" — sempre com barra final

	// SuperuserUsername/SuperuserPassword criam o superusuário inicial no
	// boot, se ainda não existir ninguém com esse login — vazio desativa o
	// recurso. Nunca redefinem a senha de uma conta já existente (ver
	// auth.EnsureSuperuser).
	SuperuserUsername string
	SuperuserPassword string
}

type SecuritySettings struct {
	SecretKey     string
	SessionTTL    int
	AllowedHost   []string
	Pepper        string
	EncryptionKey string
	TrustedProxy  string // header de IP real do cliente (ex: X-Forwarded-For); vazio = sem proxy
	CSPPolicy     string // Content-Security-Policy padrão; vazio = usa secmiddleware.DefaultCSP
}

// InstalledApps é preenchido pelo core/apps/installed.go do projeto via init().
var InstalledApps []string

func intOr(s string, fallback int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return fallback
}

func parseHosts(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	hosts := make([]string, 0, len(parts))
	for _, p := range parts {
		if h := strings.TrimSpace(p); h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

func loadDatabases() []DatabaseSettings {
	blocks := environment.GetBlocks("DB_NAME")
	if len(blocks) == 0 {
		return nil
	}
	dbs := make([]DatabaseSettings, 0, len(blocks))
	for _, b := range blocks {
		dbs = append(dbs, DatabaseSettings{
			Name:    b["DB_NAME"],
			Enabled: strings.EqualFold(b["DB_ENABLED"], "true"),
			Driver:  orDefault(b["DB_DRIVER"], "postgres"),
			DSN:     b["DB_DSN"],
		})
	}
	return dbs
}

func orDefault(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

func Load() *Settings {
	env := environment.GetOr("APP_ENV", "production")
	databases := loadDatabases()

	s := &Settings{
		InstalledApps: InstalledApps,
		App: AppSettings{
			Name:    "kyrux",
			Version: "0.4.5 (Beta)",
			Env:     env,
			Debug:   env == "development",
			URL:     strings.TrimSuffix(environment.Get("APP_URL"), "/"),
		},
		Server: ServerSettings{
			Host:    environment.GetOr("SERVER_HOST", "127.0.0.1"),
			Port:    environment.GetOr("SERVER_PORT", "8000"),
			Workers: intOr(environment.Get("SERVER_WORKERS"), runtime.NumCPU()),
		},
		Databases: databases,
		Cache: CacheSettings{
			Enabled:  environment.GetOr("CACHE_ENABLED", "false") == "true",
			Driver:   environment.Get("CACHE_DRIVER"),
			Addr:     environment.Get("CACHE_ADDR"),
			Password: environment.Get("CACHE_PASSWORD"),
		},
		Queue: QueueSettings{
			Enabled:  environment.GetOr("QUEUE_ENABLED", "false") == "true",
			Driver:   environment.GetOr("QUEUE_DRIVER", "memory"),
			Addr:     environment.Get("QUEUE_ADDR"),
			Password: environment.Get("QUEUE_PASSWORD"),
			Workers:  intOr(environment.Get("QUEUE_WORKERS"), 4),
		},
		Mail: MailSettings{
			Enabled:  environment.GetOr("MAIL_ENABLED", "false") == "true",
			Host:     environment.Get("MAIL_HOST"),
			Port:     environment.GetOr("MAIL_PORT", "587"),
			User:     environment.Get("MAIL_USER"),
			Password: environment.Get("MAIL_PASSWORD"),
		},
		Admin: AdminSettings{
			Enabled:           environment.GetOr("ADMIN_ENABLED", "false") == "true",
			Path:              environment.GetOr("ADMIN_PATH", "/admin/"),
			SuperuserUsername: environment.Get("ADMIN_SUPERUSER_USERNAME"),
			SuperuserPassword: environment.Get("ADMIN_SUPERUSER_PASSWORD"),
		},
		Security: SecuritySettings{
			SecretKey:     environment.GetOr("SECRET_KEY", "change-me"),
			SessionTTL:    intOr(environment.Get("SESSION_TTL"), 3600),
			AllowedHost:   parseHosts(environment.Get("ALLOWED_HOSTS")),
			Pepper:        environment.Get("PASSWORD_PEPPER"),
			EncryptionKey: environment.Get("FIELD_ENCRYPTION_KEY"),
			TrustedProxy:  environment.Get("TRUSTED_PROXY_HEADER"),
			CSPPolicy:     environment.Get("CSP_POLICY"),
		},
	}

	if len(databases) > 0 {
		s.Database = databases[0]
	}

	return s
}
