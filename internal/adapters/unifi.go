package adapters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// UniFiClient handles authentication and API calls to the UniFi Controller.
type UniFiClient struct {
	host        string
	user        string
	pass        string
	insecureTLS bool
	client      *http.Client
}

// NewUniFiClient creates a new UniFi Controller API client.
// Set insecureTLS to true to skip TLS certificate verification (opt-in).
func NewUniFiClient(host, user, pass string, insecureTLS bool) *UniFiClient {
	jar, _ := cookiejar.New(nil)
	return &UniFiClient{
		host:        host,
		user:        user,
		pass:        pass,
		insecureTLS: insecureTLS,
		client: &http.Client{
			Jar:       jar,
			Transport: tlsTransport(insecureTLS),
		},
	}
}

// unifiResponse is the generic envelope for UniFi API responses.
type unifiResponse[T any] struct {
	Meta struct {
		RC string `json:"rc"`
	} `json:"meta"`
	Data T `json:"data"`
}

// Ping reports whether the UniFi controller is reachable. It satisfies the adapters.HealthChecker interface.
func (c *UniFiClient) Ping() error {
	cl := &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: tlsTransport(c.insecureTLS),
	}
	resp, err := cl.Get(fmt.Sprintf("https://%s/", c.host))
	if err != nil {
		return fmt.Errorf("unifi unreachable: %w", err)
	}
	resp.Body.Close()
	return nil
}

// login authenticates with the UniFi Controller and stores the session cookie.
func (c *UniFiClient) login() error {
	body, _ := json.Marshal(map[string]string{
		"username": c.user,
		"password": c.pass,
	})
	resp, err := c.client.Post(
		fmt.Sprintf("https://%s/api/login", c.host),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("unifi login: %w", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body) //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unifi login: status %d", resp.StatusCode)
	}
	return nil
}

// get performs an authenticated GET request against the UniFi API and decodes the response.
func (c *UniFiClient) get(path string, out any) error {
	resp, err := c.client.Get(fmt.Sprintf("https://%s%s", c.host, path))
	if err != nil {
		return fmt.Errorf("unifi request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("unifi read body: %w", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("unifi parse response: %w", err)
	}
	return nil
}

// --- Device types ---

// UniFiDevice represents a managed network device from the UniFi Controller.
type UniFiDevice struct {
	ID          string `json:"_id"`
	MAC         string `json:"mac"`
	Name        string `json:"name"`
	Model       string `json:"model"`
	Type        string `json:"type"` // uap, usw, ugw, udm, udm-pro
	State       int    `json:"state"`
	IP          string `json:"ip"`
	Version     string `json:"version"`
	Uptime      int    `json:"uptime"`
	UserNumSta  int    `json:"user-num_sta"`  // connected user clients
	GuestNumSta int    `json:"guest-num_sta"` // connected guest clients
}

// GetDevices retrieves all managed network devices from the UniFi Controller.
func (c *UniFiClient) GetDevices() ([]UniFiDevice, error) {
	if err := c.login(); err != nil {
		return nil, err
	}
	var result unifiResponse[[]UniFiDevice]
	if err := c.get("/api/s/default/stat/device", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// --- Client types ---

// UniFiSta represents an active client device from the UniFi Controller.
type UniFiSta struct {
	MAC            string  `json:"mac"`
	Hostname       *string `json:"hostname"`
	Name           *string `json:"name"`
	IP             string  `json:"ip"`
	IsWired        bool    `json:"is_wired"`
	ESSID          *string `json:"essid"`
	Signal         *int    `json:"signal"`
	Uptime         int     `json:"uptime"`
	SwMAC          string  `json:"sw_mac"`           // MAC of the switch this client is connected to (wired)
	SwPort         int     `json:"sw_port"`          // Switch port number (wired)
	LastUplinkName string  `json:"last_uplink_name"` // Switch display name (wired)
}

// GetClients retrieves currently active client devices from the UniFi Controller.
func (c *UniFiClient) GetClients() ([]UniFiSta, error) {
	if err := c.login(); err != nil {
		return nil, err
	}
	var result unifiResponse[[]UniFiSta]
	if err := c.get("/api/s/default/stat/sta", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// --- Client v2 types ---

// UniFiClientV2 represents a client from the UniFi Controller v2 API.
// Active clients have IP, Uptime, ESSID, Signal populated; history clients have LastIP, LastSeen.
type UniFiClientV2 struct {
	ID             string  `json:"id"`
	MAC            string  `json:"mac"`
	DisplayName    string  `json:"display_name"`
	Name           *string `json:"name"`
	Hostname       *string `json:"hostname"`
	IP             string  `json:"ip"`
	LastIP         string  `json:"last_ip"`
	IsWired        bool    `json:"is_wired"`
	Status         string  `json:"status"` // "online" | "offline"
	LastUplinkName string  `json:"last_uplink_name"`
	Uptime         int     `json:"uptime"`
	ESSID          *string `json:"essid"`
	Signal         *int    `json:"signal"`
	LastSeen       int64   `json:"last_seen"`
}

// getV2 performs an authenticated GET request against the UniFi v2 API and decodes the bare JSON array response.
func (c *UniFiClient) getV2(path string, out any) error {
	return c.get(path, out)
}

// fetchActiveClients calls the v2 active clients endpoint. Caller must have already called login().
func (c *UniFiClient) fetchActiveClients() ([]UniFiClientV2, error) {
	var result []UniFiClientV2
	if err := c.getV2("/v2/api/site/default/clients/active?includeTrafficUsage=false&includeUnifiDevices=false", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// fetchOfflineClients calls the v2 history clients endpoint. Caller must have already called login().
func (c *UniFiClient) fetchOfflineClients(historyDays int) ([]UniFiClientV2, error) {
	path := fmt.Sprintf("/v2/api/site/default/clients/history?onlyNonBlocked=true&withinHours=%d", historyDays*24)
	var result []UniFiClientV2
	if err := c.getV2(path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetActiveClients retrieves currently connected clients from the UniFi Controller v2 API.
func (c *UniFiClient) GetActiveClients() ([]UniFiClientV2, error) {
	if err := c.login(); err != nil {
		return nil, err
	}
	return c.fetchActiveClients()
}

// GetOfflineClients retrieves recently disconnected clients from the UniFi Controller v2 API.
// historyDays controls how far back to look (passed as withinHours=historyDays*24).
func (c *UniFiClient) GetOfflineClients(historyDays int) ([]UniFiClientV2, error) {
	if err := c.login(); err != nil {
		return nil, err
	}
	return c.fetchOfflineClients(historyDays)
}

// GetAllClients retrieves all clients (active and history) with a single login.
func (c *UniFiClient) GetAllClients(historyDays int) ([]UniFiClientV2, error) {
	if err := c.login(); err != nil {
		return nil, err
	}
	active, err := c.fetchActiveClients()
	if err != nil {
		return nil, fmt.Errorf("fetch active clients: %w", err)
	}
	offline, err := c.fetchOfflineClients(historyDays)
	if err != nil {
		return nil, fmt.Errorf("fetch offline clients: %w", err)
	}
	return append(active, offline...), nil
}

// --- Health types ---

// UniFiSubsystemHealth represents health data for a single UniFi subsystem.
type UniFiSubsystemHealth struct {
	Subsystem string `json:"subsystem"`
	Status    string `json:"status"`
}

// GetHealth retrieves the health status of all UniFi subsystems.
func (c *UniFiClient) GetHealth() ([]UniFiSubsystemHealth, error) {
	if err := c.login(); err != nil {
		return nil, err
	}

	var result unifiResponse[[]UniFiSubsystemHealth]
	if err := c.get("/api/s/default/stat/health", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
