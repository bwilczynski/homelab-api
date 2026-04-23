package adapters

import (
	"crypto/tls"
	"net/http"
)

// tlsTransport returns an http.Transport with TLS verification disabled when
// insecure is true, or nil (use the default transport) when false.
// Returns http.RoundTripper to avoid the nil-interface-vs-nil-pointer trap.
func tlsTransport(insecure bool) http.RoundTripper {
	if !insecure {
		return nil
	}
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
}
