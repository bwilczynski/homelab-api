package apierrors

import (
	"encoding/json"
	"net/http"
)

// ProblemBadRequestHandler is passed as ErrorHandlerFunc to HandlerWithOptions for every
// domain. It replaces the default http.Error plain-text 400 with an RFC 9457 problem+json body.
func ProblemBadRequestHandler(w http.ResponseWriter, _ *http.Request, err error) {
	detail := err.Error()
	body := struct {
		Type   string  `json:"type"`
		Title  string  `json:"title"`
		Status int     `json:"status"`
		Detail *string `json:"detail,omitempty"`
	}{
		Type:   URNBadRequest,
		Title:  TitleBadRequest,
		Status: http.StatusBadRequest,
		Detail: &detail,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(body)
}
