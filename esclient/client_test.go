package esclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestExecuteReturnsResponseHeaders checks that Result.Headers carries the
// response's actual HTTP headers — used by the result panel to display them
// alongside the body (SPEC.md §7 backlog #2), notably a "Warning" header
// Elasticsearch sets to flag a deprecated API in use.
func TestExecuteReturnsResponseHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Warning", `299 Elasticsearch-9.0.0 "deprecated"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client, err := New(Params{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.Execute(context.Background(), "GET", "_cat/health", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got := result.Headers.Get("X-Elastic-Product"); got != "Elasticsearch" {
		t.Errorf("expected X-Elastic-Product header %q, got %q", "Elasticsearch", got)
	}
	if got := result.Headers.Get("Warning"); got == "" {
		t.Error("expected a Warning header to be present")
	}
}
