package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	keyfunc "github.com/MicahParks/keyfunc/v3"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v3"
	jwt "github.com/golang-jwt/jwt/v5"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
	"github.com/bwilczynski/homelab-api/internal/auth"
	"github.com/bwilczynski/homelab-api/internal/config"
	"github.com/bwilczynski/homelab-api/internal/docker"
	"github.com/bwilczynski/homelab-api/internal/health"
	"github.com/bwilczynski/homelab-api/internal/network"
	"github.com/bwilczynski/homelab-api/internal/storage"
	"github.com/bwilczynski/homelab-api/internal/system"
)

// Injected at build time via -ldflags; defaults to "dev" for local runs.
var (
	apiVersion    = "dev"
	serverVersion = "dev"
)

const shutdownTimeout = 10 * time.Second

func main() {
	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(os.Stdout, nil)
	} else {
		handler = slog.NewTextHandler(os.Stdout, nil)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	var jwkKeyFunc jwt.Keyfunc
	if cfg.Auth.Enabled {
		jwksURL := cfg.Dex.URL + "/dex/keys"
		k, err := keyfunc.NewDefault([]string{jwksURL})
		if err != nil {
			logger.Error("failed to initialize JWKS", "err", err)
			os.Exit(1)
		}
		jwkKeyFunc = k.Keyfunc
		logger.Info("authorization enabled", "issuer", cfg.Auth.Issuer)
	}

	jwtMw := auth.JWTMiddleware(cfg.Auth, jwkKeyFunc)
	scopeMw := auth.ScopeMiddleware(cfg.Auth)

	synologyClients, unifiClients := buildClients(cfg, logger)
	logger.Info("backends configured", "synology", len(synologyClients), "unifi", len(unifiClients))
	for _, b := range cfg.Backends {
		logger.Info("backend registered", "name", b.Name, "type", string(b.Type), "host", b.Host)
	}

	discoverAPIs(synologyClients)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	monitor := health.NewMonitor(buildHealthCheckers(synologyClients, unifiClients), 30*time.Second, logger)
	var wg sync.WaitGroup
	wg.Go(func() {
		monitor.Start(ctx)
	})

	r := chi.NewRouter()
	r.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))

	r.Get("/.well-known/homelab", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		type response struct {
			Enabled bool   `json:"enabled"`
			Issuer  string `json:"issuer,omitempty"`
		}
		_ = json.NewEncoder(w).Encode(response{
			Enabled: cfg.Auth.Enabled,
			Issuer:  cfg.Auth.Issuer,
		})
	})

	r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		type response struct {
			APIVersion    string `json:"apiVersion"`
			ServerVersion string `json:"serverVersion"`
		}
		_ = json.NewEncoder(w).Encode(response{
			APIVersion:    apiVersion,
			ServerVersion: serverVersion,
		})
	})

	if cfg.Dex.URL != "" {
		dexProxy := auth.DexProxy(cfg.Dex.URL)
		r.Handle("/dex", dexProxy)
		r.Handle("/dex/*", dexProxy)
	}

	// Protected router — all domain routes require a valid JWT.
	protected := r.With(jwtMw)

	// System: all DSM + all UniFi backends.
	dsmBackends := make(map[string]system.DSMBackendConfig, len(synologyClients))
	for name, client := range synologyClients {
		dsmBackends[name] = system.DSMBackendConfig{
			Backend:       client,
			DockerEnabled: client.SupportsContainers(),
		}
	}
	unifiBackends := make(map[string]system.UniFiBackend, len(unifiClients))
	for name, client := range unifiClients {
		unifiBackends[name] = client
	}
	systemSvc := system.NewService(dsmBackends, unifiBackends, cfg.Updates, logger, monitor)
	system.HandlerWithOptions(system.NewStrictHandler(system.NewHandler(systemSvc), nil), system.ChiServerOptions{
		BaseRouter:       protected,
		Middlewares:      []system.MiddlewareFunc{scopeMw},
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	// Docker: all Synology backends; capability checked per-request via SupportsContainers.
	dockerBackends := make(map[string]docker.DockerBackend, len(synologyClients))
	for name, client := range synologyClients {
		dockerBackends[name] = client
	}
	dockerSvc := docker.NewService(dockerBackends, logger, monitor)
	docker.HandlerWithOptions(docker.NewStrictHandler(docker.NewHandler(dockerSvc), nil), docker.ChiServerOptions{
		BaseRouter:       protected,
		Middlewares:      []docker.MiddlewareFunc{scopeMw},
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	// Storage: all Synology backends (volumes + backups).
	storageBackends := make(map[string]storage.StorageBackend, len(synologyClients))
	storageBackupBackends := make(map[string]storage.BackupBackend, len(synologyClients))
	for name, client := range synologyClients {
		storageBackends[name] = client
		storageBackupBackends[name] = client
	}
	storageSvc := storage.NewService(storageBackends, storageBackupBackends, logger, monitor)
	storage.HandlerWithOptions(storage.NewStrictHandler(storage.NewHandler(storageSvc), nil), storage.ChiServerOptions{
		BaseRouter:       protected,
		Middlewares:      []storage.MiddlewareFunc{scopeMw},
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	// Network: all UniFi backends.
	networkBackends := make(map[string]network.UniFiBackend, len(unifiClients))
	for name, client := range unifiClients {
		networkBackends[name] = client
	}
	historyDays := 30
	for _, b := range cfg.ByType(config.BackendTypeUniFi) {
		if b.ClientHistoryDays > 0 {
			historyDays = b.ClientHistoryDays
			break
		}
	}
	networkSvc := network.NewService(networkBackends, historyDays, logger, monitor)
	network.HandlerWithOptions(network.NewStrictHandler(network.NewHandler(networkSvc), nil), network.ChiServerOptions{
		BaseRouter:       protected,
		Middlewares:      []network.MiddlewareFunc{scopeMw},
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.ListenAndServe()
	}()
	logger.Info("starting server", "addr", server.Addr)

	exitCode := 0
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, stopping server")
	case err := <-serverErrCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "err", err)
			exitCode = 1
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}

	stop()
	wg.Wait()
	logger.Info("shutdown complete")

	if exitCode != 0 {
		os.Exit(exitCode)
	}
}
