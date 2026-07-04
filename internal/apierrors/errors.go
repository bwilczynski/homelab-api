package apierrors

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound is returned by service methods when a requested resource cannot be located.
var ErrNotFound = errors.New("not found")

// RFC 9457 problem+json constants.
const (
	URNBadRequest          = "urn:homelab:error:bad-request"
	URNNotFound            = "urn:homelab:error:not-found"
	URNInternalServerError = "urn:homelab:error:internal-server-error"
	URNUnauthorized        = "urn:homelab:error:unauthorized"
	URNForbidden           = "urn:homelab:error:forbidden"

	TitleBadRequest          = "Bad Request"
	TitleNotFound            = "Not Found"
	TitleInternalServerError = "Internal Server Error"
	TitleUnauthorized        = "Unauthorized"
	TitleForbidden           = "Forbidden"
)

// ParseCompositeID splits a composite ID "device.suffix" into its parts.
// what names the ID kind and format describes the expected shape for the
// error message (e.g. "volume ID", "device.name").
func ParseCompositeID(id, what, format string) (device, suffix string, err error) {
	parts := strings.SplitN(id, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid %s %q: expected format %s: %w", what, id, format, ErrNotFound)
	}
	return parts[0], parts[1], nil
}
