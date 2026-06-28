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
	apiKey      string
	insecureTLS bool
	client      *http.Client
}

// NewUniFiClient creates a new UniFi Controller API client using username/password session auth.
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

// NewUniFiClientWithAPIKey creates a new UniFi Controller API client using API key auth.
// The key is sent as the X-API-KEY header; no session login is performed.
func NewUniFiClientWithAPIKey(host, apiKey string, insecureTLS bool) *UniFiClient {
	return &UniFiClient{
		host:        host,
		apiKey:      apiKey,
		insecureTLS: insecureTLS,
		client: &http.Client{
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

// maybeLogin authenticates with the controller when using session-based auth.
// It is a no-op when the client is configured with an API key.
func (c *UniFiClient) maybeLogin() error {
	if c.apiKey != "" {
		return nil
	}
	return c.login()
}

// get performs an authenticated GET request against the UniFi API and decodes the response.
func (c *UniFiClient) get(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s%s", c.host, path), nil)
	if err != nil {
		return fmt.Errorf("unifi request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("X-API-KEY", c.apiKey)
	}

	resp, err := c.client.Do(req)
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
	ID          string           `json:"_id"`
	MAC         string           `json:"mac"`
	Name        string           `json:"name"`
	Model       string           `json:"model"`
	Type        string           `json:"type"` // uap, usw, ugw, udm, udm-pro
	State       int              `json:"state"`
	IP          string           `json:"ip"`
	Version     string           `json:"version"`
	Uptime      int              `json:"uptime"`
	UserNumSta  int              `json:"user-num_sta"`  // connected user clients
	GuestNumSta int              `json:"guest-num_sta"` // connected guest clients
	TxBytes     int64            `json:"tx_bytes"`
	RxBytes     int64            `json:"rx_bytes"`
	PortTable   []UniFiPortEntry `json:"port_table"`
	VapTable    []UniFiVap       `json:"vap_table"`
	Uplink      *UniFiUplink     `json:"uplink"`
	Wan1        *UniFiWanIface   `json:"wan1"`
	Wan2        *UniFiWanIface   `json:"wan2"`
}

// UniFiVap is one entry in a device's vap_table — one row per radio per SSID.
type UniFiVap struct {
	ID    string `json:"id"`    // wlanconf _id
	Up    bool   `json:"up"`
}

type UniFiPortEntry struct {
	PortIdx  int     `json:"port_idx"`
	Up       bool    `json:"up"`
	Speed    int     `json:"speed"`
	PortPoe  bool    `json:"port_poe"`
	PoeMode  string  `json:"poe_mode"`
	PoePower string  `json:"poe_power"`
	TxBytes  int64   `json:"tx_bytes"`
	RxBytes  int64   `json:"rx_bytes"`
	TxBytesR float64 `json:"tx_bytes-r"`
	RxBytesR float64 `json:"rx_bytes-r"`
}

type UniFiUplink struct {
	UplinkMAC        string  `json:"uplink_mac"`
	UplinkDeviceName string  `json:"uplink_device_name"`
	UplinkRemotePort *int    `json:"uplink_remote_port"`
	Speed            int     `json:"speed"`
	TxBytesR         float64 `json:"tx_bytes-r"`
	RxBytesR         float64 `json:"rx_bytes-r"`
	TxBytes          int64   `json:"tx_bytes"`
	RxBytes          int64   `json:"rx_bytes"`
}

type UniFiWanIface struct {
	Name     string   `json:"name"`
	IP       string   `json:"ip"`
	Up       bool     `json:"up"`
	DNS      []string `json:"dns"`
	TxBytes  int64    `json:"tx_bytes"`
	RxBytes  int64    `json:"rx_bytes"`
	TxBytesR float64  `json:"tx_bytes-r"`
	RxBytesR float64  `json:"rx_bytes-r"`
}

// UniFiWlanConf represents a WiFi network configuration from the UniFi Controller.
type UniFiWlanConf struct {
	ID             string   `json:"_id"`
	Name           string   `json:"name"`
	NetworkConfID  string   `json:"networkconf_id"`
	Security       string   `json:"security"`
	WpaMode        string   `json:"wpa_mode"`
	Wpa3Support    bool     `json:"wpa3_support"`
	Wpa3Transition bool     `json:"wpa3_transition"`
	WlanBands      []string `json:"wlan_bands"`
	Enabled        bool     `json:"enabled"`
}

// UniFiNetworkConf represents a network/VLAN configuration from the UniFi Controller.
// The Vlan field is interface{} because the JSON value is an integer for tagged VLANs,
// an empty string for the default untagged network, and null for WAN entries.
type UniFiNetworkConf struct {
	ID               string `json:"_id"`
	Name             string `json:"name"`
	Purpose          string `json:"purpose"` // "corporate", "guest", "wan"
	NetworkGroup     string `json:"networkgroup"`
	Vlan             any    `json:"vlan"`
	VlanEnabled      bool   `json:"vlan_enabled"`
	IPSubnet         string `json:"ip_subnet"` // gateway IP + prefix, e.g. "192.168.1.1/24"
	DhcpdEnabled     bool   `json:"dhcpd_enabled"`
	DHCPRelayEnabled bool   `json:"dhcp_relay_enabled"`
	DhcpdStart       string `json:"dhcpd_start"`
	DhcpdStop        string `json:"dhcpd_stop"`
	DhcpdDNS1        string `json:"dhcpd_dns_1"`
	DhcpdDNS2        string `json:"dhcpd_dns_2"`
	WanNetworkGroup  string `json:"wan_networkgroup"` // "WAN" → wan1, "WAN2" → wan2
	WanDNS1          string `json:"wan_dns1"`
	WanDNS2          string `json:"wan_dns2"`
}

// GetDevices retrieves all managed network devices from the UniFi Controller.
func (c *UniFiClient) GetDevices() ([]UniFiDevice, error) {
	if err := c.maybeLogin(); err != nil {
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
	ApMAC          string  `json:"ap_mac"`
	WiredRateMbps  int     `json:"wired_rate_mbps"`
}

// GetClients retrieves currently active client devices from the UniFi Controller.
func (c *UniFiClient) GetClients() ([]UniFiSta, error) {
	if err := c.maybeLogin(); err != nil {
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
	LastUplinkMAC  string  `json:"last_uplink_mac"`
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
	if err := c.maybeLogin(); err != nil {
		return nil, err
	}
	return c.fetchActiveClients()
}

// GetOfflineClients retrieves recently disconnected clients from the UniFi Controller v2 API.
// historyDays controls how far back to look (passed as withinHours=historyDays*24).
func (c *UniFiClient) GetOfflineClients(historyDays int) ([]UniFiClientV2, error) {
	if err := c.maybeLogin(); err != nil {
		return nil, err
	}
	return c.fetchOfflineClients(historyDays)
}

// GetAllClients retrieves all clients (active and history) with a single login.
func (c *UniFiClient) GetAllClients(historyDays int) ([]UniFiClientV2, error) {
	if err := c.maybeLogin(); err != nil {
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
	if err := c.maybeLogin(); err != nil {
		return nil, err
	}

	var result unifiResponse[[]UniFiSubsystemHealth]
	if err := c.get("/api/s/default/stat/health", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// GetWlanConf retrieves all WiFi network configurations from the UniFi Controller.
func (c *UniFiClient) GetWlanConf() ([]UniFiWlanConf, error) {
	if err := c.maybeLogin(); err != nil {
		return nil, err
	}
	var result unifiResponse[[]UniFiWlanConf]
	if err := c.get("/api/s/default/rest/wlanconf", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// GetNetworkConf retrieves all network configurations (VLANs + WAN) from the UniFi Controller.
func (c *UniFiClient) GetNetworkConf() ([]UniFiNetworkConf, error) {
	if err := c.maybeLogin(); err != nil {
		return nil, err
	}
	var result unifiResponse[[]UniFiNetworkConf]
	if err := c.get("/api/s/default/rest/networkconf", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
