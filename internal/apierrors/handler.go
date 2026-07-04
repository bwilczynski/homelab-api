package apierrors

import (
	"encoding/json"
	"net/http"
)

// WriteProblem writes an RFC 9457 problem+json response to the client.
func WriteProblem(w http.ResponseWriter, status int, urn, title, detail string) {
	body := map[string]any{
		"type":   urn,
		"title":  title,
		"status": status,
		"detail": detail,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ProblemBadRequestHandler is passed as ErrorHandlerFunc to HandlerWithOptions for every
// domain. It replaces the default http.Error plain-text 400 with an RFC 9457 problem+json body.
func ProblemBadRequestHandler(w http.ResponseWriter, _ *http.Request, err error) {
	WriteProblem(w, http.StatusBadRequest, URNBadRequest, TitleBadRequest, err.Error())
}
