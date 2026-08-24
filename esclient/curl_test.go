package esclient

import (
	"strings"
	"testing"
)

// newTestClient builds a Client directly from params, bypassing New()'s
// eager file-loading (CA/client cert/key) — CurlCommand only reads baseURL
// and params, never touches the file system or c.http, so tests can use
// paths that don't actually exist on disk.
func newTestClient(url string, params Params) *Client {
	params.URL = url
	return &Client{baseURL: strings.TrimRight(url, "/"), params: params}
}

// TestCurlCommandNoAuth checks the base shape of the generated command: the
// method, the full joined URL, and (with a body) the JSON content type and
// -d flag.
func TestCurlCommandNoAuth(t *testing.T) {
	client := newTestClient("https://es.example.com:9200", Params{Verify: true})

	got := client.CurlCommand("get", "_cat/health?v", nil)
	want := "curl -X GET 'https://es.example.com:9200/_cat/health?v'"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestCurlCommandWithBody checks that a request body is included with a
// Content-Type header, matching what Execute itself sends.
func TestCurlCommandWithBody(t *testing.T) {
	client := newTestClient("https://es.example.com:9200", Params{Verify: true})

	got := client.CurlCommand("POST", "_search", []byte(`{"query":{"match_all":{}}}`))
	want := "curl -X POST" +
		" -H 'Content-Type: application/json' -d '{\"query\":{\"match_all\":{}}}'" +
		" 'https://es.example.com:9200/_search'"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestCurlCommandBasicAuthRedactsPassword checks that Basic Auth includes
// the real username but replaces the password with a placeholder — per the
// user's explicit choice: a copied curl command must never carry a live
// secret in clear text.
func TestCurlCommandBasicAuthRedactsPassword(t *testing.T) {
	client := newTestClient("https://es.example.com:9200", Params{
		Verify: true, AuthType: AuthBasic, Username: "svc_devtools", Password: "hunter2",
	})

	got := client.CurlCommand("GET", "_cat/indices", nil)
	if !strings.Contains(got, "-u 'svc_devtools:<password>'") {
		t.Errorf("expected the real username with a redacted password, got %q", got)
	}
	if strings.Contains(got, "hunter2") {
		t.Errorf("expected the real password NOT to appear in the command, got %q", got)
	}
}

// TestCurlCommandAPIKeyRedactsSecret checks that API Key auth includes the
// real (non-secret) key ID but replaces the secret with a placeholder.
func TestCurlCommandAPIKeyRedactsSecret(t *testing.T) {
	client := newTestClient("https://es.example.com:9200", Params{
		Verify: true, AuthType: AuthAPIKey, APIKeyID: "myKeyId123", APIKeySecret: "supersecret",
	})

	got := client.CurlCommand("GET", "_cat/indices", nil)
	if !strings.Contains(got, "myKeyId123") {
		t.Errorf("expected the real API key ID to appear, got %q", got)
	}
	if strings.Contains(got, "supersecret") {
		t.Errorf("expected the real API key secret NOT to appear in the command, got %q", got)
	}
	if !strings.Contains(got, "<api_key_secret>") {
		t.Errorf("expected a placeholder for the API key secret, got %q", got)
	}
}

// TestCurlCommandMTLSIncludesPathsRedactsPassphrase checks that mTLS
// includes the real (non-secret) certificate/key file paths, but replaces
// the private key passphrase with a placeholder when one was used.
func TestCurlCommandMTLSIncludesPathsRedactsPassphrase(t *testing.T) {
	client := newTestClient("https://es.example.com:9200", Params{
		Verify: true, AuthType: AuthMTLS,
		ClientCert: "/etc/pki/tls/certs/client.crt", ClientKey: "/etc/pki/tls/certs/client.key",
		KeyPassphrase: "letmein",
	})

	got := client.CurlCommand("GET", "_cat/indices", nil)
	if !strings.Contains(got, "--cert '/etc/pki/tls/certs/client.crt'") {
		t.Errorf("expected the real client cert path, got %q", got)
	}
	if !strings.Contains(got, "--key '/etc/pki/tls/certs/client.key'") {
		t.Errorf("expected the real client key path, got %q", got)
	}
	if strings.Contains(got, "letmein") {
		t.Errorf("expected the real key passphrase NOT to appear in the command, got %q", got)
	}
	if !strings.Contains(got, "--pass '<key_passphrase>'") {
		t.Errorf("expected a placeholder for the key passphrase, got %q", got)
	}
}

// TestCurlCommandMTLSNoPassphraseFlag checks that --pass is omitted
// entirely when the private key wasn't encrypted — no placeholder for
// something that was never needed.
func TestCurlCommandMTLSNoPassphraseFlag(t *testing.T) {
	client := newTestClient("https://es.example.com:9200", Params{
		Verify: true, AuthType: AuthMTLS,
		ClientCert: "/cert.pem", ClientKey: "/key.pem",
	})

	if got := client.CurlCommand("GET", "_cat/indices", nil); strings.Contains(got, "--pass") {
		t.Errorf("expected no --pass flag when no key passphrase was used, got %q", got)
	}
}

// TestCurlCommandTLSFlags checks --cacert and -k (insecure) are included
// when configured.
func TestCurlCommandTLSFlags(t *testing.T) {
	client := newTestClient("https://es.example.com:9200", Params{
		CAFile: "/etc/pki/tls/certs/ca.pem", Verify: false,
	})

	got := client.CurlCommand("GET", "_cat/indices", nil)
	if !strings.Contains(got, "--cacert '/etc/pki/tls/certs/ca.pem'") {
		t.Errorf("expected --cacert with the configured CA path, got %q", got)
	}
	if !strings.Contains(got, " -k") {
		t.Errorf("expected -k (insecure) since Verify is false, got %q", got)
	}
}

// TestShellQuoteEscapesSingleQuotes checks the POSIX-safe single-quote
// escaping a body/header/URL containing an apostrophe needs to remain
// paste-safe.
func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	got := shellQuote(`it's "quoted"`)
	want := `'it'"'"'s "quoted"'`
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}
