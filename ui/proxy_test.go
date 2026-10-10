package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"termdevtools/config"
	"termdevtools/esclient"
	"termdevtools/i18n"
)

// testForwardProxy is a plain HTTP proxy relaying each request to where it
// is addressed. A cluster's "proxy:" setting sends even a loopback address
// through it — unlike a proxy the environment designates — which is what
// lets these tests reach their local servers through one.
type testForwardProxy struct {
	URL  string
	hits atomic.Int32
	// hold, when not nil, keeps every request waiting until it is closed.
	hold chan struct{}
}

func startForwardProxy(t *testing.T, hold chan struct{}) *testForwardProxy {
	t.Helper()
	p := &testForwardProxy{hold: hold}
	relay := &httputil.ReverseProxy{Director: func(*http.Request) {}, Transport: &http.Transport{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.hits.Add(1)
		if p.hold != nil {
			<-p.hold
		}
		relay.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	p.URL = srv.URL
	return p
}

// flatScreen returns what the screen shows as one line of single-spaced
// words: a message long enough to wrap is found whatever the width.
func flatScreen(screen tcell.SimulationScreen) string {
	return strings.Join(strings.Fields(screenText(screen)), " ")
}

func waitForScreen(t *testing.T, screen tcell.SimulationScreen, want string) {
	t.Helper()
	want = strings.Join(strings.Fields(want), " ")
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(flatScreen(screen), want) {
		if time.Now().After(deadline) {
			t.Fatalf("expected %q to be displayed, got:\n%s", want, screenText(screen))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

const elasticsearchRoot = `{"version": {"number": "9.5.4", "build_flavor": "default"}, "tagline": "You Know, for Search"}`

// TestConnectThroughTheClusterProxy checks a cluster's "proxy:" from
// config.yaml end to end: the connection goes through it, says so once
// established, and the setting — not a form field — is still on the
// cluster's entry after the rewrite every successful connection performs.
func TestConnectThroughTheClusterProxy(t *testing.T) {
	proxy := startForwardProxy(t, nil)
	cr, cfg := connectToTestServer(t, config.Cluster{Proxy: proxy.URL}, rootHandler(elasticsearchRoot))

	if proxy.hits.Load() == 0 {
		t.Error("expected the connection to go through the proxy")
	}
	if got, want := cr.Client.Proxy(), (esclient.ProxyInfo{URL: proxy.URL}); got != want {
		t.Errorf("expected the client to report %+v, got %+v", want, got)
	}
	if !strings.Contains(cr.Warning, proxy.URL) || !strings.Contains(cr.Warning, "config.yaml") {
		t.Errorf("expected the proxy and where it comes from to be shown once connected, got %q", cr.Warning)
	}
	if got := cfg.Clusters[0].Proxy; got != proxy.URL {
		t.Errorf("proxy setting lost from the saved cluster entry: %q", got)
	}
}

// TestConnectDirectlyWithProxyNone checks the other value of the setting:
// "none" is a direct connection — nothing to announce — and is kept too.
func TestConnectDirectlyWithProxyNone(t *testing.T) {
	cr, cfg := connectToTestServer(t, config.Cluster{Proxy: "none"}, rootHandler(elasticsearchRoot))

	if got := cr.Client.Proxy(); got != (esclient.ProxyInfo{}) {
		t.Errorf("expected a direct connection, got %+v", got)
	}
	if cr.Warning != "" {
		t.Errorf("unexpected notice: %q", cr.Warning)
	}
	if got := cfg.Clusters[0].Proxy; got != "none" {
		t.Errorf("proxy setting lost from the saved cluster entry: %q", got)
	}
}

// TestProxyIsNamedWhileConnecting checks that the proxy is shown before the
// cluster has answered anything: it is what the credentials are about to go
// through.
func TestProxyIsNamedWhileConnecting(t *testing.T) {
	srv := httptest.NewServer(rootHandler(elasticsearchRoot))
	defer srv.Close()
	hold := make(chan struct{})
	proxy := startForwardProxy(t, hold)

	screen, results := connectOnceTo(t, config.Cluster{URL: srv.URL, AuthType: config.AuthNone, Proxy: proxy.URL})
	waitForScreen(t, screen, fmt.Sprintf(i18n.For(i18n.FR).StatusConnectingViaProxyFmt, proxy.URL, "config.yaml"))

	close(hold)
	select {
	case <-results:
	case <-time.After(5 * time.Second):
		t.Fatalf("no connection once the proxy let the request through; screen:\n%s", screenText(screen))
	}
}

// TestConnectionFailureNamesTheProxy checks that a failed attempt through a
// proxy says so: nothing in the error itself tells the cluster apart from
// the proxy in front of it.
func TestConnectionFailureNamesTheProxy(t *testing.T) {
	srv := httptest.NewServer(rootHandler(elasticsearchRoot))
	defer srv.Close()
	// A proxy that isn't there: nothing listens on its port anymore.
	gone := httptest.NewServer(http.NotFoundHandler())
	proxyURL := gone.URL
	gone.Close()

	screen, results := connectOnceTo(t, config.Cluster{URL: srv.URL, AuthType: config.AuthNone, Proxy: proxyURL})
	// The proxy first, the error after it: however long the error, what it
	// went through stays in view.
	msgs := i18n.For(i18n.FR)
	failure := strings.TrimSuffix(msgs.ErrConnectFailedFmt, "%s")
	waitForScreen(t, screen, fmt.Sprintf(msgs.HintProxyConfigFmt, proxyURL)+" "+failure)
	select {
	case <-results:
		t.Error("expected no connection")
	case <-time.After(300 * time.Millisecond):
	}
}

// TestProxyAnswerIsNotTakenForTheCluster checks the other way a proxy fails
// a connection: by answering in the cluster's place. The status is reported
// with the proxy it went through.
func TestProxyAnswerIsNotTakenForTheCluster(t *testing.T) {
	var reached atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Store(true) }))
	defer srv.Close()
	denying := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Proxy-Authenticate", `Basic realm="proxy"`)
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	defer denying.Close()

	screen, results := connectOnceTo(t, config.Cluster{URL: srv.URL, AuthType: config.AuthNone, Proxy: denying.URL})
	msgs := i18n.For(i18n.FR)
	waitForScreen(t, screen, fmt.Sprintf(msgs.HintProxyConfigFmt, denying.URL)+" "+fmt.Sprintf(msgs.ErrClusterHTTPFmt, http.StatusProxyAuthRequired))
	select {
	case <-results:
		t.Error("expected no connection")
	case <-time.After(300 * time.Millisecond):
	}
	if reached.Load() {
		t.Error("expected nothing to reach the cluster")
	}
}

// TestUnusableProxyStopsTheAttempt checks that a "proxy:" which can't be
// used is reported before anything is sent anywhere.
func TestUnusableProxyStopsTheAttempt(t *testing.T) {
	var reached atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Store(true) }))
	defer srv.Close()

	screen, results := connectOnceTo(t, config.Cluster{URL: srv.URL, AuthType: config.AuthNone, Proxy: "https://proxy.example.test:3128"})
	waitForScreen(t, screen, "a proxy reached over TLS (https://) is not supported")
	select {
	case <-results:
		t.Error("expected no connection")
	case <-time.After(300 * time.Millisecond):
	}
	if reached.Load() {
		t.Error("expected nothing to be sent to the cluster")
	}
}

// TestLongestFormFitsAboveTheMessage checks that the three lines kept for
// the message leave the longest form — a cluster in https with a client
// certificate — whole on a standard 80x24 terminal: every field and both
// buttons.
func TestLongestFormFitsAboveTheMessage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.Config{Language: i18n.FR, DefaultTimeoutSeconds: 5, Clusters: []config.Cluster{{
		URL: "https://es.example.test:9200", AuthType: config.AuthMTLS, TLS: config.TLS{Verify: true},
	}}}
	screen := newTestConnectScreen(t, cfg)
	screen.SetSize(80, 24)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // the cluster, first in the list
	waitForDraw(t, screen)

	if width, height := screen.Size(); width != 80 || height != 24 {
		t.Fatalf("test setup: expected an 80x24 screen, got %dx%d", width, height)
	}
	msgs := i18n.For(i18n.FR)
	text := screenText(screen)
	t.Logf("the longest form on an 80x24 terminal:\n%s", text)
	for _, want := range []string{
		msgs.AuthFieldLabel, msgs.FieldClientCert, msgs.FieldClientKey, msgs.FieldKeyPassphrase,
		msgs.FieldCAFile, msgs.FieldVerifyTLS, msgs.ButtonConnect, msgs.ButtonCancel,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q to be visible, got:\n%s", want, text)
		}
	}
}

// TestProxyHint checks what completes a failure for each origin of the
// proxy: one the environment designates comes with the way to do without it
// — the case of a cluster that was reached directly before proxies were
// honored.
func TestProxyHint(t *testing.T) {
	for _, lang := range []string{i18n.FR, i18n.EN} {
		msgs := i18n.For(lang)
		if got := proxyHint(esclient.ProxyInfo{}, msgs); got != "" {
			t.Errorf("%s: expected nothing to add for a direct connection, got %q", lang, got)
		}

		fromEnv := proxyHint(esclient.ProxyInfo{URL: "http://proxy.example.com:3128", EnvVar: "HTTPS_PROXY"}, msgs)
		for _, want := range []string{"http://proxy.example.com:3128", "HTTPS_PROXY", "NO_PROXY", "proxy: none"} {
			if !strings.Contains(fromEnv, want) {
				t.Errorf("%s: expected %q in the hint for a proxy from the environment, got %q", lang, want, fromEnv)
			}
		}

		fromConfig := proxyHint(esclient.ProxyInfo{URL: "socks5://127.0.0.1:1080"}, msgs)
		if !strings.Contains(fromConfig, "socks5://127.0.0.1:1080") || !strings.Contains(fromConfig, "config.yaml") {
			t.Errorf("%s: expected the proxy and config.yaml in the hint, got %q", lang, fromConfig)
		}
		if strings.Contains(fromConfig, "NO_PROXY") {
			t.Errorf("%s: NO_PROXY has no say over a proxy set in config.yaml, got %q", lang, fromConfig)
		}
	}
}
