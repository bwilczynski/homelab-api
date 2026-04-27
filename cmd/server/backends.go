package main

import (
	"log/slog"
	"sync"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/config"
)

// buildClients creates Synology and UniFi clients from a loaded config.
func buildClients(cfg *config.Config) (map[string]*adapters.SynologyClient, map[string]*adapters.UniFiClient) {
	synologyClients := make(map[string]*adapters.SynologyClient)
	unifiClients := make(map[string]*adapters.UniFiClient)
	for _, b := range cfg.Backends {
		switch b.Type {
		case config.BackendTypeSynology:
			synologyClients[b.Name] = adapters.NewSynologyClient(b.Host, b.Username, b.Password, b.AuthVersion, b.InsecureTLS)
		case config.BackendTypeUniFi:
			unifiClients[b.Name] = adapters.NewUniFiClient(b.Host, b.Username, b.Password, b.InsecureTLS)
		}
	}
	return synologyClients, unifiClients
}

// discoverAPIs runs API discovery on all Synology clients in parallel.
// On failure, each client marks itself as having no capabilities until it comes back online.
func discoverAPIs(logger *slog.Logger, clients map[string]*adapters.SynologyClient) {
	var wg sync.WaitGroup
	for name, client := range clients {
		wg.Add(1)
		go func(name string, client *adapters.SynologyClient) {
			defer wg.Done()
			if err := client.DiscoverAPIs(); err != nil {
				logger.Warn("API discovery failed; capabilities unavailable until backend is reachable",
					"backend", name, "err", err)
			} else {
				logger.Info("API discovery complete",
					"backend", name,
					"docker", client.SupportsAPI(adapters.APISynoDockerContainer),
					"backup", client.SupportsAPI(adapters.APISynoBackupTask),
				)
			}
		}(name, client)
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
