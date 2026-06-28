package meta

// Service holds the static configuration values returned by meta endpoints.
type Service struct {
	apiVersion    string
	serverVersion string
	authEnabled   bool
	authIssuer    string
}

// NewService creates a Service with version and auth configuration.
func NewService(apiVersion, serverVersion string, authEnabled bool, authIssuer string) *Service {
	return &Service{
		apiVersion:    apiVersion,
		serverVersion: serverVersion,
		authEnabled:   authEnabled,
		authIssuer:    authIssuer,
	}
}

// GetVersion returns the API contract version and server build version.
func (s *Service) GetVersion() (apiVersion, serverVersion string) {
	return s.apiVersion, s.serverVersion
}

// GetAuth returns the auth configuration: whether auth is enabled and the issuer URL.
func (s *Service) GetAuth() (enabled bool, issuer string) {
	return s.authEnabled, s.authIssuer
}
