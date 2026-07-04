package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Regression test for the schemathesis contract-test flake: path params in
// canonical percent-encoded form containing an encoded '%' were decoded twice
// (chi routing on the decoded URL.Path + oapi-codegen runtime PathUnescape)
// and rejected with 400 despite being valid requests.
func TestEncodedPercentPathParamsAreNotRejected(t *testing.T) {
	r := newRouter("../../internal", slog.New(slog.DiscardHandler))

	paths := []string{
		"/network/vlans/%C2%A7%25%F2%BF%A3%88%C2%AC%C3%83%F1%89%A7%8D",
		"/storage/volumes/%25%F3%B8%81%A5v%0D%C3%99%C3%96%E3%B0%98%18%F2%A8%8D%8BiL%C2%BC%C3%BF%C2%9B%F3%93%8D%91b%C3%B5Z%F0%9E%86%81",
	}

	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, body %q; want 404", path, rec.Code, rec.Body.String())
		}
	}
}
