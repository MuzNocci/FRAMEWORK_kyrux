package debug

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"kyrux/core/cache"
	"kyrux/core/database"
	kyerrors "kyrux/core/errors"
	"kyrux/core/router"
	"net/http"
	"runtime"
	"time"
)

//go:embed debug.html
var dashHTML string

var (
	dashTpl   = template.Must(template.New("debug").Funcs(template.FuncMap{"methodClass": methodClass}).Parse(dashHTML))
	startTime = time.Now()
)

type cacheInfo struct {
	Enabled bool
	Entries int
}

type dashData struct {
	AppName    string
	Version    string
	Env        string
	Addr       string
	GoVersion  string
	GOOS       string
	GOARCH     string
	Workers    int
	Goroutines int
	Uptime     string
	HeapAlloc  string
	HeapSys    string
	NumGC      uint32
	Databases  []database.ConnInfo
	Cache      cacheInfo
	Routes     []kyerrors.RouteEntry
}

func Handler(appName, version, env, addr string, workers int, routesFn func() []kyerrors.RouteEntry, dbm *database.Manager, c *cache.Cache) router.HandlerFunc {
	return func(ctx *router.Context) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)

		ci := cacheInfo{Enabled: c != nil}
		if c != nil {
			ci.Entries = c.Len()
		}

		var dbs []database.ConnInfo
		if dbm != nil {
			dbs = dbm.Info()
		}

		d := dashData{
			AppName:    appName,
			Version:    version,
			Env:        env,
			Addr:       addr,
			GoVersion:  runtime.Version(),
			GOOS:       runtime.GOOS,
			GOARCH:     runtime.GOARCH,
			Workers:    workers,
			Goroutines: runtime.NumGoroutine(),
			Uptime:     fmtDuration(time.Since(startTime)),
			HeapAlloc:  fmtBytes(ms.HeapAlloc),
			HeapSys:    fmtBytes(ms.HeapSys),
			NumGC:      ms.NumGC,
			Databases:  dbs,
			Cache:      ci,
			Routes:     routesFn(),
		}

		var buf bytes.Buffer
		if err := dashTpl.Execute(&buf, d); err != nil {
			http.Error(ctx.Writer, "debug: "+err.Error(), http.StatusInternalServerError)
			return
		}
		ctx.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		ctx.Writer.Header().Set("Cache-Control", "no-store")
		ctx.Writer.Write(buf.Bytes())
	}
}

func methodClass(m string) string {
	switch m {
	case "GET":
		return "m-get"
	case "POST":
		return "m-post"
	case "PUT":
		return "m-put"
	case "DELETE":
		return "m-delete"
	case "PATCH":
		return "m-patch"
	default:
		return "m-other"
	}
}

func fmtBytes(b uint64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
