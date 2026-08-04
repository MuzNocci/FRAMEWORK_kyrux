package benchmark_test

// Layer 3 — Throughput: rotas reais da aplicação
//
// Usa bootstrap.Init() para carregar o app completo (DB, templates, middlewares)
// e descobre todas as rotas GET via fw.Router.Routes(). Cada rota é testada
// individualmente via TCP real durante 5 segundos.
//
// Duas correções em relação a uma versão anterior deste teste, as duas
// descobertas medindo os números com desconfiança em vez de aceitar valores
// que "pareciam" corretos:
//
//  1. As rotas de admin (exceto /admin/login/) exigem sessão autenticada
//     (requireStaff) — sem login, todo request é redirecionado pra
//     /admin/login/, e como http.Client segue redirect por padrão e reporta
//     o status FINAL, o teste estava medindo o custo de renderizar a
//     página de LOGIN pra toda rota protegida, não a rota de verdade. Este
//     teste agora autentica de verdade (loginAsAdmin) antes de medir essas
//     rotas.
//  2. Sem cookiejar, cada requisição chegava sem o cookie CSRF — o
//     middleware gerava um token novo via crypto/rand a cada vez (custo
//     real, e ruído entre rodadas) em vez de reaproveitar um, como um
//     cliente de verdade faria. Todo client usado neste teste agora carrega
//     um cookiejar persistente.
//
// Além disso, registra um model real no admin (com dados seedados) antes
// do bootstrap — sem isso, /admin/<slug>/ e /admin/<slug>/<pk>/ não
// existiriam nas rotas descobertas (0 models = admin sem CRUD nenhum), e o
// benchmark não estaria testando nada representativo de uso real.
//
// Rotas com parâmetros de path recebem valores de exemplo definidos em
// testParamByName e testParamByType. Ajuste esses mapas se alguma rota
// precisar de um ID ou slug específico que exista no banco.
//
// Uso:
//
//	go test ./core/router/benchmark/ -run TestThroughputReal -v -count=1

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kyrux/core/admin"
	"kyrux/core/bootstrap"
	"kyrux/core/database"
	"kyrux/core/orm"
	"kyrux/core/render"
	"kyrux/core/security/auth"

	_ "github.com/lib/pq"
	_ "kyrux/core/apps"
)

// testParamByName substitui parâmetros pelo nome da variável.
// Tem prioridade sobre testParamByType.
var testParamByName = map[string]string{
	// Exemplo: "id": "1", "slug": "meu-post"
}

// testParamByType substitui parâmetros pelo tipo detectado.
// Usado quando o nome não está em testParamByName.
var testParamByType = map[string]string{
	"path": "example",
}

// reDisplayParam captura <nome> e <nome:tipo> retornados por displayPath.
var reDisplayParam = regexp.MustCompile(`<([a-zA-Z_][a-zA-Z0-9_]*)(?::([a-zA-Z]+))?>`)

// resolveTestURL substitui parâmetros de path por valores de exemplo concretos.
// Ex: /posts/<slug>/ → /posts/example/
//
//	/files/<arquivo:path>/ → /files/example/
func resolveTestURL(path string) string {
	return reDisplayParam.ReplaceAllStringFunc(path, func(m string) string {
		sub := reDisplayParam.FindStringSubmatch(m)
		name, typ := sub[1], sub[2]
		if v, ok := testParamByName[name]; ok {
			return v
		}
		if v, ok := testParamByType[typ]; ok {
			return v
		}
		return "1"
	})
}

// ── model real de benchmark pro admin ────────────────────────────────────

const (
	throughputBenchSlug      = "throughput-bench"
	throughputBenchTable     = "throughput_bench_produtos"
	throughputBenchSeedCount = 200
)

type throughputBenchProduto struct {
	ID    int64 `kyrux:"pk"`
	Nome  string
	Preco float64
}

// registerThroughputBenchModel registra o model no admin — precisa
// acontecer ANTES de bootstrap.Init (admin.Mount lê o registry nesse
// momento). O recover cobre reexecução no mesmo processo (ex: -count=2):
// registrar o mesmo slug duas vezes é panic por design do pacote admin.
func registerThroughputBenchModel() {
	defer func() { _ = recover() }()
	admin.Register[throughputBenchProduto](throughputBenchSlug, "Produtos (benchmark)")
}

// seedThroughputBenchTable cria e popula a tabela do model de benchmark —
// só Postgres/pgx (mesma convenção do resto da suíte). Idempotente: dropa
// e recria a cada execução.
func seedThroughputBenchTable(db *database.DB) error {
	if _, err := db.Exec("DROP TABLE IF EXISTS " + throughputBenchTable); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE ` + throughputBenchTable + ` (
		id BIGSERIAL PRIMARY KEY,
		nome VARCHAR(120) NOT NULL DEFAULT '',
		preco DOUBLE PRECISION NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("create table: %w", err)
	}
	seed := make([]*throughputBenchProduto, 0, throughputBenchSeedCount)
	for i := 0; i < throughputBenchSeedCount; i++ {
		seed = append(seed, &throughputBenchProduto{Nome: fmt.Sprintf("Produto %d", i), Preco: float64(i) * 1.5})
	}
	return orm.CreateAll(db, seed)
}

// ── usuário staff exclusivo do benchmark ─────────────────────────────────
//
// Não reaproveita ADMIN_SUPERUSER_USERNAME/PASSWORD do .env nem
// auth.EnsureSuperuser: esse mecanismo nunca redefine a senha de uma conta
// já existente (proteção correta em produção — evita reset silencioso a
// cada boot), mas isso significa que não há garantia de qual senha está
// de fato salva numa conta "admin" que já existia no banco antes deste
// teste. Um usuário dedicado, sempre recriado do zero com senha conhecida,
// evita essa dependência de estado externo.

const (
	throughputBenchStaffUsername = "kyrux_throughput_bench"
	throughputBenchStaffPassword = "kyrux-throughput-bench-2026!"
)

// ensureThroughputBenchStaffUser recria (delete + create) o usuário staff
// do benchmark, garantindo senha conhecida independente do que já existia.
func ensureThroughputBenchStaffUser(db *database.DB) error {
	if err := orm.FromDB[auth.User](db).Where("username = ?", throughputBenchStaffUsername).Delete(); err != nil {
		return fmt.Errorf("remover usuário anterior: %w", err)
	}
	user := &auth.User{
		UUID:      "00000000-0000-0000-0000-000000000001",
		Username:  throughputBenchStaffUsername,
		IsAdmin:   true,
		IsStaff:   true,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := user.SetPassword(throughputBenchStaffPassword); err != nil {
		return fmt.Errorf("hash da senha: %w", err)
	}
	if err := orm.Create(db, user); err != nil {
		return fmt.Errorf("criar usuário: %w", err)
	}
	return nil
}

// ── autenticação real pras rotas protegidas do admin ─────────────────────

var reCSRFToken = regexp.MustCompile(`name="kyrux_csrf_token" value="([a-f0-9]+)"`)

// loginAsAdmin autentica client (deve ter Jar configurado — ver
// newPooledClient) como o usuário de throughputBenchStaffUsername/Password
// (ver ensureThroughputBenchStaffUser). Faz o fluxo completo de verdade:
// GET a página de login (pega o cookie CSRF e extrai o token do form
// renderizado), POST com as credenciais + token. Devolve o próprio client
// (já autenticado) se tudo der certo; nil se qualquer etapa falhar — o
// chamador decide se prossegue sem
// autenticação (medindo menos rotas) ou aborta.
func loginAsAdmin(t *testing.T, base string, client *http.Client) *http.Client {
	t.Helper()

	resp, err := client.Get(base + "/admin/login/")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Logf("loginAsAdmin: GET /admin/login/ falhou (status=%v, err=%v)", statusOf(resp), err)
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Logf("loginAsAdmin: ler corpo do login: %v", err)
		return nil
	}

	m := reCSRFToken.FindSubmatch(body)
	if m == nil {
		t.Log("loginAsAdmin: token CSRF não encontrado na página de login")
		return nil
	}

	form := url.Values{
		"login":            {throughputBenchStaffUsername},
		"password":         {throughputBenchStaffPassword},
		"kyrux_csrf_token": {string(m[1])},
	}
	resp2, err := client.PostForm(base+"/admin/login/", form)
	if err != nil {
		t.Logf("loginAsAdmin: POST /admin/login/: %v", err)
		return nil
	}
	io.Copy(io.Discard, resp2.Body) //nolint:errcheck
	resp2.Body.Close()

	// PostForm segue o redirect de sucesso (POST-redirect-GET) — se o
	// login falhar, handleLoginSubmit RE-RENDERIZA o próprio form de login
	// (sem redirect), então resp2.Request.URL ainda aponta pra
	// /admin/login/. Só considera autenticado se saiu de lá de verdade.
	if resp2.StatusCode != http.StatusOK || resp2.Request.URL.Path == "/admin/login/" {
		t.Logf("loginAsAdmin: login não confirmado (status=%d, path final=%s)", resp2.StatusCode, resp2.Request.URL.Path)
		return nil
	}
	return client
}

func statusOf(resp *http.Response) any {
	if resp == nil {
		return nil
	}
	return resp.StatusCode
}

// ── benchmark ─────────────────────────────────────────────────────────────

func TestThroughputReal(t *testing.T) {
	render.AppsDir = "../../../apps"

	// Força production antes de environment.Load() — o package respeita vars já definidas.
	// Valores fixos garantem que as verificações de segurança do bootstrap passam.
	os.Setenv("APP_ENV", "production")
	os.Setenv("SECRET_KEY", "kyrux-benchmark-secret-key-placeholder-32")
	os.Setenv("PASSWORD_PEPPER", "kyrux-benchmark-pepper-placeholder-32ch")

	// Precisa acontecer antes de bootstrap.Init — admin.Mount lê o registry
	// nesse momento; depois disso, registrar não teria efeito nas rotas.
	registerThroughputBenchModel()

	fw, err := bootstrap.Init("../../../.env")
	if err != nil {
		t.Fatalf("bootstrap.Init: %v", err)
	}

	// Popula o model de benchmark — sem isso, /admin/throughput-bench/ e
	// /admin/throughput-bench/<pk>/ existiriam nas rotas mas seriam listas
	// vazias, não representativas de uso real.
	adminBenchReady := false
	if db := fw.DB.Use(); db != nil && db.Ping() == nil && (db.Driver == "postgres" || db.Driver == "pgx") {
		if err := seedThroughputBenchTable(db); err != nil {
			t.Fatalf("seed do model de benchmark: %v", err)
		}
		if err := ensureThroughputBenchStaffUser(db); err != nil {
			t.Fatalf("criar usuário staff de benchmark: %v", err)
		}
		adminBenchReady = true
		defer db.Exec("DROP TABLE IF EXISTS " + throughputBenchTable)
		defer orm.FromDB[auth.User](db).Where("username = ?", throughputBenchStaffUsername).Delete()
		testParamByName["slug"] = throughputBenchSlug
		testParamByName["pk"] = "1"
	} else {
		t.Log("banco indisponível ou driver != postgres/pgx — pulando seed do model de benchmark (rotas /admin/<slug>/ ficam sem dado real)")
	}

	// Se habilitado, captura perfis CPU/heap via runtime/pprof.
	if os.Getenv("ENABLE_PPROF") == "1" {
		// CPU
		if out, err := os.Create("/tmp/kyrux_cpu.pprof"); err == nil {
			if err := pprof.StartCPUProfile(out); err == nil {
				defer func() {
					pprof.StopCPUProfile()
					out.Close()
				}()
			} else {
				out.Close()
			}
		}
		// Heap será escrito ao final do teste
		defer func() {
			if out, err := os.Create("/tmp/kyrux_heap.pprof"); err == nil {
				pprof.WriteHeapProfile(out) //nolint:errcheck
				out.Close()
			}
		}()
	}

	type scenario struct {
		pattern   string
		url       string
		protected bool // true = exige sessão staff/admin (requireStaff)
	}

	var scenarios []scenario
	for _, r := range fw.Router.Routes() {
		if r.Method != "GET" {
			continue
		}
		protected := strings.HasPrefix(r.Path, "/admin/") && r.Path != "/admin/login/"
		scenarios = append(scenarios, scenario{
			pattern:   r.Path,
			url:       resolveTestURL(r.Path),
			protected: protected,
		})
	}

	if len(scenarios) == 0 {
		t.Skip("nenhuma rota GET registrada na aplicação")
	}

	workers := fw.Settings.Server.Workers
	prev := runtime.GOMAXPROCS(workers)
	defer runtime.GOMAXPROCS(prev)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{
		Handler:      fw.Router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()

	base := "http://" + ln.Addr().String()

	const (
		duration   = 5 * time.Second
		goroutines = 8
	)
	concurrency := workers * goroutines

	// newPooledClient monta um client com pool de conexões dimensionado pra
	// concurrency (sem isso, o default do Go — 2 idle conns por host —
	// forçaria abertura de conexão nova a cada request, medindo o custo de
	// handshake TCP em vez do handler) e um cookiejar persistente (evita
	// que o middleware CSRF gere um token novo via crypto/rand a cada
	// requisição — custo real e ruído que um cliente de verdade não paga).
	newPooledClient := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{
			Jar: jar,
			Transport: &http.Transport{
				MaxIdleConns:        concurrency,
				MaxIdleConnsPerHost: concurrency,
				IdleConnTimeout:     30 * time.Second,
			},
		}
	}

	// Autentica uma vez — todas as goroutines concorrentes de uma rota
	// protegida reusam a MESMA sessão (equivalente a um usuário staff com
	// várias abas abertas), em vez de pagar login (hash de senha) por
	// requisição, o que mediria outra coisa.
	var authedClient *http.Client
	if adminBenchReady {
		authedClient = loginAsAdmin(t, base, newPooledClient())
		if authedClient == nil {
			t.Log("autenticação falhou — rotas protegidas do admin serão puladas nesta rodada")
		}
	}

	plainClient := newPooledClient()

	clientFor := func(sc scenario) *http.Client {
		if sc.protected && authedClient != nil {
			return authedClient
		}
		return plainClient
	}

	var tested []scenario
	for _, sc := range scenarios {
		if sc.protected && authedClient == nil {
			continue // sem sessão válida, não dá pra medir a rota de verdade
		}
		tested = append(tested, sc)
	}

	for _, sc := range tested {
		c := clientFor(sc)
		for i := 0; i < workers*5; i++ {
			resp, err := c.Get(base + sc.url)
			if err == nil {
				io.Copy(io.Discard, resp.Body) //nolint:errcheck
				resp.Body.Close()
			}
		}
	}

	fmt.Printf("\n╔══════════════════════════════════════════════════════╗\n")
	fmt.Printf("║    Kyrux — Throughput (rotas reais da aplicação)     ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Workers (SERVER_WORKERS): %-4d                      ║\n", workers)
	fmt.Printf("║  Goroutines clientes:      %-4d (%d por worker)      ║\n", concurrency, goroutines)
	fmt.Printf("║  Duração por cenário:  %-4s                          ║\n", duration)
	fmt.Printf("║  Rotas testadas:       %-4d                          ║\n", len(tested))
	if adminBenchReady {
		fmt.Printf("║  Sessão admin:         %-4s                          ║\n", map[bool]string{true: "autenticada", false: "falhou"}[authedClient != nil])
	}
	fmt.Printf("╚══════════════════════════════════════════════════════╝\n\n")

	for _, sc := range tested {
		c := clientFor(sc)
		var total, errs atomic.Int64

		deadline := time.Now().Add(duration)
		var wg sync.WaitGroup

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for time.Now().Before(deadline) {
					resp, err := c.Get(base + sc.url)
					if err != nil {
						errs.Add(1)
						continue
					}
					io.Copy(io.Discard, resp.Body) //nolint:errcheck
					resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						total.Add(1)
					} else {
						errs.Add(1)
					}
				}
			}()
		}

		wg.Wait()

		rps := float64(total.Load()) / duration.Seconds()
		errRate := 0.0
		if sum := total.Load() + errs.Load(); sum > 0 {
			errRate = float64(errs.Load()) / float64(sum) * 100
		}

		fmt.Printf("  Rota    : GET %s\n", sc.pattern)
		if strings.Contains(sc.pattern, "<") {
			fmt.Printf("  URL     : GET %s\n", sc.url)
		}
		fmt.Printf("  Total   : %d requisições\n", total.Load())
		fmt.Printf("  Erros   : %d (%.2f%%)\n", errs.Load(), errRate)
		fmt.Printf("  ► Throughput: %.0f req/s\n\n", rps)

		t.Logf("[GET %s] %.0f req/s | total=%d erros=%d workers=%d concurrency=%d",
			sc.pattern, rps, total.Load(), errs.Load(), workers, concurrency)
	}
}
