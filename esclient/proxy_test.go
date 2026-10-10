package esclient

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// envProxy is the proxy the environment designates, for every test of this
// package: http.ProxyFromEnvironment reads the environment once per process,
// so the variables are set before any test runs rather than by the tests
// that need them. It carries credentials, as an authenticated proxy does.
//
// The tests that reach a server on a loopback address — all the others —
// are not affected: such an address is never proxied.
var envProxy *testProxy

const (
	envProxyUser     = "proxyuser"
	envProxyPassword = "proxysecret"
	// noProxyHost is the one name NO_PROXY excludes.
	noProxyHost = "direct.example.test"
)

func TestMain(m *testing.M) {
	envProxy = startTestProxy()
	withCredentials := "http://" + envProxyUser + ":" + envProxyPassword + "@" + envProxy.addr()
	// The lowercase spellings first: on Windows they are the same variables.
	for _, name := range []string{"http_proxy", "https_proxy", "no_proxy", "REQUEST_METHOD"} {
		os.Unsetenv(name)
	}
	os.Setenv("HTTP_PROXY", withCredentials)
	os.Setenv("HTTPS_PROXY", withCredentials)
	os.Setenv("NO_PROXY", noProxyHost)

	code := m.Run()
	envProxy.close()
	os.Exit(code)
}

// testProxy is a minimal HTTP proxy: it forwards plain requests, opens
// CONNECT tunnels, and records what it was asked — all a proxy gets to see.
type testProxy struct {
	server *httptest.Server
	direct *http.Transport

	mu       sync.Mutex
	seen     []proxiedRequest
	backends map[string]string // "host:port" asked for → address really dialed
}

type proxiedRequest struct {
	method string // "CONNECT", or the method of a forwarded request
	target string // "host:port" for CONNECT, the absolute URL otherwise
	header http.Header
}

func startTestProxy() *testProxy {
	p := &testProxy{direct: &http.Transport{}, backends: map[string]string{}}
	p.server = httptest.NewServer(p)
	return p
}

func (p *testProxy) addr() string { return p.server.Listener.Addr().String() }

func (p *testProxy) close() {
	p.direct.CloseIdleConnections()
	p.server.Close()
}

// route makes the proxy reach backend when asked for hostPort, a name that
// doesn't resolve: a proxy is only ever asked for names here, since a
// loopback address is never sent through one.
func (p *testProxy) route(t *testing.T, hostPort, backend string) {
	t.Helper()
	p.mu.Lock()
	p.backends[hostPort] = backend
	p.mu.Unlock()
	t.Cleanup(func() {
		p.mu.Lock()
		delete(p.backends, hostPort)
		p.mu.Unlock()
	})
}

func (p *testProxy) backendFor(hostPort string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if backend, ok := p.backends[hostPort]; ok {
		return backend
	}
	return hostPort
}

func (p *testProxy) reset() {
	p.mu.Lock()
	p.seen = nil
	p.mu.Unlock()
}

func (p *testProxy) requests() []proxiedRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]proxiedRequest(nil), p.seen...)
}

func (p *testProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := r.URL.String()
	if r.Method == http.MethodConnect {
		target = r.Host
	}
	p.mu.Lock()
	p.seen = append(p.seen, proxiedRequest{method: r.Method, target: target, header: r.Header.Clone()})
	p.mu.Unlock()

	if r.Method == http.MethodConnect {
		p.tunnel(w, r)
		return
	}
	p.forward(w, r)
}

func (p *testProxy) tunnel(w http.ResponseWriter, r *http.Request) {
	backend, err := net.Dial("tcp", p.backendFor(r.Host))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer backend.Close()
	client, buffered, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	relay(client, buffered, backend)
}

func (p *testProxy) forward(w http.ResponseWriter, r *http.Request) {
	out := r.Clone(r.Context())
	out.RequestURI = ""
	out.URL.Host = p.backendFor(r.URL.Host)
	// What is addressed to the proxy stops at the proxy.
	out.Header.Del("Proxy-Authorization")
	out.Header.Del("Proxy-Connection")
	resp, err := p.direct.RoundTrip(out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for name, values := range resp.Header {
		w.Header()[name] = values
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// relay copies both ways between client and backend until either side is
// done; the caller then closes both.
func relay(client net.Conn, fromClient io.Reader, backend net.Conn) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(backend, fromClient); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, backend); done <- struct{}{} }()
	<-done
}

// testSOCKS5 is a minimal SOCKS5 server (RFC 1928: no authentication,
// CONNECT only) relaying to backend whatever it is asked to reach, and
// recording what that was.
type testSOCKS5 struct {
	listener net.Listener
	backend  string

	mu    sync.Mutex
	asked []string
}

func startTestSOCKS5(t *testing.T, backend string) *testSOCKS5 {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	s := &testSOCKS5{listener: listener, backend: backend}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *testSOCKS5) addr() string { return s.listener.Addr().String() }

func (s *testSOCKS5) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.asked...)
}

func (s *testSOCKS5) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)

	// Greeting: version, number of methods, the methods.
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(r, greeting); err != nil || greeting[0] != 5 {
		return
	}
	if _, err := io.CopyN(io.Discard, r, int64(greeting[1])); err != nil {
		return
	}
	_, _ = conn.Write([]byte{5, 0}) // version 5, no authentication

	// Request: version, command (1 = CONNECT), reserved, address type.
	request := make([]byte, 4)
	if _, err := io.ReadFull(r, request); err != nil || request[1] != 1 {
		return
	}
	var host string
	switch request[3] {
	case 1, 4: // IPv4, IPv6
		ip := make([]byte, map[byte]int{1: net.IPv4len, 4: net.IPv6len}[request[3]])
		if _, err := io.ReadFull(r, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 3: // a name, left to the proxy to resolve
		length, err := r.ReadByte()
		if err != nil {
			return
		}
		name := make([]byte, length)
		if _, err := io.ReadFull(r, name); err != nil {
			return
		}
		host = string(name)
	default:
		return
	}
	port := make([]byte, 2)
	if _, err := io.ReadFull(r, port); err != nil {
		return
	}
	s.mu.Lock()
	s.asked = append(s.asked, net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port)))))
	s.mu.Unlock()

	backend, err := net.Dial("tcp", s.backend)
	if err != nil {
		_, _ = conn.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0}) // general failure
		return
	}
	defer backend.Close()
	_, _ = conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}) // succeeded
	relay(conn, r, backend)
}

// tlsCluster starts an https server standing for a cluster, and returns it
// with the path of a CA file vouching for it. Its certificate (httptest's)
// is valid for "example.com": the name the tests reach it under, through a
// proxy routed to it.
func tlsCluster(t *testing.T, handler http.HandlerFunc) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	caFile := filepath.Join(t.TempDir(), "cluster-ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return srv, caFile
}

func newProxyTestClient(t *testing.T, params Params) *Client {
	t.Helper()
	client, err := New(params)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Before the servers close: a tunnel lives as long as its connection.
	t.Cleanup(client.http.CloseIdleConnections)
	return client
}

func basicAuth(user, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
}

// TestEnvironmentProxyTunnelsAnHTTPSCluster is the common corporate case: a
// cluster in https behind the proxy HTTPS_PROXY designates. The proxy only
// opens a tunnel — TLS is verified end to end against the cluster's CA — and
// each side is given its own credentials, never the other's.
func TestEnvironmentProxyTunnelsAnHTTPSCluster(t *testing.T) {
	clusterSaw := make(chan http.Header, 1)
	cluster, caFile := tlsCluster(t, func(w http.ResponseWriter, r *http.Request) {
		clusterSaw <- r.Header.Clone()
		_, _ = w.Write([]byte(`{"tagline":"You Know, for Search"}`))
	})
	envProxy.reset()
	envProxy.route(t, "example.com:9443", cluster.Listener.Addr().String())

	client := newProxyTestClient(t, Params{
		URL: "https://example.com:9443", Verify: true, CAFile: caFile,
		AuthType: AuthBasic, Username: "elastic", Password: "clustersecret",
	})
	result, err := client.Execute(context.Background(), "GET", "/", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), "You Know, for Search") {
		t.Errorf("expected the cluster's answer, got HTTP %d %q", result.StatusCode, result.Body)
	}

	seen := envProxy.requests()
	if len(seen) != 1 || seen[0].method != http.MethodConnect || seen[0].target != "example.com:9443" {
		t.Fatalf("expected the proxy to be asked for one tunnel to example.com:9443, got %+v", seen)
	}
	if got, want := seen[0].header.Get("Proxy-Authorization"), basicAuth(envProxyUser, envProxyPassword); got != want {
		t.Errorf("expected the proxy to be given its credentials (%q), got %q", want, got)
	}
	if got := seen[0].header.Get("Authorization"); got != "" {
		t.Errorf("expected the cluster's credentials to be kept from the proxy, it got %q", got)
	}

	headers := <-clusterSaw
	if got, want := headers.Get("Authorization"), basicAuth("elastic", "clustersecret"); got != want {
		t.Errorf("expected the cluster to be given its credentials (%q), got %q", want, got)
	}
	if got := headers.Get("Proxy-Authorization"); got != "" {
		t.Errorf("expected the proxy's credentials to be kept from the cluster, it got %q", got)
	}

	// What is shown of the proxy: where it is and where it comes from, not
	// its credentials.
	if got, want := client.Proxy(), (ProxyInfo{URL: "http://" + envProxy.addr(), EnvVar: "HTTPS_PROXY"}); got != want {
		t.Errorf("expected %+v, got %+v", want, got)
	}
}

// TestEnvironmentProxyForwardsAnHTTPCluster checks the other variable: a
// cluster in plain http goes through the proxy HTTP_PROXY designates, which
// is handed the whole request.
func TestEnvironmentProxyForwardsAnHTTPCluster(t *testing.T) {
	cluster := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("green"))
	}))
	defer cluster.Close()
	envProxy.reset()
	envProxy.route(t, "cluster.example.test:9200", cluster.Listener.Addr().String())

	client := newProxyTestClient(t, Params{URL: "http://cluster.example.test:9200"})
	result, err := client.Execute(context.Background(), "GET", "_cat/health", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if string(result.Body) != "green" {
		t.Errorf("expected the cluster's answer, got HTTP %d %q", result.StatusCode, result.Body)
	}

	seen := envProxy.requests()
	if len(seen) != 1 || seen[0].method != http.MethodGet || seen[0].target != "http://cluster.example.test:9200/_cat/health" {
		t.Fatalf("expected the proxy to be handed the request, got %+v", seen)
	}
	if got, want := client.Proxy(), (ProxyInfo{URL: "http://" + envProxy.addr(), EnvVar: "HTTP_PROXY"}); got != want {
		t.Errorf("expected %+v, got %+v", want, got)
	}
}

// TestClustersTheEnvironmentLeavesAlone checks who is reached directly even
// though the environment designates a proxy: what NO_PROXY excludes, and any
// loopback address (a local cluster, the near end of an SSH tunnel).
func TestClustersTheEnvironmentLeavesAlone(t *testing.T) {
	for _, clusterURL := range []string{
		"https://" + noProxyHost + ":9200",
		"http://localhost:9200",
		"https://127.0.0.1:9200",
		"http://[::1]:9200",
	} {
		if info, err := ProxyFor(clusterURL, ""); err != nil || info != (ProxyInfo{}) {
			t.Errorf("%s: expected a direct connection, got %+v (%v)", clusterURL, info, err)
		}
	}

	cluster := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer cluster.Close()
	envProxy.reset()

	client := newProxyTestClient(t, Params{URL: cluster.URL})
	if _, err := client.Execute(context.Background(), "GET", "/", nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if seen := envProxy.requests(); len(seen) != 0 {
		t.Errorf("expected a loopback address to be reached directly, the proxy saw %+v", seen)
	}
	if got := client.Proxy(); got != (ProxyInfo{}) {
		t.Errorf("expected no proxy to be reported, got %+v", got)
	}
}

// TestClusterProxySettingOverridesTheEnvironment checks a cluster's own
// "proxy:" in config.yaml: that proxy is used, the environment's is not.
func TestClusterProxySettingOverridesTheEnvironment(t *testing.T) {
	cluster := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("green"))
	}))
	defer cluster.Close()
	own := startTestProxy()
	defer own.close()
	own.route(t, "cluster.example.test:9200", cluster.Listener.Addr().String())
	envProxy.reset()

	client := newProxyTestClient(t, Params{URL: "http://cluster.example.test:9200", Proxy: "http://" + own.addr()})
	result, err := client.Execute(context.Background(), "GET", "_cat/health", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if string(result.Body) != "green" {
		t.Errorf("expected the cluster's answer, got HTTP %d %q", result.StatusCode, result.Body)
	}

	if seen := own.requests(); len(seen) != 1 {
		t.Errorf("expected the cluster's own proxy to be used once, it saw %+v", seen)
	}
	if seen := envProxy.requests(); len(seen) != 0 {
		t.Errorf("expected the environment's proxy to be left out, it saw %+v", seen)
	}
	if got, want := client.Proxy(), (ProxyInfo{URL: "http://" + own.addr()}); got != want {
		t.Errorf("expected %+v, got %+v", want, got)
	}
}

// TestProxyNoneIgnoresTheEnvironment checks the way out for a cluster the
// environment would wrongly send through its proxy: "proxy: none".
func TestProxyNoneIgnoresTheEnvironment(t *testing.T) {
	const clusterURL = "https://example.com:9443"
	if info, err := ProxyFor(clusterURL, ""); err != nil || info.URL == "" {
		t.Fatalf("test setup: expected the environment to designate a proxy for %s, got %+v (%v)", clusterURL, info, err)
	}

	for _, setting := range []string{"none", " None "} {
		if info, err := ProxyFor(clusterURL, setting); err != nil || info != (ProxyInfo{}) {
			t.Errorf("%q: expected a direct connection, got %+v (%v)", setting, info, err)
		}
		client, err := New(Params{URL: clusterURL, Verify: true, Proxy: setting})
		if err != nil {
			t.Fatalf("%q: New: %v", setting, err)
		}
		if client.http.Transport.(*http.Transport).Proxy != nil {
			t.Errorf("%q: expected requests to be sent without asking for a proxy", setting)
		}
		if got := client.Proxy(); got != (ProxyInfo{}) {
			t.Errorf("%q: expected no proxy to be reported, got %+v", setting, got)
		}
	}
}

// TestUnusableProxySettingsAreRefused checks that a "proxy:" which can't be
// used fails the connection before anything is sent — and that a password
// typed there by mistake isn't repeated in the error.
func TestUnusableProxySettingsAreRefused(t *testing.T) {
	cases := map[string]struct{ setting, want string }{
		"reached over TLS":             {"https://proxy.example.test:3128", "TLS"},
		"with credentials":             {"http://admin:hunter2@proxy.example.test:3128", "no credentials"},
		"with credentials, unparsable": {"http://admin:hunter2@[::1", "expected http://host:port"},
		"without a scheme":             {"proxy.example.test:3128", "expected http://host:port"},
		"a bare host":                  {"proxy.example.test", "expected http://host:port"},
		"of another kind":              {"ftp://proxy.example.test", "unsupported proxy scheme"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			client, err := New(Params{URL: "https://es.example.test:9200", Verify: true, Proxy: c.setting})
			if err == nil {
				t.Fatalf("expected %q to be refused, got a client going through %+v", c.setting, client.Proxy())
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected an error mentioning %q, got: %v", c.want, err)
			}
			if strings.Contains(err.Error(), "hunter2") {
				t.Errorf("expected the password to be left out of the error, got: %v", err)
			}
			if _, err := ProxyFor("https://es.example.test:9200", c.setting); err == nil {
				t.Errorf("expected ProxyFor to refuse %q as well", c.setting)
			}
		})
	}
}

// TestProxiesFromTheEnvironmentAreVetted checks what is made of the
// environment's answer: the same proxies are refused as in config.yaml, and
// nothing shown about them carries the credentials the variable may hold.
func TestProxiesFromTheEnvironmentAreVetted(t *testing.T) {
	target, _ := url.Parse("https://es.example.test:9200")

	overTLS, _ := url.Parse("https://proxyuser:proxysecret@proxy.example.test:3128")
	_, err := vetEnvProxy(overTLS, nil, target)
	if err == nil {
		t.Fatal("expected a proxy reached over TLS to be refused")
	}
	for _, want := range []string{"https://proxy.example.test:3128", "HTTPS_PROXY", "TLS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected an error mentioning %q, got: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "proxysecret") || strings.Contains(err.Error(), "proxyuser") {
		t.Errorf("expected the proxy's credentials to be left out of the error, got: %v", err)
	}

	// The standard library's own error quotes the variable as it is.
	_, err = vetEnvProxy(nil, errors.New(`invalid proxy address "http://proxyuser:proxysecret@["`), target)
	if err == nil || !strings.Contains(err.Error(), "HTTPS_PROXY") {
		t.Errorf("expected an error naming the variable, got: %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "proxysecret") {
		t.Errorf("expected the proxy's credentials to be left out of the error, got: %v", err)
	}

	for _, usable := range []string{"http://proxyuser:proxysecret@proxy.example.test:3128", "socks5://127.0.0.1:1080", "socks5h://127.0.0.1:1080"} {
		proxy, _ := url.Parse(usable)
		got, err := vetEnvProxy(proxy, nil, target)
		if err != nil || got != proxy {
			t.Errorf("%s: expected the proxy to be used as it is, got %v (%v)", usable, got, err)
		}
	}
	if got, err := vetEnvProxy(nil, nil, target); got != nil || err != nil {
		t.Errorf("expected no proxy to stay no proxy, got %v (%v)", got, err)
	}
}

// TestSOCKS5Proxy checks the other kind of proxy accepted — what "ssh -D"
// opens: the cluster's name is handed to the proxy to resolve, and TLS is
// still verified end to end.
func TestSOCKS5Proxy(t *testing.T) {
	cluster, caFile := tlsCluster(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tagline":"You Know, for Search"}`))
	})
	socks := startTestSOCKS5(t, cluster.Listener.Addr().String())
	envProxy.reset()

	client := newProxyTestClient(t, Params{
		URL: "https://example.com:9444", Verify: true, CAFile: caFile,
		Proxy: "socks5://" + socks.addr(),
	})
	result, err := client.Execute(context.Background(), "GET", "/", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), "You Know, for Search") {
		t.Errorf("expected the cluster's answer, got HTTP %d %q", result.StatusCode, result.Body)
	}

	if asked := socks.requests(); len(asked) != 1 || asked[0] != "example.com:9444" {
		t.Errorf("expected the proxy to be asked for example.com:9444 by name, got %v", asked)
	}
	if seen := envProxy.requests(); len(seen) != 0 {
		t.Errorf("expected the environment's proxy to be left out, it saw %+v", seen)
	}
	if got, want := client.Proxy(), (ProxyInfo{URL: "socks5://" + socks.addr()}); got != want {
		t.Errorf("expected %+v, got %+v", want, got)
	}
}
