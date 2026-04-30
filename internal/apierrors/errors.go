package apierrors

import "errors"

// ErrNotFound is returned by service methods when a requested resource cannot be located.
var ErrNotFound = errors.New("not found")

// RFC 9457 problem+json constants.
const (
	URNBadRequest          = "urn:homelab:error:bad-request"
	URNNotFound            = "urn:homelab:error:not-found"
	URNInternalServerError = "urn:homelab:error:internal-server-error"

	TitleBadRequest          = "Bad Request"
	TitleNotFound            = "Not Found"
	TitleInternalServerError = "Internal Server Error"
)
