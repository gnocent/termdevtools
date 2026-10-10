//go:build integration

// Integration tests: authentication against a real cluster with security on.
//
//	wsl.exe -- bash tools/testclusters.sh up secured-es-9.5.4
//	go test -tags integration -run Integration ./esclient/
//
// They create, use and delete an API key and a service account token on the
// throwaway cluster tools/testclusters.sh starts for the purpose — a
// container on this machine, with a password that protects nothing.
package esclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"termdevtools/internal/testclusters"
)

// securedCall sends one request to the secured cluster, authenticated as
// params say, and returns the status and the JSON body of the answer.
func securedCall(t *testing.T, params Params, method, path, body string) (int, map[string]any) {
	t.Helper()
	params.URL = testclusters.Secured.URL
	client, err := New(params)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := client.Execute(ctx, method, path, []byte(body))
	if err != nil {
		t.Fatalf("%s %s: %v — is the secured cluster up? (tools/testclusters.sh up %s)", method, path, err, testclusters.Secured.Name)
	}
	var decoded map[string]any
	_ = json.Unmarshal(result.Body, &decoded)
	return result.StatusCode, decoded
}

// TestIntegrationAuthentication checks every way to authenticate that
// carries a secret against a cluster that really verifies it — which the
// other tests, facing servers that accept anything, cannot: each is accepted
// with the right secret, as who it should be, and turned down with a wrong
// one.
func TestIntegrationAuthentication(t *testing.T) {
	secured := testclusters.Secured
	basic := Params{AuthType: AuthBasic, Username: secured.Username, Password: secured.Password}
	const whoAmI = "_security/_authenticate"

	// Without the right credentials, nothing: this cluster does check them.
	if status, _ := securedCall(t, Params{AuthType: AuthNone}, "GET", "/", ""); status != http.StatusUnauthorized {
		t.Fatalf("expected the cluster to turn down an anonymous request, got HTTP %d: is security on?", status)
	}
	if status, _ := securedCall(t, Params{AuthType: AuthBasic, Username: secured.Username, Password: "not-the-password"}, "GET", "/", ""); status != http.StatusUnauthorized {
		t.Errorf("Basic Auth: expected a wrong password to be turned down, got HTTP %d", status)
	}
	if status, who := securedCall(t, basic, "GET", whoAmI, ""); status != http.StatusOK || who["username"] != secured.Username {
		t.Fatalf("Basic Auth: expected to be %q, got HTTP %d %v", secured.Username, status, who)
	}

	name := fmt.Sprintf("tdt-it-%d", time.Now().UnixNano())

	t.Run("API key", func(t *testing.T) {
		status, key := securedCall(t, basic, "POST", "_security/api_key", `{"name": "`+name+`"}`)
		id, _ := key["id"].(string)
		secret, _ := key["api_key"].(string)
		encoded, _ := key["encoded"].(string)
		if status != http.StatusOK || id == "" || secret == "" || encoded == "" {
			t.Fatalf("creating an API key: HTTP %d %v", status, key)
		}
		t.Cleanup(func() { securedCall(t, basic, "DELETE", "_security/api_key", `{"ids": ["`+id+`"]}`) })

		for label, typed := range map[string][2]string{
			"identifier and secret":                     {id, secret},
			"encoded form, no identifier":               {"", encoded},
			"encoded form, another identifier in place": {"an-older-key", encoded},
		} {
			resolvedID, resolvedSecret := ResolveAPIKey(typed[0], typed[1])
			if resolvedID != id {
				t.Errorf("%s: expected the key's identifier %q, got %q", label, id, resolvedID)
			}
			status, who := securedCall(t, Params{AuthType: AuthAPIKey, APIKeyID: resolvedID, APIKeySecret: resolvedSecret}, "GET", whoAmI, "")
			if status != http.StatusOK || who["authentication_type"] != "api_key" {
				t.Errorf("%s: expected to be authenticated by the API key, got HTTP %d %v", label, status, who)
			}
		}

		if status, _ := securedCall(t, Params{AuthType: AuthAPIKey, APIKeyID: id, APIKeySecret: "not-the-secret"}, "GET", whoAmI, ""); status != http.StatusUnauthorized {
			t.Errorf("expected a wrong secret to be turned down, got HTTP %d", status)
		}
	})

	t.Run("Bearer token", func(t *testing.T) {
		// A service account token: the bearer token a cluster issues without
		// any further setup.
		const account = "elastic/fleet-server"
		tokenPath := "_security/service/" + account + "/credential/token/" + name
		status, created := securedCall(t, basic, "POST", tokenPath, "")
		details, _ := created["token"].(map[string]any)
		token, _ := details["value"].(string)
		if status != http.StatusOK || token == "" {
			t.Fatalf("creating a service account token: HTTP %d %v", status, created)
		}
		t.Cleanup(func() { securedCall(t, basic, "DELETE", tokenPath, "") })

		for label, typed := range map[string]string{
			"the token":                token,
			"with the scheme in front": "Bearer " + token,
		} {
			status, who := securedCall(t, Params{AuthType: AuthBearer, BearerToken: typed}, "GET", whoAmI, "")
			if status != http.StatusOK || who["username"] != account {
				t.Errorf("%s: expected to be %q, got HTTP %d %v", label, account, status, who)
			}
		}

		if status, _ := securedCall(t, Params{AuthType: AuthBearer, BearerToken: "not-a-token"}, "GET", whoAmI, ""); status != http.StatusUnauthorized {
			t.Errorf("expected a wrong token to be turned down, got HTTP %d", status)
		}
	})
}
