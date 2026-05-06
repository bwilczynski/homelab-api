package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	keyfunc "github.com/MicahParks/keyfunc/v3"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v3"
	jwt "github.com/golang-jwt/jwt/v5"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
	"github.com/bwilczynski/homelab-api/internal/auth"
	"github.com/bwilczynski/homelab-api/internal/backups"
	"github.com/bwilczynski/homelab-api/internal/config"
	"github.com/bwilczynski/homelab-api/internal/containers"
	"github.com/bwilczynski/homelab-api/internal/health"
	"github.com/bwilczynski/homelab-api/internal/network"
	"github.com/bwilczynski/homelab-api/internal/storage"
	"github.com/bwilczynski/homelab-api/internal/system"
)

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

	monitor := health.NewMonitor(buildHealthCheckers(synologyClients, unifiClients), 30*time.Second, logger)
	go monitor.Start(context.Background())

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

	// Containers: all Synology backends; capability checked per-request via SupportsContainers.
	containerBackends := make(map[string]containers.ContainerBackend, len(synologyClients))
	for name, client := range synologyClients {
		containerBackends[name] = client
	}
	containersSvc := containers.NewService(containerBackends, monitor)
	containers.HandlerWithOptions(containers.NewStrictHandler(containers.NewHandler(containersSvc), nil), containers.ChiServerOptions{
		BaseRouter:       protected,
		Middlewares:      []containers.MiddlewareFunc{scopeMw},
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	// Storage: all Synology backends.
	storageBackends := make(map[string]storage.StorageBackend, len(synologyClients))
	for name, client := range synologyClients {
		storageBackends[name] = client
	}
	storageSvc := storage.NewService(storageBackends, monitor)
	storage.HandlerWithOptions(storage.NewStrictHandler(storage.NewHandler(storageSvc), nil), storage.ChiServerOptions{
		BaseRouter:       protected,
		Middlewares:      []storage.MiddlewareFunc{scopeMw},
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	// Backups: all Synology backends; capability checked per-request via SupportsBackups.
	backupBackends := make(map[string]backups.BackupBackend, len(synologyClients))
	for name, client := range synologyClients {
		backupBackends[name] = client
	}
	backupsSvc := backups.NewService(backupBackends, monitor)
	backups.HandlerWithOptions(backups.NewStrictHandler(backups.NewHandler(backupsSvc), nil), backups.ChiServerOptions{
		BaseRouter:       protected,
		Middlewares:      []backups.MiddlewareFunc{scopeMw},
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	// Network: all UniFi backends.
	networkBackends := make(map[string]network.UniFiBackend, len(unifiClients))
	for name, client := range unifiClients {
		networkBackends[name] = client
	}
	networkSvc := network.NewService(networkBackends, monitor)
	network.HandlerWithOptions(network.NewStrictHandler(network.NewHandler(networkSvc), nil), network.ChiServerOptions{
		BaseRouter:       protected,
		Middlewares:      []network.MiddlewareFunc{scopeMw},
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	addr := ":8080"
	logger.Info("starting server", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}
