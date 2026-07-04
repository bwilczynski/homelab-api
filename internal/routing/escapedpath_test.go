package routing

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/oapi-codegen/runtime"
)

// bindPathParam mirrors the param binding in generated api.gen.go handlers.
func bindPathParam(r *http.Request, name string) (string, error) {
	var v string
	err := runtime.BindStyledParameterWithOptions("simple", name, chi.URLParam(r, name), &v, runtime.BindStyledParameterOptions{
		ParamLocation: runtime.ParamLocationPath,
		Explode:       false,
		Required:      true,
		Type:          "string",
	})
	return v, err
}

func TestEscapedPathRouting(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"canonically encoded percent", "/things/%25abc", "%abc"},
		{"encoded percent with utf8", "/things/%C2%A7%25%F2%BF%A3%88", "§%\xf2\xbf\xa3\x88"},
		{"plain value", "/things/foo", "foo"},
		{"encoded slash stays in one segment", "/things/a%2Fb", "a/b"},
		{"non-canonical encoding", "/things/%41%42", "AB"},
	}

	r := chi.NewRouter()
	r.Use(EscapedPathRouting)
	r.Get("/things/{id}", func(w http.ResponseWriter, req *http.Request) {
		id, err := bindPathParam(req, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Write([]byte(id))
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %q; want 200", rec.Code, rec.Body.String())
			}
			if got := rec.Body.String(); got != tt.want {
				t.Errorf("param = %q, want %q", got, tt.want)
			}
		})
	}
}
