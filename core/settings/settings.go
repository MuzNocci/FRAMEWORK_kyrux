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
	Security      SecuritySettings
}

type AppSettings struct {
	Name    string
	Version string
	Debug   bool
	Env     string
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
	Enabled bool
	Driver  string
	Addr    string
}

type SecuritySettings struct {
	SecretKey     string
	SessionTTL    int
	AllowedHost   []string
	Pepper        string
	EncryptionKey string
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
			Version: "0.1.0 (Alpha)",
			Env:     env,
			Debug:   env == "development",
		},
		Server: ServerSettings{
			Host:    environment.GetOr("SERVER_HOST", "127.0.0.1"),
			Port:    environment.GetOr("SERVER_PORT", "8000"),
			Workers: intOr(environment.Get("SERVER_WORKERS"), runtime.NumCPU()),
		},
		Databases: databases,
		Cache: CacheSettings{
			Enabled: environment.GetOr("CACHE_ENABLED", "false") == "true",
			Driver:  environment.Get("CACHE_DRIVER"),
			Addr:    environment.Get("CACHE_ADDR"),
		},
		Security: SecuritySettings{
			SecretKey:     environment.GetOr("SECRET_KEY", "change-me"),
			SessionTTL:    intOr(environment.Get("SESSION_TTL"), 3600),
			AllowedHost:   parseHosts(environment.Get("ALLOWED_HOSTS")),
			Pepper:        environment.Get("PASSWORD_PEPPER"),
			EncryptionKey: environment.Get("FIELD_ENCRYPTION_KEY"),
		},
	}

	if len(databases) > 0 {
		s.Database = databases[0]
	}

	return s
}
