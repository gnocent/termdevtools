package esclient

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ProxyNone is the value of Params.Proxy that forces a direct connection,
// whatever the environment says.
const ProxyNone = "none"

// ProxyInfo tells which proxy a connection goes through, for display: a
// proxy sees where every request goes — and, for a cluster reached over
// plain http, everything in it — so it is never used without being shown.
// The zero value means a direct connection.
type ProxyInfo struct {
	// URL is the proxy's address, without the credentials it may carry in
	// the environment.
	URL string
	// EnvVar names the environment variable the proxy comes from
	// ("HTTPS_PROXY" or "HTTP_PROXY"); empty when it is the cluster's own
	// "proxy:" setting in config.yaml.
	EnvVar string
}

// ProxyFor tells which proxy, if any, a connection to clusterURL goes
// through. setting is the cluster's "proxy:" in config.yaml (Params.Proxy):
// a proxy URL, ProxyNone, or empty to leave the decision to the environment
// (HTTPS_PROXY, HTTP_PROXY, NO_PROXY — a loopback address is never proxied).
// It is the decision the Client itself makes for every request, available
// before there is a Client: the connection screen names the proxy before
// anything is sent through it. See SPEC.md §5.
func ProxyFor(clusterURL, setting string) (ProxyInfo, error) {
	choose, err := proxyFunc(setting)
	if err != nil || choose == nil {
		return ProxyInfo{}, err
	}
	target, err := url.Parse(clusterURL)
	if err != nil {
		// Not this function's to report: the request itself will fail on it.
		return ProxyInfo{}, nil
	}
	proxy, err := choose(&http.Request{URL: target})
	if err != nil || proxy == nil {
		return ProxyInfo{}, err
	}
	info := ProxyInfo{URL: displayProxy(proxy)}
	if strings.TrimSpace(setting) == "" {
		info.EnvVar = proxyEnvVar(target)
	}
	return info, nil
}

// proxyFunc returns what http.Transport asks, for each request, which proxy
// to go through — nil for none at all.
func proxyFunc(setting string) (func(*http.Request) (*url.URL, error), error) {
	switch setting = strings.TrimSpace(setting); {
	case setting == "":
		return proxyFromEnvironment, nil
	case strings.EqualFold(setting, ProxyNone):
		return nil, nil
	default:
		proxy, err := parseProxySetting(setting)
		if err != nil {
			return nil, err
		}
		return http.ProxyURL(proxy), nil
	}
}

// parseProxySetting reads a cluster's "proxy:" from config.yaml. Neither the
// value nor the parsing error is quoted in what it returns: either could
// hold a password typed there by mistake.
func parseProxySetting(setting string) (*url.URL, error) {
	malformed := errors.New(`invalid "proxy:" setting for this cluster in config.yaml: expected http://host:port, socks5://host:port or "none"`)
	proxy, err := url.Parse(setting)
	if err != nil {
		return nil, malformed
	}
	// Same rule as for the cluster's URL: config.yaml holds no secret.
	if proxy.User != nil {
		return nil, errors.New(`no credentials in the "proxy:" setting of config.yaml: nothing secret is stored there; an authenticated proxy goes through the HTTPS_PROXY environment variable`)
	}
	if proxy.Host == "" {
		return nil, malformed
	}
	if err := checkProxyScheme(proxy); err != nil {
		return nil, fmt.Errorf(`"proxy:" setting of config.yaml: %w`, err)
	}
	return proxy, nil
}

// proxyFromEnvironment is http.ProxyFromEnvironment, restricted to the
// proxies this program accepts.
func proxyFromEnvironment(req *http.Request) (*url.URL, error) {
	proxy, err := http.ProxyFromEnvironment(req)
	return vetEnvProxy(proxy, err, req.URL)
}

// vetEnvProxy decides what to make of the standard library's answer for a
// request to target. Its error is not passed on: it quotes the variable's
// value, credentials included, and would end up on screen.
func vetEnvProxy(proxy *url.URL, err error, target *url.URL) (*url.URL, error) {
	if err != nil {
		return nil, fmt.Errorf("the proxy set in %s is not a valid address", proxyEnvVar(target))
	}
	if proxy == nil {
		return nil, nil
	}
	if err := checkProxyScheme(proxy); err != nil {
		return nil, fmt.Errorf("proxy %s (from %s): %w", displayProxy(proxy), proxyEnvVar(target), err)
	}
	return proxy, nil
}

// checkProxyScheme accepts a plain HTTP proxy — a cluster in https is then
// reached through a CONNECT tunnel, encrypted end to end — and a SOCKS5 one
// (what "ssh -D" opens).
//
// A proxy itself reached over TLS is refused: http.Transport has a single
// TLS configuration, the cluster's, and would apply it to the proxy as well
// — verification turned off for the cluster would be off for the proxy, a CA
// file given for the cluster would be the only authority trusted for the
// proxy, and the client certificate meant for the cluster would be offered
// to it.
func checkProxyScheme(proxy *url.URL) error {
	switch proxy.Scheme {
	case "http", "socks5", "socks5h":
		return nil
	case "https":
		return errors.New("a proxy reached over TLS (https://) is not supported, use an http:// or socks5:// one")
	default:
		return fmt.Errorf("unsupported proxy scheme %q, use http:// or socks5://", proxy.Scheme)
	}
}

// proxyEnvVar names the variable http.ProxyFromEnvironment reads for a
// request to target (its lowercase spelling is read too).
func proxyEnvVar(target *url.URL) string {
	if target.Scheme == "https" {
		return "HTTPS_PROXY"
	}
	return "HTTP_PROXY"
}

// displayProxy renders a proxy's address for display: scheme and host only,
// never the credentials.
func displayProxy(proxy *url.URL) string {
	return (&url.URL{Scheme: proxy.Scheme, Host: proxy.Host}).String()
}
