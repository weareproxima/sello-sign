// Command api is Sello Sign's API (api.sign.sello.tax): it signs and
// verifies documents and manages spaces, organizations, certificates,
// delegations and keys. See contracts/README.md.
//
//	api                       serve (the default)
//	api migrate               apply the pending migrations and exit
//	api workspace create …    create a space (self-hosted, Sello's platform space)
//	api member add …          make a panel user a member of a space
//	api key create …          mint an API key and print it once
//	api health                exit 0 if this machine's server answers /health
//
// `api health` is the container's health check: its image has no shell nor
// curl.
//
// It reads its configuration from the environment and from ./.env, whose
// values never override the environment's.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/billing"
	"proxima.la/sello/sign/internal/cache"
	"proxima.la/sello/sign/internal/config"
	"proxima.la/sello/sign/internal/httpapi"
	"proxima.la/sello/sign/internal/secrets"
	"proxima.la/sello/sign/internal/signing/countries"
	"proxima.la/sello/sign/internal/store"
)

func main() {
	loadDotEnv(".env")
	args := os.Args[1:]
	var err error
	switch {
	case len(args) == 0 || args[0] == "serve":
		err = serve()
	case args[0] == "migrate":
		err = migrateOnly()
	case len(args) >= 2 && args[0] == "workspace" && args[1] == "create":
		err = workspaceCreate(args[2:])
	case len(args) >= 2 && args[0] == "member" && args[1] == "add":
		err = memberAdd(args[2:])
	case len(args) >= 2 && args[0] == "key" && args[1] == "create":
		err = keyCreate(args[2:])
	case args[0] == "health":
		err = health()
	case args[0] == "help" || args[0] == "-h" || args[0] == "--help":
		fmt.Fprint(os.Stderr, usage)
		return
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "sign api:", err)
		os.Exit(1)
	}
}

const usage = `usage:
  api [serve]
  api migrate
  api workspace create -name NAME [-country SV] [-owner PANEL_USER_ID]
  api member add -workspace ID -user PANEL_USER_ID [-role owner|admin|member]
  api key create -workspace ID [-environment test|prod] [-name NAME] [-scopes a,b] [-organization ID]
  api health
`

// loadDotEnv sets the variables of path that the environment doesn't
// have: KEY=value lines, # comments, optional quotes.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		if _, set := os.LookupEnv(k); !set {
			_ = os.Setenv(k, v)
		}
	}
}

func setupLogs(format, level string) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: l}
	var h slog.Handler = slog.NewJSONHandler(os.Stderr, opts)
	if format == "text" {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(h))
}

func serve() error {
	cfg, err := config.FromOS()
	if err != nil {
		return err
	}
	setupLogs(cfg.LogFormat, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	keyring, err := secrets.NewKeyring(cfg.SecretsKey, cfg.SecretsKeyPrevious)
	if err != nil {
		return fmt.Errorf("SIGN_SECRETS_KEY: %w", err)
	}
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()
	applied, err := store.Migrate(ctx, pool)
	if err != nil {
		return err
	}
	if len(applied) > 0 {
		slog.Info("migrations applied", "versions", applied)
	}
	db := store.New(pool)

	var keys cache.Keys
	if cfg.RedisURL != "" {
		rdb, err := cache.OpenValkey(ctx, cfg.RedisURL)
		if err != nil {
			return fmt.Errorf("valkey: %w", err)
		}
		defer rdb.Close()
		keys = cache.NewValkeyKeys(rdb)
	} else {
		slog.Warn("REDIS_URL is unset: API keys are cached in memory, so a revoked key may work on another instance for a few minutes")
		keys = cache.NewMemoryKeys()
	}

	registry := countries.Registry()
	for _, c := range cfg.Countries {
		if !slices.Contains(registry.Codes(), c) {
			slog.Warn("SIGN_COUNTRIES names a country without a signer", "country", c)
		}
	}
	signer := httpapi.NewSigner(keyring, cache.NewMaterial(1000, 10*time.Minute), registry, cfg.Countries, db.ActiveMaterial)

	d := httpapi.Deps{
		Store: db, Keys: keys, Signer: signer,
		Verifier: auth.NewJWKSVerifier(cfg.PanelURL, cfg.APIURL),
		Cloud:    cfg.Cloud, PlatformWorkspace: cfg.PlatformWorkspace,
		CORSOrigins: cfg.CORSOrigins,
	}
	if cfg.Cloud {
		if _, err := db.Workspace(ctx, cfg.PlatformWorkspace); err != nil {
			slog.Warn("SIGN_PLATFORM_WORKSPACE_ID doesn't name a space yet (api workspace create)", "error", err)
		}
	}
	if cfg.Billing() {
		client := billing.NewClient(cfg.BillingURL, cfg.BillingToken)
		d.Billing, d.BillingPushToken = client, cfg.BillingPushToken
		go billing.RunReporter(ctx, client, db, time.Now)
	} else if cfg.Cloud {
		slog.Warn("in the cloud without BILLING_URL: spaces can't be opened and usage isn't reported")
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.New(d).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	slog.Info("sign api listening", "addr", cfg.Addr, "cloud", cfg.Cloud, "countries", cfg.Countries)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// health asks the server listening on ADDR, on this machine, for /health.
// It reads only ADDR, so it works without the rest of the configuration.
func health() error {
	addr := strings.TrimSpace(os.Getenv("ADDR"))
	if addr == "" {
		addr = "127.0.0.1:4441"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("ADDR: %w", err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Get("http://" + net.JoinHostPort(host, port) + "/health")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("/health answered %d", res.StatusCode)
	}
	return nil
}

// openDB opens and migrates DATABASE_URL, for the console commands.
func openDB(ctx context.Context) (*store.Store, func(), error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, nil, errors.New("DATABASE_URL is required")
	}
	pool, err := store.Open(ctx, url)
	if err != nil {
		return nil, nil, err
	}
	if _, err := store.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, err
	}
	return store.New(pool), pool.Close, nil
}

func migrateOnly() error {
	ctx := context.Background()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is required")
	}
	pool, err := store.Open(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	applied, err := store.Migrate(ctx, pool)
	if err != nil {
		return err
	}
	fmt.Printf("applied: %v\n", applied)
	return nil
}
