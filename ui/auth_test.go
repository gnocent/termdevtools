package ui

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"termdevtools/config"
	"termdevtools/i18n"
)

// A made-up API key of the shape Elasticsearch generates, and its "encoded"
// form: what Kibana shows first.
const (
	testAPIKeyID     = "AbCdEfGhIjKl-nOpQrSt"
	testAPIKeySecret = "ZyXwVuTsRqPoNmLkJi_gFe"
)

var testAPIKeyEncoded = base64.StdEncoding.EncodeToString([]byte(testAPIKeyID + ":" + testAPIKeySecret))

// recordAuthorization answers as a cluster's root does, and reports the
// Authorization header of what it is sent.
func recordAuthorization(received chan<- string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(elasticsearchRoot))
	}
}

func savedConfig(t *testing.T) string {
	t.Helper()
	path, err := config.Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(saved)
}

// TestConnectWithAnEncodedAPIKey checks the form of an API key one most
// often has at hand: pasted where the secret goes, it is recognized, sent as
// the same header an identifier and a secret make, and its identifier — the
// one part that isn't secret — is what the cluster's entry keeps, in place
// of the one it had or didn't have.
func TestConnectWithAnEncodedAPIKey(t *testing.T) {
	for name, knownID := range map[string]string{"entry without an identifier": "", "entry with another identifier": "an-older-key"} {
		t.Run(name, func(t *testing.T) {
			received := make(chan string, 1)
			cr, cfg := connectFilling(t, config.Cluster{AuthType: config.AuthAPIKey, APIKeyID: knownID}, recordAuthorization(received),
				func(screen tcell.SimulationScreen) {
					screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to the identifier
					screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to the secret
					injectText(screen, testAPIKeyEncoded)
					screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to Connect
					waitForDraw(t, screen)
				})

			if got, want := <-received, "ApiKey "+testAPIKeyEncoded; got != want {
				t.Errorf("expected the cluster to be sent %q, got %q", want, got)
			}
			if got := cfg.Clusters[0].APIKeyID; got != testAPIKeyID {
				t.Errorf("expected the key's own identifier on the cluster's entry, got %q", got)
			}
			if want := "api_key:" + testAPIKeyID; cr.DisplayUser != want {
				t.Errorf("expected %q in the status bar, got %q", want, cr.DisplayUser)
			}
			saved := savedConfig(t)
			if !strings.Contains(saved, "api_key_id: "+testAPIKeyID) {
				t.Errorf("expected the identifier to be saved, got:\n%s", saved)
			}
			if strings.Contains(saved, testAPIKeySecret) || strings.Contains(saved, testAPIKeyEncoded) {
				t.Errorf("the key was written to config.yaml:\n%s", saved)
			}
		})
	}
}

// TestConnectWithAnAPIKeyIDAndSecret checks that the other way of giving a
// key still works as it did: the identifier of the entry, and the secret.
func TestConnectWithAnAPIKeyIDAndSecret(t *testing.T) {
	received := make(chan string, 1)
	_, cfg := connectFilling(t, config.Cluster{AuthType: config.AuthAPIKey, APIKeyID: testAPIKeyID}, recordAuthorization(received),
		func(screen tcell.SimulationScreen) {
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to the identifier
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to the secret
			injectText(screen, testAPIKeySecret)
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to Connect
			waitForDraw(t, screen)
		})

	if got, want := <-received, "ApiKey "+testAPIKeyEncoded; got != want {
		t.Errorf("expected the cluster to be sent %q, got %q", want, got)
	}
	if got := cfg.Clusters[0].APIKeyID; got != testAPIKeyID {
		t.Errorf("expected the identifier to be kept, got %q", got)
	}
}

// TestAPIKeyWithoutIdentifierIsRefused checks the one case left without an
// identifier: a secret alone. Nothing is sent, and the message says what is
// missing instead of leaving it to a 401.
func TestAPIKeyWithoutIdentifierIsRefused(t *testing.T) {
	var reached atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Store(true) }))
	defer srv.Close()

	screen, results := openClusterForm(t, config.Cluster{URL: srv.URL, AuthType: config.AuthAPIKey})
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to the identifier, left empty
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to the secret
	injectText(screen, testAPIKeySecret)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to Connect
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)

	waitForScreen(t, screen, i18n.For(i18n.FR).ErrAPIKeyIDRequired)
	select {
	case <-results:
		t.Error("expected no connection")
	case <-time.After(300 * time.Millisecond):
	}
	if reached.Load() {
		t.Error("expected nothing to be sent to the cluster")
	}
}

// TestConnectWithABearerToken checks the fifth way to authenticate: the
// token is sent as it is, and nothing of it is kept — it is asked again on
// the next connection, like any other secret.
func TestConnectWithABearerToken(t *testing.T) {
	const token = "bm90LWEtcmVhbC10b2tlbg.only-the-shape-of-one"
	received := make(chan string, 1)
	cr, cfg := connectFilling(t, config.Cluster{AuthType: config.AuthBearer}, recordAuthorization(received),
		func(screen tcell.SimulationScreen) {
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to the token
			injectText(screen, token)
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to Connect
			waitForDraw(t, screen)
		})

	if got, want := <-received, "Bearer "+token; got != want {
		t.Errorf("expected the cluster to be sent %q, got %q", want, got)
	}
	if got := cfg.Clusters[0].AuthType; got != config.AuthBearer {
		t.Errorf("expected the authentication type to be kept, got %q", got)
	}
	if cr.DisplayUser != "Bearer" {
		t.Errorf("expected the status bar to say how the session is authenticated, got %q", cr.DisplayUser)
	}
	saved := savedConfig(t)
	if !strings.Contains(saved, "auth_type: bearer") {
		t.Errorf("expected the authentication type to be saved, got:\n%s", saved)
	}
	if strings.Contains(saved, token) {
		t.Errorf("the token was written to config.yaml:\n%s", saved)
	}
}
