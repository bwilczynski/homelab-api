package main

import (
	"log/slog"
	"sync"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/config"
)

// buildClients creates Synology and UniFi clients from a loaded config.
func buildClients(cfg *config.Config, logger *slog.Logger) (map[string]*adapters.SynologyClient, map[string]*adapters.UniFiClient) {
	synologyClients := make(map[string]*adapters.SynologyClient)
	unifiClients := make(map[string]*adapters.UniFiClient)
	for _, b := range cfg.Backends {
		switch b.Type {
		case config.BackendTypeSynology:
			loc := time.Local
			if b.Timezone != "" {
				if l, err := time.LoadLocation(b.Timezone); err != nil {
					logger.Warn("invalid timezone, falling back to server local TZ", "backend", b.Name, "timezone", b.Timezone, "error", err)
				} else {
					loc = l
				}
			}
			synologyClients[b.Name] = adapters.NewSynologyClient(b.Name, b.Host, b.Username, b.Password, b.AuthVersion, b.InsecureTLS, logger, loc)
		case config.BackendTypeUniFi:
			unifiClients[b.Name] = adapters.NewUniFiClient(b.Host, b.Username, b.Password, b.InsecureTLS)
		}
	}
	return synologyClients, unifiClients
}

// discoverAPIs runs API discovery on all Synology clients in parallel.
// Each client logs its own outcome and retries automatically via Ping when the backend recovers.
func discoverAPIs(clients map[string]*adapters.SynologyClient) {
	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func(client *adapters.SynologyClient) {
			defer wg.Done()
			_ = client.DiscoverAPIs()
		}(client)
	}
	wg.Wait()
}

// buildHealthCheckers combines Synology and UniFi clients into a health checker map.
func buildHealthCheckers(
	synology map[string]*adapters.SynologyClient,
	unifi map[string]*adapters.UniFiClient,
) map[string]adapters.HealthChecker {
	checkers := make(map[string]adapters.HealthChecker, len(synology)+len(unifi))
	for name, c := range synology {
		checkers[name] = c
	}
	for name, c := range unifi {
		checkers[name] = c
	}
	return checkers
}
