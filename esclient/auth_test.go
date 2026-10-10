package esclient

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A made-up API key of the shape Elasticsearch generates — a 20-character
// identifier, a 22-character secret — and its "encoded" form.
const (
	testAPIKeyID     = "AbCdEfGhIjKl-nOpQrSt"
	testAPIKeySecret = "ZyXwVuTsRqPoNmLkJi_gFe"
)

var testAPIKeyEncoded = base64.StdEncoding.EncodeToString([]byte(testAPIKeyID + ":" + testAPIKeySecret))

// TestResolveAPIKey checks what is made of the secret field: an encoded key
// is recognized and brings its own identifier, anything else is the secret
// to go with the identifier given — a plain secret above all, which is
// itself made of base64 characters.
func TestResolveAPIKey(t *testing.T) {
	encode := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	cases := map[string]struct {
		id, secret         string
		wantID, wantSecret string
	}{
		"identifier and secret":                   {testAPIKeyID, testAPIKeySecret, testAPIKeyID, testAPIKeySecret},
		"encoded key, no identifier":              {"", testAPIKeyEncoded, testAPIKeyID, testAPIKeySecret},
		"encoded key replaces another identifier": {"an-older-key", testAPIKeyEncoded, testAPIKeyID, testAPIKeySecret},
		"encoded key without its padding":         {"", strings.TrimRight(testAPIKeyEncoded, "="), testAPIKeyID, testAPIKeySecret},
		"encoded key pasted with a line break":    {"", " " + testAPIKeyEncoded + "\n", testAPIKeyID, testAPIKeySecret},
		"secret alone, no identifier":             {"", testAPIKeySecret, "", testAPIKeySecret},
		"base64 of something else":                {"my-id", encode("no colon in here"), "my-id", encode("no colon in here")},
		"base64 of a part with a space":           {"my-id", encode("an id:a secret"), "my-id", encode("an id:a secret")},
		"base64 of an empty part":                 {"my-id", encode(":secret"), "my-id", encode(":secret")},
		"nothing":                                 {"my-id", "", "my-id", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			id, secret := ResolveAPIKey(c.id, c.secret)
			if id != c.wantID || secret != c.wantSecret {
				t.Errorf("expected (%q, %q), got (%q, %q)", c.wantID, c.wantSecret, id, secret)
			}
		})
	}
}

// authorizationSentWith returns the Authorization header a client built from
// params sends.
func authorizationSentWith(t *testing.T, params Params) string {
	t.Helper()
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	params.URL = srv.URL
	client, err := New(params)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Execute(context.Background(), "GET", "/", nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return <-received
}

// TestAPIKeyAuthorization checks that both ways of giving a key send the
// same header — which is "ApiKey" followed by the encoded form itself.
func TestAPIKeyAuthorization(t *testing.T) {
	want := "ApiKey " + testAPIKeyEncoded

	if got := authorizationSentWith(t, Params{AuthType: AuthAPIKey, APIKeyID: testAPIKeyID, APIKeySecret: testAPIKeySecret}); got != want {
		t.Errorf("identifier and secret: expected %q, got %q", want, got)
	}

	id, secret := ResolveAPIKey("", testAPIKeyEncoded)
	if got := authorizationSentWith(t, Params{AuthType: AuthAPIKey, APIKeyID: id, APIKeySecret: secret}); got != want {
		t.Errorf("encoded key: expected %q, got %q", want, got)
	}
}

// TestBearerAuthorization checks the header of a bearer token, including one
// pasted the way it is usually found: after "Bearer", or with a line break.
func TestBearerAuthorization(t *testing.T) {
	const token = "bm90LWEtcmVhbC10b2tlbg.only-the-shape-of-one"
	for name, typed := range map[string]string{
		"the token":                    token,
		"with the scheme in front":     "Bearer " + token,
		"with the scheme in lowercase": "bearer  " + token,
		"with spaces and a line break": "  " + token + "\r\n",
	} {
		if got, want := authorizationSentWith(t, Params{AuthType: AuthBearer, BearerToken: typed}), "Bearer "+token; got != want {
			t.Errorf("%s: expected %q, got %q", name, want, got)
		}
	}
}

// TestCurlCommandBearerRedactsToken checks that the copied command names the
// header to send and never the token itself.
func TestCurlCommandBearerRedactsToken(t *testing.T) {
	client := newTestClient("https://es.example.com:9200", Params{Verify: true, AuthType: AuthBearer, BearerToken: "s3cr3t-t0ken"})

	got := client.CurlCommand("GET", "_cat/health", nil)
	want := "curl -X GET -H 'Authorization: Bearer <token>' 'https://es.example.com:9200/_cat/health'"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	if strings.Contains(got, "s3cr3t-t0ken") {
		t.Errorf("the token must never appear in the generated command, got %q", got)
	}
}
