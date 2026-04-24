package apierrors

import "errors"

// ErrNotFound is returned by service methods when a requested resource cannot be located.
var ErrNotFound = errors.New("not found")

// URN type identifiers for RFC 9457 problem+json responses.
const (
	URNNotFound            = "urn:homelab:error:not-found"
	URNInternalServerError = "urn:homelab:error:internal-server-error"
)
