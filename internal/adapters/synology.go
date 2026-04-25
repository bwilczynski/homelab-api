package adapters

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// DSM API names used for capability checks via SupportsAPI.
const (
	APISynoDockerContainer = "SYNO.Docker.Container"
	APISynoBackupTask      = "SYNO.Backup.Task"
)

// DSM error codes.
const dsmErrContainerNotFound = 117

// dsmAPIInfo holds the discovered path and max version for a DSM API.
type dsmAPIInfo struct {
	path    string
	maxVer  int
}

// SynologyClient handles authentication and API calls to the Synology DSM.
type SynologyClient struct {
	host          string
	user          string
	pass          string
	authVersion   string // SYNO.API.Auth version to use for login (default "6")
	insecureTLS   bool
	authInfo      *dsmAPIInfo
	sid           string
	client        *http.Client
	supportedAPIs map[string]bool // nil means DiscoverAPIs not yet called (fail-open)
}

// NewSynologyClient creates a new Synology DSM API client.
// authVersion is the SYNO.API.Auth version to use for login (default "6"; use "3" for older DSM).
// Set insecureTLS to true to skip TLS certificate verification (opt-in).
func NewSynologyClient(host, user, pass, authVersion string, insecureTLS bool) *SynologyClient {
	if authVersion == "" {
		authVersion = "6"
	}
	return &SynologyClient{
		host:        host,
		user:        user,
		pass:        pass,
		authVersion: authVersion,
		insecureTLS: insecureTLS,
		client: &http.Client{
			Transport: tlsTransport(insecureTLS),
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

// DSMAPIError is returned by Call when the DSM API responds with success=false.
type DSMAPIError struct {
	API  string
	Code int
}

func (e *DSMAPIError) Error() string {
	return fmt.Sprintf("synology API %s error code %d", e.API, e.Code)
}

// discoverAuth queries the DSM's API info endpoint to find the correct path and
// maximum supported version for SYNO.API.Auth. The result is cached on the client.
func (c *SynologyClient) discoverAuth() (*dsmAPIInfo, error) {
	if c.authInfo != nil {
		return c.authInfo, nil
	}
	params := url.Values{
		"api":     {"SYNO.API.Info"},
		"version": {"1"},
		"method":  {"query"},
		"query":   {"SYNO.API.Auth"},
	}
	resp, err := c.rawGet("query.cgi", params)
	if err != nil {
		return nil, fmt.Errorf("discover auth API: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("discover auth API: request failed")
	}
	var apis map[string]struct {
		Path       string `json:"path"`
		MaxVersion int    `json:"maxVersion"`
	}
	if err := json.Unmarshal(resp.Data, &apis); err != nil {
		return nil, fmt.Errorf("discover auth API: parse response: %w", err)
	}
	info, ok := apis["SYNO.API.Auth"]
	if !ok {
		return nil, fmt.Errorf("discover auth API: SYNO.API.Auth not found in response")
	}
	c.authInfo = &dsmAPIInfo{path: info.Path, maxVer: info.MaxVersion}
	return c.authInfo, nil
}

// Ping reports whether the DSM is reachable by making a short-timeout unauthenticated
// request to the API info endpoint. It satisfies the adapters.HealthChecker interface.
func (c *SynologyClient) Ping() error {
	cl := &http.Client{
		Timeout:   3 * time.Second,
		Transport: tlsTransport(c.insecureTLS),
	}
	resp, err := cl.Get(fmt.Sprintf("https://%s/webapi/query.cgi", c.host))
	if err != nil {
		return fmt.Errorf("synology unreachable: %w", err)
	}
	resp.Body.Close()
	return nil
}

// DiscoverAPIs queries the DSM for its full API catalogue and caches which APIs are
// available. Call this once at startup; subsequent SupportsAPI calls are local.
func (c *SynologyClient) DiscoverAPIs() error {
	params := url.Values{
		"api":     {"SYNO.API.Info"},
		"version": {"1"},
		"method":  {"query"},
		"query":   {"all"},
	}
	resp, err := c.rawGet("query.cgi", params)
	if err != nil {
		return fmt.Errorf("discover APIs: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("discover APIs: request failed")
	}
	var apis map[string]json.RawMessage
	if err := json.Unmarshal(resp.Data, &apis); err != nil {
		return fmt.Errorf("discover APIs: parse response: %w", err)
	}
	c.supportedAPIs = make(map[string]bool, len(apis))
	for name := range apis {
		c.supportedAPIs[name] = true
	}
	return nil
}

// SupportsAPI reports whether the named DSM API is available on this host.
// Returns true if DiscoverAPIs has not been called yet (fail-open).
func (c *SynologyClient) SupportsAPI(api string) bool {
	if c.supportedAPIs == nil {
		return true
	}
	return c.supportedAPIs[api]
}

// Login authenticates with the DSM and stores the session ID.
// It first discovers the correct auth endpoint and version from the DSM itself.
func (c *SynologyClient) Login() error {
	info, err := c.discoverAuth()
	if err != nil {
		return fmt.Errorf("synology login: %w", err)
	}

	params := url.Values{
		"api":     {"SYNO.API.Auth"},
		"method":  {"login"},
		"version": {c.authVersion},
		"account": {c.user},
		"passwd":  {c.pass},
		"format":  {"sid"},
	}
	resp, err := c.rawGet(info.path, params)
	if err != nil {
		return fmt.Errorf("synology login: %w", err)
	}
	if !resp.Success {
		code := 0
		if resp.Error != nil {
			code = resp.Error.Code
		}
		return fmt.Errorf("synology login failed: error code %d", code)
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
	_, err := c.rawGet("entry.cgi", params)
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

	resp, err := c.rawGet("entry.cgi", params)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		code := 0
		if resp.Error != nil {
			code = resp.Error.Code
		}
		return nil, &DSMAPIError{API: api, Code: code}
	}
	return resp.Data, nil
}

func (c *SynologyClient) rawGet(endpoint string, params url.Values) (*SynologyResponse, error) {
	u := fmt.Sprintf("https://%s/webapi/%s?%s", c.host, endpoint, params.Encode())
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
		return nil, mapContainerNotFound(name, err)
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

// --- System types ---

// DSMSystemInfoResponse is the data payload from SYNO.Core.System info.
type DSMSystemInfoResponse struct {
	FirmwareVer string `json:"firmware_ver"`
	Model       string `json:"model"`
	RamSize     int    `json:"ram_size"`
	UpTime      string `json:"up_time"`
}

// DSMSystemUtilizationResponse is the data payload from SYNO.Core.System.Utilization get.
type DSMSystemUtilizationResponse struct {
	CPU     DSMCPUUsage      `json:"cpu"`
	Memory  DSMMemoryUsage   `json:"memory"`
	Network []DSMNetworkStat `json:"network"`
	Disk    DSMDiskStats     `json:"disk"`
}

// DSMCPUUsage holds CPU utilization from DSM.
type DSMCPUUsage struct {
	UserLoad   int `json:"user_load"`
	SystemLoad int `json:"system_load"`
	OtherLoad  int `json:"other_load"`
}

// DSMMemoryUsage holds memory utilization from DSM (all values in KB).
type DSMMemoryUsage struct {
	MemorySize int `json:"memory_size"`
	TotalReal  int `json:"total_real"`
	AvailReal  int `json:"avail_real"`
	RealUsage  int `json:"real_usage"`
	TotalSwap  int `json:"total_swap"`
	AvailSwap  int `json:"avail_swap"`
}

// DSMNetworkStat holds network throughput for a single interface (bytes/sec).
type DSMNetworkStat struct {
	Device string `json:"device"`
	Rx     int64  `json:"rx"`
	Tx     int64  `json:"tx"`
}

// DSMDiskStats holds disk I/O stats.
type DSMDiskStats struct {
	Disk []DSMDiskStat `json:"disk"`
}

// DSMDiskStat holds I/O counters for a single disk (ops/sec).
type DSMDiskStat struct {
	Device      string `json:"device"`
	ReadAccess  int    `json:"read_access"`
	WriteAccess int    `json:"write_access"`
}

// GetSystemInfo retrieves static system information from the DSM.
func (c *SynologyClient) GetSystemInfo() (*DSMSystemInfoResponse, error) {
	data, err := c.Call("SYNO.Core.System", "info", "1", nil)
	if err != nil {
		return nil, err
	}
	var result DSMSystemInfoResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse system info: %w", err)
	}
	return &result, nil
}

// GetSystemUtilization retrieves live utilization stats from the DSM.
func (c *SynologyClient) GetSystemUtilization() (*DSMSystemUtilizationResponse, error) {
	data, err := c.Call("SYNO.Core.System.Utilization", "get", "1", nil)
	if err != nil {
		return nil, err
	}
	var result DSMSystemUtilizationResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse system utilization: %w", err)
	}
	return &result, nil
}

// --- Storage types ---

// DSMStorageVolumeResponse is the data payload from SYNO.Storage.CGI.Storage load_info.
type DSMStorageVolumeResponse struct {
	Volumes      []DSMStorageVolume      `json:"volumes"`
	Disks        []DSMStorageDisk        `json:"disks"`
	StoragePools []DSMStoragePool        `json:"storagePools"`
}

// DSMStorageVolume represents a single storage volume reported by DSM.
type DSMStorageVolume struct {
	ID       string               `json:"id"`
	VolPath  string               `json:"vol_path"`
	Status   string               `json:"status"`
	FsType   string               `json:"fs_type"`
	RaidType string               `json:"raidType"`
	PoolPath string               `json:"pool_path"`
	Size     DSMStorageVolumeSize `json:"size"`
}

// DSMStorageVolumeSize holds the capacity info for a volume (values are strings in bytes).
type DSMStorageVolumeSize struct {
	Total string `json:"total"`
	Used  string `json:"used"`
}

// DSMStorageDisk represents a physical disk reported by DSM.
type DSMStorageDisk struct {
	ID        string `json:"id"`
	Model     string `json:"model"`
	SizeTotal string `json:"size_total"`
	Status    string `json:"status"`
	Temp      int    `json:"temp"`
}

// DSMStoragePool represents a storage pool (RAID group) reported by DSM.
type DSMStoragePool struct {
	ID       string   `json:"id"`
	Disks    []string `json:"disks"`
	RaidType string   `json:"raidType"`
	Status   string   `json:"status"`
}

// GetStorageVolumes retrieves the list of storage volumes from the DSM.
func (c *SynologyClient) GetStorageVolumes() (*DSMStorageVolumeResponse, error) {
	data, err := c.Call("SYNO.Storage.CGI.Storage", "load_info", "1", nil)
	if err != nil {
		return nil, err
	}
	var result DSMStorageVolumeResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse storage volumes: %w", err)
	}
	return &result, nil
}

// StartContainer starts a container by name.
func (c *SynologyClient) StartContainer(name string) error {
	_, err := c.Call("SYNO.Docker.Container", "start", "1", url.Values{
		"name": {name},
	})
	return mapContainerNotFound(name, err)
}

// StopContainer stops a container by name.
func (c *SynologyClient) StopContainer(name string) error {
	_, err := c.Call("SYNO.Docker.Container", "stop", "1", url.Values{
		"name": {name},
	})
	return mapContainerNotFound(name, err)
}

// RestartContainer restarts a container by name.
func (c *SynologyClient) RestartContainer(name string) error {
	_, err := c.Call("SYNO.Docker.Container", "restart", "1", url.Values{
		"name": {name},
	})
	return mapContainerNotFound(name, err)
}

// mapContainerNotFound converts DSM error code 117 to apierrors.ErrNotFound.
func mapContainerNotFound(name string, err error) error {
	if err == nil {
		return nil
	}
	if apiErr, ok := err.(*DSMAPIError); ok && apiErr.Code == dsmErrContainerNotFound {
		return fmt.Errorf("container %q: %w", name, apierrors.ErrNotFound)
	}
	return err
}

// --- Backup types ---

// DSMBackupTaskListResponse is the data payload from SYNO.Backup.Task list.
type DSMBackupTaskListResponse struct {
	TaskList          []DSMBackupTask `json:"task_list"`
	Total             int             `json:"total"`
	IsDataRestoring   bool            `json:"is_data_restoring"`
	IsDownloading     bool            `json:"is_downloading"`
	IsLunRestoring    bool            `json:"is_lun_restoring"`
	IsRestoring       bool            `json:"is_restoring"`
	IsSnapshotRestore bool            `json:"is_snapshot_restoring"`
}

// DSMBackupTask represents a single backup task from the Hyper Backup service.
type DSMBackupTask struct {
	TaskID       int    `json:"task_id"`
	Name         string `json:"name"`
	State        string `json:"state"`
	Status       string `json:"status"`
	Type         string `json:"type"`
	TransferType string `json:"transfer_type"`
	TargetID     string `json:"target_id"`
	TargetType   string `json:"target_type"`
	DataEnc      bool   `json:"data_enc"`
}

// DSMTaskSchedulerListResponse is the data payload from SYNO.Core.TaskScheduler list.
type DSMTaskSchedulerListResponse struct {
	Tasks []DSMScheduledTask `json:"tasks"`
	Total int                `json:"total"`
}

// DSMScheduledTask represents a single task in the DSM task scheduler.
type DSMScheduledTask struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Action         string `json:"action"`
	AppName        string `json:"app_name"`
	Enable         bool   `json:"enable"`
	NextTriggerTime string `json:"next_trigger_time"`
	Owner          string `json:"owner"`
	Type           string `json:"type"`
}

// DSMBackupLogListResponse is the data payload from SYNO.SDS.Backup.Client.Common.Log list.
type DSMBackupLogListResponse struct {
	LogList    []DSMBackupLogEntry `json:"log_list"`
	Total      int                 `json:"total"`
	ErrorCount int                 `json:"error_count"`
	InfoCount  int                 `json:"info_count"`
}

// DSMBackupLogEntry represents a single log entry for a backup task.
type DSMBackupLogEntry struct {
	Event string `json:"event"`
	Level string `json:"level"`
	Time  string `json:"time"`
	User  string `json:"user"`
}

// ListBackupTasks retrieves all backup tasks from the Hyper Backup service.
func (c *SynologyClient) ListBackupTasks() (*DSMBackupTaskListResponse, error) {
	data, err := c.Call("SYNO.Backup.Task", "list", "1", nil)
	if err != nil {
		return nil, err
	}
	var result DSMBackupTaskListResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse backup task list: %w", err)
	}
	return &result, nil
}

// ListScheduledTasks retrieves scheduled tasks from the DSM task scheduler.
func (c *SynologyClient) ListScheduledTasks() (*DSMTaskSchedulerListResponse, error) {
	data, err := c.Call("SYNO.Core.TaskScheduler", "list", "2", url.Values{
		"offset": {"0"},
		"limit":  {"50"},
	})
	if err != nil {
		return nil, err
	}
	var result DSMTaskSchedulerListResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse task scheduler list: %w", err)
	}
	return &result, nil
}

// ListBackupLogs retrieves recent log entries for a specific backup task.
func (c *SynologyClient) ListBackupLogs(taskID int) (*DSMBackupLogListResponse, error) {
	data, err := c.Call("SYNO.SDS.Backup.Client.Common.Log", "list", "1", url.Values{
		"task_id": {fmt.Sprintf("%d", taskID)},
		"offset":  {"0"},
		"limit":   {"100"},
	})
	if err != nil {
		return nil, err
	}
	var result DSMBackupLogListResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse backup logs: %w", err)
	}
	return &result, nil
}
