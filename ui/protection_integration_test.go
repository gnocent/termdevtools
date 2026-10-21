//go:build integration

// Integration test: the protection levels against real clusters.
//
//	wsl.exe -- bash tools/testclusters.sh up es-9.5.4 os-3.9.0
//	go test -tags integration -run Integration ./ui/
//
// It creates and deletes an index of its own on the throwaway clusters
// tools/testclusters.sh starts — containers on this machine, holding nothing.
package ui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/gnocent/termdevtools/config"
	"github.com/gnocent/termdevtools/esclient"
	"github.com/gnocent/termdevtools/internal/testclusters"
)

// TestIntegrationProtection deletes a test index on a real Elasticsearch and
// a real OpenSearch through each protection level: refused in read_only,
// where a search still goes through; kept while the confirmation is
// cancelled, deleted once it is confirmed; deleted at once without
// protection.
func TestIntegrationProtection(t *testing.T) {
	clusters, err := testclusters.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	// The most recent cluster of each distribution in the list.
	latest := map[string]testclusters.Cluster{}
	for _, c := range clusters {
		distribution, _, _ := strings.Cut(c.Name, "-")
		latest[distribution] = c
	}
	for _, distribution := range []string{"es", "os"} {
		cluster, ok := latest[distribution]
		if !ok {
			t.Errorf("no %s-* cluster in the list", distribution)
			continue
		}
		t.Run(cluster.Name, func(t *testing.T) { checkProtection(t, cluster) })
	}
}

func checkProtection(t *testing.T, cluster testclusters.Cluster) {
	index := fmt.Sprintf("tdt-it-protection-%d", time.Now().UnixNano())
	deleteIndex := "DELETE /" + index
	createIndex := func() {
		t.Helper()
		if status := clusterStatus(t, cluster, "PUT", "/"+index, `{"settings": {"number_of_replicas": 0}}`); status != http.StatusOK {
			t.Fatalf("creating %s: HTTP %d", index, status)
		}
	}
	indexExists := func() bool {
		t.Helper()
		return clusterStatus(t, cluster, "GET", "/"+index, "") == http.StatusOK
	}
	t.Cleanup(func() { clusterStatus(t, cluster, "DELETE", "/"+index, "") })
	createIndex()

	// A refresh or a flush acts on the cluster whatever the method: the
	// cluster takes it as a GET too, and so must the classification.
	for _, api := range []string{"_refresh", "_flush"} {
		if status := clusterStatus(t, cluster, "GET", "/"+index+"/"+api, ""); status != http.StatusOK {
			t.Errorf("GET %s: expected the cluster to carry it out, got HTTP %d", api, status)
		}
		if esclient.IsRead("GET", index+"/"+api) {
			t.Errorf("GET %s: classified as a read", api)
		}
	}

	app, screen := newTestAppWith(t, testAppOptions{clusterURL: cluster.URL, protection: config.ProtectionReadOnly})
	execute(t, app, screen, deleteIndex)
	if got := statusText(app); !strings.Contains(got, "lecture seule : écriture non envoyée") {
		t.Errorf("read_only: expected the refusal in the status bar, got %q", got)
	}
	if !indexExists() {
		t.Fatal("read_only: the index was deleted")
	}
	execute(t, app, screen, "POST /"+index+"/_search\n{\"size\": 0}")
	eventually(t, "read_only: the search's answer", func() bool {
		return strings.Contains(uiValue(app, app.result.PlainText), `"hits"`)
	})

	app, screen = newTestAppWith(t, testAppOptions{clusterURL: cluster.URL, protection: config.ProtectionConfirm, label: "PROD"})
	execute(t, app, screen, deleteIndex)
	if !confirmVisible(app) {
		t.Fatal("confirm: expected a confirmation")
	}
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if !indexExists() {
		t.Fatal("confirm: the index was deleted although the confirmation was cancelled")
	}
	screen.InjectKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDeletion(t, app, deleteIndex)
	if indexExists() {
		t.Fatal("confirm: the index is still there once the deletion was confirmed")
	}

	createIndex()
	app, screen = newTestAppWith(t, testAppOptions{clusterURL: cluster.URL})
	execute(t, app, screen, deleteIndex)
	if confirmVisible(app) {
		t.Fatal("none: expected no confirmation")
	}
	waitForDeletion(t, app, deleteIndex)
	if indexExists() {
		t.Fatal("none: the index is still there")
	}
}

// waitForDeletion waits for the answer to request in the result panel, and
// checks that it is the cluster acknowledging it.
func waitForDeletion(t *testing.T, app *App, request string) {
	t.Helper()
	eventually(t, request+"'s answer", func() bool {
		return strings.Contains(uiValue(app, app.result.PlainText), "# "+request)
	})
	if got := uiValue(app, app.result.PlainText); !strings.Contains(got, `"acknowledged": true`) {
		t.Errorf("expected the deletion acknowledged, got:\n%s", got)
	}
}

// clusterStatus sends one request to cluster directly, without the
// interface, and returns the status of its answer.
func clusterStatus(t *testing.T, cluster testclusters.Cluster, method, path, body string) int {
	t.Helper()
	client, err := esclient.New(esclient.Params{URL: cluster.URL})
	if err != nil {
		t.Fatalf("esclient.New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := client.Execute(ctx, method, path, []byte(body))
	if err != nil {
		t.Fatalf("%s %s: %v — is the cluster up? (wsl.exe -- bash tools/testclusters.sh up %s)", method, path, err, cluster.Name)
	}
	return result.StatusCode
}
