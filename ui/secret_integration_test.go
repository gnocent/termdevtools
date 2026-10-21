//go:build integration

// Integration test: secrets read from a variable or a file, against a real
// cluster with security on.
//
//	wsl.exe -- bash tools/testclusters.sh up secured-es-9.5.4
//	go test -tags integration -run Integration ./ui/
//
// It creates and deletes an API key and a service account token on the
// throwaway cluster tools/testclusters.sh starts for the purpose — a
// container on this machine, with a password that protects nothing.
package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/gnocent/termdevtools/config"
	"github.com/gnocent/termdevtools/esclient"
	"github.com/gnocent/termdevtools/internal/testclusters"
)

// securedRequest sends one request to the secured cluster through client,
// and returns the status and the JSON body of the answer.
func securedRequest(t *testing.T, client *esclient.Client, method, path, body string) (int, map[string]any) {
	t.Helper()
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

// TestIntegrationSecretWithoutRetyping connects to a cluster that really
// checks what it is sent, with each kind of secret read from a variable and
// from a file: the form opens on Connect, Enter alone connects, as who the
// secret says.
func TestIntegrationSecretWithoutRetyping(t *testing.T) {
	secured := testclusters.Secured
	admin, err := esclient.New(esclient.Params{
		URL: secured.URL, AuthType: config.AuthBasic, Username: secured.Username, Password: secured.Password, Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("esclient.New: %v", err)
	}
	name := fmt.Sprintf("tdt-it-secret-%d", time.Now().UnixNano())

	status, key := securedRequest(t, admin, "POST", "_security/api_key", `{"name": "`+name+`"}`)
	keyID, _ := key["id"].(string)
	encoded, _ := key["encoded"].(string)
	if status != http.StatusOK || encoded == "" {
		t.Fatalf("creating an API key: HTTP %d %v", status, key)
	}
	t.Cleanup(func() { securedRequest(t, admin, "DELETE", "_security/api_key", `{"ids": ["`+keyID+`"]}`) })

	const account = "elastic/fleet-server"
	tokenPath := "_security/service/" + account + "/credential/token/" + name
	status, created := securedRequest(t, admin, "POST", tokenPath, "")
	details, _ := created["token"].(map[string]any)
	token, _ := details["value"].(string)
	if status != http.StatusOK || token == "" {
		t.Fatalf("creating a service account token: HTTP %d %v", status, created)
	}
	t.Cleanup(func() { securedRequest(t, admin, "DELETE", tokenPath, "") })

	for _, c := range []struct {
		name    string
		cluster config.Cluster
		secret  string
		field   string // of _security/_authenticate's answer
		want    string // its value
	}{
		{"Basic", config.Cluster{AuthType: config.AuthBasic, Username: secured.Username}, secured.Password, "username", secured.Username},
		{"API key", config.Cluster{AuthType: config.AuthAPIKey}, encoded, "authentication_type", "api_key"},
		{"Bearer", config.Cluster{AuthType: config.AuthBearer}, token, "username", account},
	} {
		for _, source := range []string{"variable", "file"} {
			t.Run(c.name+" from a "+source, func(t *testing.T) {
				cluster := c.cluster
				cluster.URL = secured.URL
				if source == "variable" {
					t.Setenv("TDT_IT_SECRET", c.secret)
					cluster.SecretEnv = "TDT_IT_SECRET"
				} else {
					cluster.SecretFile = writeSecretFile(t, c.secret+"\n")
				}

				screen, results := openClusterForm(t, cluster)
				screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // Connect, focused already
				var cr ConnectResult
				select {
				case cr = <-results:
				case <-time.After(30 * time.Second):
					t.Fatalf("not connected; screen:\n%s", screenText(screen))
				}

				status, who := securedRequest(t, cr.Client, "GET", "_security/_authenticate", "")
				if status != http.StatusOK || who[c.field] != c.want {
					t.Errorf("expected %s %q, got HTTP %d %v", c.field, c.want, status, who)
				}
			})
		}
	}
}
