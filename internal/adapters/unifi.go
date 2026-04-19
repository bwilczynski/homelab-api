package adapters

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
)

// UniFiClient handles authentication and API calls to the UniFi Controller.
type UniFiClient struct {
	host   string
	user   string
	pass   string
	client *http.Client
}

// NewUniFiClient creates a new UniFi Controller API client.
func NewUniFiClient(host, user, pass string) *UniFiClient {
	jar, _ := cookiejar.New(nil)
	return &UniFiClient{
		host: host,
		user: user,
		pass: pass,
		client: &http.Client{
			Jar: jar,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
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
func (c *UniFiClient) get(path string, out interface{}) error {
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
	ID         string `json:"_id"`
	MAC        string `json:"mac"`
	Name       string `json:"name"`
	Model      string `json:"model"`
	Type       string `json:"type"` // uap, usw, ugw, udm, udm-pro
	State      int    `json:"state"`
	IP         string `json:"ip"`
	Version    string `json:"version"`
	Uptime     int    `json:"uptime"`
	UserNumSta int    `json:"user-num_sta"` // connected user clients
	GuestNumSta int   `json:"guest-num_sta"` // connected guest clients
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
	MAC      string  `json:"mac"`
	Hostname *string `json:"hostname"`
	Name     *string `json:"name"`
	IP       string  `json:"ip"`
	IsWired  bool    `json:"is_wired"`
	ESSID    *string `json:"essid"`
	Signal   *int    `json:"signal"`
	Uptime   int     `json:"uptime"`
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
