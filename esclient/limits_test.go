package esclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestExecuteRefusesOversizedResponse checks that a response beyond the
// size bound is reported as such — with what to do about it — instead of
// being loaded whole, and that one right at the bound still goes through.
func TestExecuteRefusesOversizedResponse(t *testing.T) {
	previous := maxResponseBytes
	maxResponseBytes = 1 << 20
	t.Cleanup(func() { maxResponseBytes = previous })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		size := int(maxResponseBytes)
		if r.URL.Path == "/too-big" {
			size++
		}
		_, _ = w.Write([]byte(strings.Repeat("x", size)))
	}))
	defer srv.Close()
	client, err := New(Params{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := client.Execute(context.Background(), "GET", "too-big", nil); err == nil || !strings.Contains(err.Error(), "larger than 1 MB") {
		t.Errorf("expected the oversized response to be refused, got %v", err)
	}
	result, err := client.Execute(context.Background(), "GET", "just-fits", nil)
	if err != nil {
		t.Fatalf("expected a response of exactly the bound to be accepted, got %v", err)
	}
	if int64(len(result.Body)) != maxResponseBytes {
		t.Errorf("expected the whole body, got %d bytes", len(result.Body))
	}
}

// TestRedirectionsAreNotFollowed checks that a redirection is reported as
// such, not acted on: the request must never reach the address it points to
// — neither replayed there (307) nor turned into a GET (302).
func TestRedirectionsAreNotFollowed(t *testing.T) {
	var elsewhere []string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere = append(elsewhere, r.Method+" "+r.URL.Path)
	}))
	defer target.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusFound
		if r.URL.Path == "/replayed" {
			status = http.StatusTemporaryRedirect
		}
		http.Redirect(w, r, target.URL+"/landed", status)
	}))
	defer srv.Close()
	client, err := New(Params{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for path, wantStatus := range map[string]int{"converted": http.StatusFound, "replayed": http.StatusTemporaryRedirect} {
		result, err := client.Execute(context.Background(), "DELETE", path, []byte(`{"dangerous":true}`))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if result.StatusCode != wantStatus || result.Headers.Get("Location") != target.URL+"/landed" {
			t.Errorf("%s: expected the redirection itself (HTTP %d, with its Location), got HTTP %d, Location %q",
				path, wantStatus, result.StatusCode, result.Headers.Get("Location"))
		}
	}
	if len(elsewhere) != 0 {
		t.Errorf("expected nothing to be sent where the redirections point, got %v", elsewhere)
	}
}

// TestDurationCoversTheWholeResponse checks that the reported duration is
// the time the user waited: a body that trickles in after the headers counts.
func TestDurationCoversTheWholeResponse(t *testing.T) {
	const bodyDelay = 150 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush() // headers sent, body still to come
		time.Sleep(bodyDelay)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	client, err := New(Params{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.Execute(context.Background(), "GET", "slow-body", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Duration < bodyDelay {
		t.Errorf("expected the duration to include the body (at least %v), got %v", bodyDelay, result.Duration)
	}
}
