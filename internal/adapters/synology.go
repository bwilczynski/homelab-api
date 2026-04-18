package adapters

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// SynologyClient handles authentication and API calls to the Synology DSM.
type SynologyClient struct {
	host   string
	user   string
	pass   string
	sid    string
	client *http.Client
}

// NewSynologyClient creates a new Synology DSM API client.
func NewSynologyClient(host, user, pass string) *SynologyClient {
	return &SynologyClient{
		host: host,
		user: user,
		pass: pass,
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// SynologyResponse is the generic envelope for all DSM API responses.
type SynologyResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *SynologyError  `json:"error,omitempty"`
}

// SynologyError contains error details from the DSM API.
type SynologyError struct {
	Code int `json:"code"`
}

// Login authenticates with the DSM and stores the session ID.
func (c *SynologyClient) Login() error {
	params := url.Values{
		"api":     {"SYNO.API.Auth"},
		"method":  {"login"},
		"version": {"6"},
		"account": {c.user},
		"passwd":  {c.pass},
		"format":  {"sid"},
	}
	resp, err := c.rawGet(params)
	if err != nil {
		return fmt.Errorf("synology login: %w", err)
	}

	var loginData struct {
		SID string `json:"sid"`
	}
	if err := json.Unmarshal(resp.Data, &loginData); err != nil {
		return fmt.Errorf("synology login parse: %w", err)
	}
	c.sid = loginData.SID
	return nil
}

// Logout ends the DSM session.
func (c *SynologyClient) Logout() error {
	params := url.Values{
		"api":     {"SYNO.API.Auth"},
		"method":  {"logout"},
		"version": {"6"},
		"_sid":    {c.sid},
	}
	_, err := c.rawGet(params)
	c.sid = ""
	return err
}

// Call makes an authenticated API call and returns the raw data payload.
func (c *SynologyClient) Call(api, method, version string, extra url.Values) (json.RawMessage, error) {
	if c.sid == "" {
		if err := c.Login(); err != nil {
			return nil, err
		}
	}

	params := url.Values{
		"api":     {api},
		"method":  {method},
		"version": {version},
		"_sid":    {c.sid},
	}
	for k, v := range extra {
		params[k] = v
	}

	resp, err := c.rawGet(params)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		code := 0
		if resp.Error != nil {
			code = resp.Error.Code
		}
		return nil, fmt.Errorf("synology API %s error code %d", api, code)
	}
	return resp.Data, nil
}

func (c *SynologyClient) rawGet(params url.Values) (*SynologyResponse, error) {
	u := fmt.Sprintf("https://%s/webapi/entry.cgi?%s", c.host, params.Encode())
	resp, err := c.client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("synology request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("synology read body: %w", err)
	}

	var result SynologyResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("synology parse response: %w", err)
	}
	return &result, nil
}

// --- Docker container types ---

// DSMContainerListResponse is the data payload from SYNO.Docker.Container list.
type DSMContainerListResponse struct {
	Containers []DSMContainer `json:"containers"`
	Total      int            `json:"total"`
}

// DSMContainer represents a container in the list response.
type DSMContainer struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Status string            `json:"status"`
	State  DSMContainerState `json:"State"`
}

// DSMContainerState holds the state flags from the DSM API.
type DSMContainerState struct {
	Dead       bool   `json:"Dead"`
	Paused     bool   `json:"Paused"`
	Restarting bool   `json:"Restarting"`
	Running    bool   `json:"Running"`
	Status     string `json:"Status"`
	StartedAt  string `json:"StartedAt"`
	FinishedAt string `json:"FinishedAt"`
	ExitCode   int    `json:"ExitCode"`
	OOMKilled  bool   `json:"OOMKilled"`
}

// DSMContainerDetailResponse is the data payload from SYNO.Docker.Container get.
type DSMContainerDetailResponse struct {
	Details DSMContainerDetail        `json:"details"`
	Profile DSMContainerDetailProfile `json:"profile"`
}

// DSMContainerDetailProfile holds the container profile from the DSM get response.
type DSMContainerDetailProfile struct {
	Name           string             `json:"name"`
	Image          string             `json:"image"`
	EnvVariables   []DSMEnvVariable   `json:"env_variables"`
	Networks       []DSMNetwork       `json:"network"`
	PortBindings   []DSMProfilePortBinding `json:"port_bindings"`
	VolumeBindings []DSMVolumeBinding `json:"volume_bindings"`
	Privileged     bool               `json:"privileged"`
	MemoryLimit    int                `json:"memory_limit"`
	Cmd            string             `json:"cmd"`
	RestartPolicy  DSMRestartPolicy   `json:"enable_restart_policy"`
}

// DSMEnvVariable represents an environment variable.
type DSMEnvVariable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// DSMNetwork represents a container network.
type DSMNetwork struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
}

// DSMProfilePortBinding represents a port binding.
type DSMProfilePortBinding struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
	Type          string `json:"type"`
}

// DSMVolumeBinding represents a volume bind mount.
// DSM uses "host_absolute_path" for direct host paths and "host_volume_file" for DSM-managed volumes.
type DSMVolumeBinding struct {
	HostAbsolutePath string `json:"host_absolute_path"`
	HostVolumePath   string `json:"host_volume_file"`
	MountPath        string `json:"mount_point"`
	Type             string `json:"type"`
}

// HostPath returns the host-side path, preferring host_absolute_path over host_volume_file.
func (v DSMVolumeBinding) HostPath() string {
	if v.HostAbsolutePath != "" {
		return v.HostAbsolutePath
	}
	return v.HostVolumePath
}

// DSMRestartPolicy represents restart policy.
type DSMRestartPolicy bool

// DSMContainerDetail contains detailed container information.
type DSMContainerDetail struct {
	Name            string             `json:"Name"`
	RestartCount    int                `json:"RestartCount"`
	State           DSMContainerState  `json:"State"`
	Config          DSMContainerConfig `json:"Config"`
	HostConfig      DSMHostConfig      `json:"HostConfig"`
	Created         string             `json:"Created"`
	Mounts          []DSMMount         `json:"Mounts"`
	NetworkSettings DSMNetworkSettings `json:"NetworkSettings"`
}

// DSMContainerConfig holds container configuration.
type DSMContainerConfig struct {
	Image      string                 `json:"Image"`
	Env        []string               `json:"Env"`
	Cmd        []string               `json:"Cmd"`
	Entrypoint []string               `json:"Entrypoint"`
	Labels     map[string]string      `json:"Labels"`
	Volumes    map[string]interface{} `json:"Volumes"`
	WorkingDir string                 `json:"WorkingDir"`
	Hostname   string                 `json:"Hostname"`
}

// DSMHostConfig holds host configuration.
type DSMHostConfig struct {
	Memory        int                         `json:"Memory"`
	Privileged    bool                        `json:"Privileged"`
	PortBindings  map[string][]DSMPortBinding `json:"PortBindings"`
	RestartPolicy DSMHostRestartPolicy        `json:"RestartPolicy"`
	Binds         []string                    `json:"Binds"`
}

// DSMPortBinding represents a port binding from HostConfig.
type DSMPortBinding struct {
	HostIp   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

// DSMHostRestartPolicy represents restart policy from HostConfig.
type DSMHostRestartPolicy struct {
	Name              string `json:"Name"`
	MaximumRetryCount int    `json:"MaximumRetryCount"`
}

// DSMMount represents a mount point.
type DSMMount struct {
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Mode        string `json:"Mode"`
	RW          bool   `json:"RW"`
	Type        string `json:"Type"`
}

// DSMNetworkSettings holds network settings.
type DSMNetworkSettings struct {
	Networks map[string]DSMNetworkInfo   `json:"Networks"`
	Ports    map[string][]DSMPortBinding `json:"Ports"`
}

// DSMNetworkInfo represents network info.
type DSMNetworkInfo struct {
	IPAddress  string   `json:"IPAddress"`
	MacAddress string   `json:"MacAddress"`
	Gateway    string   `json:"Gateway"`
	NetworkID  string   `json:"NetworkID"`
	Aliases    []string `json:"Aliases"`
}

// DSMContainerResourceResponse is the data payload from SYNO.Docker.Container.Resource get.
type DSMContainerResourceResponse struct {
	Resources []DSMContainerResource `json:"resources"`
}

// DSMContainerResource represents resource usage for a single container.
type DSMContainerResource struct {
	Name          string  `json:"name"`
	CPU           float64 `json:"cpu"`
	Memory        int64   `json:"memory"`
	MemoryPercent float64 `json:"memoryPercent"`
}

// ListContainers retrieves all containers from the DSM Docker API.
func (c *SynologyClient) ListContainers() (*DSMContainerListResponse, error) {
	data, err := c.Call("SYNO.Docker.Container", "list", "1", url.Values{
		"limit":  {"0"},
		"offset": {"0"},
	})
	if err != nil {
		return nil, err
	}
	var result DSMContainerListResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse container list: %w", err)
	}
	return &result, nil
}

// GetContainer retrieves a single container's details from the DSM Docker API.
func (c *SynologyClient) GetContainer(name string) (*DSMContainerDetailResponse, error) {
	data, err := c.Call("SYNO.Docker.Container", "get", "1", url.Values{
		"name": {name},
	})
	if err != nil {
		return nil, err
	}
	var result DSMContainerDetailResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse container detail: %w", err)
	}
	return &result, nil
}

// GetContainerResources retrieves resource usage for all containers.
func (c *SynologyClient) GetContainerResources() (*DSMContainerResourceResponse, error) {
	data, err := c.Call("SYNO.Docker.Container.Resource", "get", "1", nil)
	if err != nil {
		return nil, err
	}
	var result DSMContainerResourceResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse container resources: %w", err)
	}
	return &result, nil
}

// StartContainer starts a container by name.
func (c *SynologyClient) StartContainer(name string) error {
	_, err := c.Call("SYNO.Docker.Container", "start", "1", url.Values{
		"name": {name},
	})
	return err
}

// StopContainer stops a container by name.
func (c *SynologyClient) StopContainer(name string) error {
	_, err := c.Call("SYNO.Docker.Container", "stop", "1", url.Values{
		"name": {name},
	})
	return err
}

// RestartContainer restarts a container by name.
func (c *SynologyClient) RestartContainer(name string) error {
	_, err := c.Call("SYNO.Docker.Container", "restart", "1", url.Values{
		"name": {name},
	})
	return err
}
