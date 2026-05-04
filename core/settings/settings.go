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
	Database      DatabaseSettings
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
	SecretKey   string
	SessionTTL  int
	AllowedHost []string
}

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

func Load() *Settings {
	return &Settings{
		InstalledApps: []string{},
		App: AppSettings{
			Name:    "kyrux",
			Version: "0.1.0",
			Env:     environment.GetOr("APP_ENV", "production"),
			Debug:   environment.GetOr("APP_ENV", "production") == "development",
		},
		Server: ServerSettings{
			Host:    environment.GetOr("SERVER_HOST", "0.0.0.0"),
			Port:    environment.GetOr("SERVER_PORT", "8000"),
			Workers: intOr(environment.Get("SERVER_WORKERS"), runtime.NumCPU()),
		},
		Database: DatabaseSettings{
			Enabled: environment.GetOr("DB_ENABLED", "false") == "true",
			Driver:  environment.GetOr("DB_DRIVER", "postgres"),
			DSN:     environment.Get("DB_DSN"),
		},
		Cache: CacheSettings{
			Enabled: environment.GetOr("CACHE_ENABLED", "false") == "true",
			Driver:  environment.Get("CACHE_DRIVER"),
			Addr:    environment.Get("CACHE_ADDR"),
		},
		Security: SecuritySettings{
			SecretKey:   environment.GetOr("SECRET_KEY", "change-me"),
			SessionTTL:  3600,
			AllowedHost: parseHosts(environment.Get("ALLOWED_HOSTS")),
		},
	}
}
