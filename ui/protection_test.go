package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/gnocent/termdevtools/config"
)

// recordingServer is a test cluster that answers every request with {} and
// records each one as "METHOD /path".
type recordingServer struct {
	mu       sync.Mutex
	requests []string
}

func newRecordingServer(t *testing.T) (*recordingServer, string) {
	t.Helper()
	s := &recordingServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return s, srv.URL
}

func (s *recordingServer) received() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

// execute puts request alone in the editor and presses Ctrl+E.
func execute(t *testing.T, app *App, screen tcell.SimulationScreen, request string) {
	t.Helper()
	onUI(app, func() {
		app.editor.view.SetText(request, true)
	})
	screen.InjectKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)
	waitForDraw(t, screen)
}

func confirmVisible(app *App) bool {
	return uiValue(app, func() bool { return app.confirmVisible })
}

// TestReadOnlyRefusesWrites checks that a cluster in read_only protection
// gets no write at all — refused with a word in the status bar, the result
// panel left alone — while its reads go through.
func TestReadOnlyRefusesWrites(t *testing.T) {
	server, url := newRecordingServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: url, protection: config.ProtectionReadOnly})

	for _, write := range []string{"DELETE /my-index", "PUT /my-index/_settings\n{\"number_of_replicas\": 0}", "POST /_bulk\n{}"} {
		execute(t, app, screen, write)
		if confirmVisible(app) {
			t.Fatalf("%q: expected a refusal, not a confirmation", write)
		}
		if got := statusText(app); !strings.Contains(got, "lecture seule : écriture non envoyée") {
			t.Errorf("%q: expected the refusal in the status bar, got %q", write, got)
		}
	}
	if got := server.received(); len(got) != 0 {
		t.Fatalf("expected no request to reach the cluster, got %v", got)
	}
	if got := uiValue(app, app.result.PlainText); got != "" {
		t.Errorf("expected the result panel left alone, got %q", got)
	}

	execute(t, app, screen, "POST /my-index/_search\n{\"size\": 0}")
	eventually(t, "the read's result", func() bool {
		return strings.Contains(uiValue(app, app.result.PlainText), "# POST /my-index/_search")
	})
	if got := server.received(); len(got) != 1 || got[0] != "POST /my-index/_search" {
		t.Errorf("expected the read alone to reach the cluster, got %v", got)
	}
}

// TestConfirmWrite checks the confirmation a write gets on a cluster in
// confirm protection: nothing sent while it is shown, Enter pressed at once
// and Escape both cancel, Ctrl+E pressed again does nothing more, and Send
// sends the request.
func TestConfirmWrite(t *testing.T) {
	server, url := newRecordingServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: url, protection: config.ProtectionConfirm, label: "PROD"})

	execute(t, app, screen, "DELETE /my-index")
	if !confirmVisible(app) {
		t.Fatal("expected a confirmation before sending a write")
	}
	text := screenText(screen)
	t.Logf("screen with the confirmation:\n%s", text)
	for _, want := range []string{"Cette requête écrit sur PROD — " + url, "DELETE /my-index", "Annuler", "Envoyer"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in the confirmation, got:\n%s", want, text)
		}
	}
	screen.InjectKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if confirmVisible(app) {
		t.Fatal("expected Enter to close the confirmation")
	}
	if got := statusText(app); !strings.Contains(got, "envoi annulé") {
		t.Errorf("expected the cancellation in the status bar, got %q", got)
	}
	if app.tapp.GetFocus() != app.editor.Primitive() {
		t.Error("expected the focus back on the editor")
	}

	screen.InjectKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)
	waitForDraw(t, screen)
	if !confirmVisible(app) {
		t.Fatal("expected a confirmation again")
	}
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if confirmVisible(app) {
		t.Fatal("expected Escape to close the confirmation")
	}
	if got := server.received(); len(got) != 0 {
		t.Fatalf("expected nothing sent until confirmed, got %v", got)
	}

	screen.InjectKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if confirmVisible(app) {
		t.Fatal("expected Send to close the confirmation")
	}
	eventually(t, "the write's result", func() bool {
		return strings.Contains(uiValue(app, app.result.PlainText), "# DELETE /my-index")
	})
	if got := server.received(); len(got) != 1 || got[0] != "DELETE /my-index" {
		t.Errorf("expected the write sent once, got %v", got)
	}
}

// TestConfirmWriteArrowKeys checks that the arrows move between the buttons
// too: left back to Cancel, right to Send.
func TestConfirmWriteArrowKeys(t *testing.T) {
	server, url := newRecordingServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: url, protection: config.ProtectionConfirm})

	execute(t, app, screen, "DELETE /my-index")
	screen.InjectKey(tcell.KeyRight, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyLeft, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if confirmVisible(app) {
		t.Fatal("expected Cancel to close the confirmation")
	}
	if got := server.received(); len(got) != 0 {
		t.Fatalf("expected nothing sent after going back to Cancel, got %v", got)
	}

	// No going round: Left on Cancel stays there.
	screen.InjectKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyLeft, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if got := server.received(); len(got) != 0 {
		t.Fatalf("expected nothing sent after Left on Cancel, got %v", got)
	}

	screen.InjectKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyRight, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyRight, 0, tcell.ModNone) // stays on Send
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	eventually(t, "the write sent", func() bool { return len(server.received()) == 1 })
}

// TestConfirmWriteTextTakesNoClick checks that a click on the
// confirmation's text, with the mouse on, doesn't take the keys away from
// its buttons: Escape still cancels.
func TestConfirmWriteTextTakesNoClick(t *testing.T) {
	server, url := newRecordingServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: url, protection: config.ProtectionConfirm, mouse: true})
	execute(t, app, screen, "DELETE /my-index")
	_, _, height := screenContents(screen)
	y := -1
	for row := 0; row < height; row++ {
		if strings.Contains(screenRow(screen, row), "Cette requête écrit sur") {
			y = row
		}
	}
	if y < 0 {
		t.Fatalf("expected the confirmation's text on screen, got:\n%s", screenText(screen))
	}
	screen.InjectMouse(20, y, tcell.Button1, tcell.ModNone)
	screen.InjectMouse(20, y, tcell.ButtonNone, tcell.ModNone)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if confirmVisible(app) {
		t.Fatal("expected Escape to close the confirmation after a click on its text")
	}
	if got := server.received(); len(got) != 0 {
		t.Errorf("expected nothing sent, got %v", got)
	}
}

// TestConfirmWriteHidesWhatIsBehind checks that nothing of the panels
// behind the confirmation shows inside its frame. Its margins, a column on
// either side, let the editor's text through when the frame drew them.
func TestConfirmWriteHidesWhatIsBehind(t *testing.T) {
	_, url := newRecordingServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: url, protection: config.ProtectionConfirm})
	behind := strings.Repeat(strings.Repeat("X", 30)+"\n", 20)
	execute(t, app, screen, behind+"DELETE /my-index")
	if !confirmVisible(app) {
		t.Fatalf("expected the confirmation, got:\n%s", screenText(screen))
	}

	_, _, height := screenContents(screen)
	title := strings.TrimSpace(app.msgs.ConfirmWriteTitle)
	left, right := -1, -1
	for row := 0; row < height; row++ {
		line := []rune(screenRow(screen, row))
		if left < 0 {
			if !strings.Contains(string(line), title) {
				continue
			}
			for x, r := range line {
				switch r {
				case '┏', '┌':
					left = x
				case '┓', '┐':
					right = x
				}
			}
			continue
		}
		if line[left] == '┗' || line[left] == '└' {
			return
		}
		if inside := string(line[left+1 : right]); strings.Contains(inside, "X") {
			t.Errorf("expected nothing of the editor inside the confirmation, row %d holds %q", row, inside)
		}
	}
	t.Fatalf("expected the confirmation's frame on screen, got:\n%s", screenText(screen))
}

// TestConfirmPath checks how a long path is shown in the confirmation: its
// API always, and the first and last names of a long list of indices.
func TestConfirmPath(t *testing.T) {
	var indices []string
	for day := 1; day <= 15; day++ {
		indices = append(indices, fmt.Sprintf("logs-2026.10.%02d", day))
	}
	long := "/" + strings.Join(indices, ",") + "/_delete_by_query"
	cases := []struct {
		path      string
		want      []string
		unchanged bool
	}{
		{"/my-index/_delete_by_query?conflicts=proceed", nil, true},
		{long, []string{"/logs-2026.10.01,", "…", ",logs-2026.10.15/_delete_by_query"}, false},
		{long + "?conflicts=proceed&slices=auto", []string{"/logs-2026.10.01,", "…", "logs-2026.10.15/_delete_by_query?…"}, false},
		{"/" + strings.Join(indices, ","), []string{"/logs-2026.10.01,", "…", ",logs-2026.10.15"}, false},
		{"/my-index/_delete_by_query?q=" + strings.Repeat("x", 150), []string{"/my-index/_delete_by_query?q=xxx", "…"}, false},
	}
	for _, c := range cases {
		got := confirmPath(c.path)
		if c.unchanged && got != c.path {
			t.Errorf("%q: expected it unchanged, got %q", c.path, got)
		}
		if n := len([]rune(got)); n > confirmPathMax {
			t.Errorf("%q: expected at most %d runes, got %d: %q", c.path, confirmPathMax, n, got)
		}
		for _, want := range c.want {
			if !strings.Contains(got, want) {
				t.Errorf("%q: expected %q in %q", c.path, want, got)
			}
		}
	}
}

// TestProtectionMessagesOn80Columns checks that the refusal and the
// cancellation can be read on an 80-column terminal, a cluster's URL and
// the user taking most of the status line: they are all there is to tell
// that nothing was sent.
func TestProtectionMessagesOn80Columns(t *testing.T) {
	const url = "https://es-audit.example.test:9200"
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: url, protection: config.ProtectionReadOnly})
	execute(t, app, screen, "DELETE /my-index")
	if text := screenText(screen); !strings.Contains(text, "lecture seule : écriture non envoyée (DELETE /my-index)") {
		t.Errorf("expected the refusal on screen, got:\n%s", text)
	}

	app, screen = newTestAppWith(t, testAppOptions{clusterURL: url, protection: config.ProtectionConfirm})
	execute(t, app, screen, "DELETE /my-index")
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if text := screenText(screen); !strings.Contains(text, "envoi annulé : rien n'a été envoyé") {
		t.Errorf("expected the cancellation on screen, got:\n%s", text)
	}
}

// TestQuitWhileConfirming checks that Ctrl+C still quits while a write waits
// for confirmation — sending nothing.
func TestQuitWhileConfirming(t *testing.T) {
	server, url := newRecordingServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: url, protection: config.ProtectionConfirm})
	execute(t, app, screen, "DELETE /my-index")
	if !confirmVisible(app) {
		t.Fatal("expected a confirmation")
	}
	screen.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
	eventually(t, "Ctrl+C to quit", func() bool { return !appRunning(app) })
	if got := server.received(); len(got) != 0 {
		t.Errorf("expected nothing sent, got %v", got)
	}
}

// TestConfirmWriteSendsWhatWasResolved checks that what is sent once
// confirmed is the request as Ctrl+E resolved it — variables included —
// even if the variables change while the confirmation is shown (F7 can't be
// pressed then, but a reload is the kind of thing that could happen).
func TestConfirmWriteSendsWhatWasResolved(t *testing.T) {
	server, url := newRecordingServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: url, protection: config.ProtectionConfirm})
	onUI(app, func() { app.variables = map[string]string{"target": "logs-1"} })

	execute(t, app, screen, "POST /${target}/_delete_by_query\n{\"query\": {\"match_all\": {}}}")
	if !confirmVisible(app) {
		t.Fatal("expected a confirmation, the variable holding only the index")
	}
	if text := screenText(screen); !strings.Contains(text, "POST /logs-1/_delete_by_query") || !strings.Contains(text, "écrit sur "+url) {
		t.Errorf("expected the resolved request and the cluster's URL in the confirmation, got:\n%s", text)
	}
	onUI(app, func() { app.variables = map[string]string{"target": "logs-2"} })
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	eventually(t, "the write sent", func() bool { return len(server.received()) == 1 })
	if got := server.received(); got[0] != "POST /logs-1/_delete_by_query" {
		t.Errorf("expected the request resolved on Ctrl+E, got %v", got)
	}
}

// TestConfirmWriteClassifiesTheResolvedPath checks that a variable holding
// the API itself doesn't get a write past the protection.
func TestConfirmWriteClassifiesTheResolvedPath(t *testing.T) {
	server, url := newRecordingServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: url, protection: config.ProtectionReadOnly})
	onUI(app, func() { app.variables = map[string]string{"api": "_delete_by_query"} })

	execute(t, app, screen, "POST /my-index/${api}\n{\"query\": {\"match_all\": {}}}")
	if got := statusText(app); !strings.Contains(got, "POST /my-index/_delete_by_query") {
		t.Errorf("expected the resolved request refused, got %q", got)
	}
	if got := server.received(); len(got) != 0 {
		t.Errorf("expected nothing sent, got %v", got)
	}
}

// TestProtectedReadsAndUnprotectedWrites checks the requests that are sent
// at once: a read whatever the protection, a write to an unprotected
// cluster.
func TestProtectedReadsAndUnprotectedWrites(t *testing.T) {
	cases := []struct {
		protection, request, want string
	}{
		{config.ProtectionConfirm, "GET /_cluster/health", "GET /_cluster/health"},
		{config.ProtectionConfirm, "POST /my-index/_count\n{}", "POST /my-index/_count"},
		{config.ProtectionReadOnly, "GET /my-index/_doc/42", "GET /my-index/_doc/42"},
		{"", "DELETE /my-index", "DELETE /my-index"},
		{config.ProtectionNone, "POST /_bulk\n{}", "POST /_bulk"},
	}
	for _, c := range cases {
		server, url := newRecordingServer(t)
		app, screen := newTestAppWith(t, testAppOptions{clusterURL: url, protection: c.protection})
		execute(t, app, screen, c.request)
		if confirmVisible(app) {
			t.Errorf("%s/%q: expected no confirmation", c.protection, c.request)
		}
		eventually(t, c.want+" sent", func() bool { return len(server.received()) == 1 })
		if got := server.received(); got[0] != c.want {
			t.Errorf("%s/%q: expected %q sent, got %v", c.protection, c.request, c.want, got)
		}
	}
}

// TestConnectSavesProtection checks the protection chosen in the connection
// form: from the authentication the form opens on, back to Protection (the
// label, disabled, skipped), "lecture seule" chosen, then the label — enabled
// by that choice — typed. Both reach the main screen and config.yaml.
func TestConnectSavesProtection(t *testing.T) {
	cr, cfg := connectFilling(t, config.Cluster{AuthType: config.AuthNone}, rootHandler(elasticsearchRoot),
		func(screen tcell.SimulationScreen) {
			screen.InjectKey(tcell.KeyBacktab, 0, tcell.ModNone) // to Protection
			screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)   // opens its list
			screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
			screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
			screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // "lecture seule"
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)   // to the label
			injectText(screen, "PROD")
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to the authentication
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to Connect
			waitForDraw(t, screen)
		})

	if cr.Cluster.Protection != config.ProtectionReadOnly || cr.Cluster.Label != "PROD" {
		t.Errorf("expected read_only and PROD handed to the main screen, got %q and %q", cr.Cluster.Protection, cr.Cluster.Label)
	}
	if got := cfg.Clusters[0]; got.Protection != config.ProtectionReadOnly || got.Label != "PROD" {
		t.Errorf("expected read_only and PROD on the cluster's entry, got %+v", got)
	}
	saved := savedConfig(t)
	for _, want := range []string{"protection: read_only", "label: PROD"} {
		if !strings.Contains(saved, want) {
			t.Errorf("expected %q in config.yaml, got:\n%s", want, saved)
		}
	}
}

// TestConnectWithoutProtectionKeepsTheLabel checks that turning protection
// off disables the label without losing it, and writes no protection key.
func TestConnectWithoutProtectionKeepsTheLabel(t *testing.T) {
	cr, cfg := connectFilling(t, config.Cluster{AuthType: config.AuthNone, Protection: config.ProtectionConfirm, Label: "PROD"},
		rootHandler(elasticsearchRoot),
		func(screen tcell.SimulationScreen) {
			screen.InjectKey(tcell.KeyBacktab, 0, tcell.ModNone) // to the label, enabled
			screen.InjectKey(tcell.KeyBacktab, 0, tcell.ModNone) // to Protection
			screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
			screen.InjectKey(tcell.KeyUp, 0, tcell.ModNone)
			screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // "aucune"
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)   // to the authentication: the label is now skipped
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)   // to Connect
			waitForDraw(t, screen)
		})

	if cr.Cluster.Protection != "" || cfg.Clusters[0].Protection != "" {
		t.Errorf("expected no protection, got %q (handed over) and %q (saved)", cr.Cluster.Protection, cfg.Clusters[0].Protection)
	}
	if cfg.Clusters[0].Label != "PROD" {
		t.Errorf("expected the label kept, got %q", cfg.Clusters[0].Label)
	}
	saved := savedConfig(t)
	for _, line := range strings.Split(saved, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "protection:") {
			t.Errorf("expected no protection key in config.yaml, got:\n%s", saved)
		}
	}
}

// TestDisabledLabelIsGreyedOut checks that the label, disabled while there
// is no protection, still shows as a field — greyed out, box included —
// rather than as its caption alone, and comes back once a protection is
// chosen.
func TestDisabledLabelIsGreyedOut(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	screen := newTestConnectScreen(t, &config.Config{Language: "fr", DefaultTimeoutSeconds: 5})
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // + Nouvelle connexion
	waitForDraw(t, screen)

	labelRow := func() (row, caption int) {
		_, _, height := screenContents(screen)
		for y := 0; y < height; y++ {
			if x := strings.Index(screenRow(screen, y), "Libellé (bandeau)"); x >= 0 {
				return y, len([]rune(screenRow(screen, y)[:x]))
			}
		}
		t.Fatalf("no label field on screen:\n%s", screenText(screen))
		return 0, 0
	}
	grey := func() (captionGrey bool, greyCells int) {
		y, x := labelRow()
		cells, width, _ := screenContents(screen)
		foreground, _, _ := cells[y*width+x].Style.Decompose()
		for column := 0; column < width; column++ {
			if screenBackground(screen, column, y) == tcell.ColorGray {
				greyCells++
			}
		}
		return foreground == tcell.ColorGray, greyCells
	}

	if caption, cells := grey(); !caption || cells != labelMaxLength+1 {
		t.Errorf("without protection: expected a grey caption and a grey box of %d cells, got %v and %d cells", labelMaxLength+1, caption, cells)
	}
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)   // to Protection
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // opens its list
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // confirmation des écritures
	waitForDraw(t, screen)
	if caption, cells := grey(); caption || cells != 0 {
		t.Errorf("with protection: expected the label as any field, got a grey caption %v and %d grey cells", caption, cells)
	}
}

// TestUnknownProtectionIsReadOnly checks a level config.yaml holds but no
// version knows (a typo made by hand): the form says so, offers read-only
// instead, and saves it on connecting.
func TestUnknownProtectionIsReadOnly(t *testing.T) {
	var screenOnOpening string
	cr, cfg := connectFilling(t, config.Cluster{AuthType: config.AuthNone, Protection: "readonly"}, rootHandler(elasticsearchRoot),
		func(screen tcell.SimulationScreen) {
			screenOnOpening = screenText(screen)
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to Connect
			waitForDraw(t, screen)
		})

	for _, want := range []string{"protection « readonly » inconnue", "lecture seule"} {
		if !strings.Contains(screenOnOpening, want) {
			t.Errorf("expected %q in the form, got:\n%s", want, screenOnOpening)
		}
	}
	if cr.Cluster.Protection != config.ProtectionReadOnly || cfg.Clusters[0].Protection != config.ProtectionReadOnly {
		t.Errorf("expected read_only, got %q (handed over) and %q (saved)", cr.Cluster.Protection, cfg.Clusters[0].Protection)
	}
}

// TestClusterListShowsProtection checks that the history names each
// protected cluster's protection, the way its banner will.
func TestClusterListShowsProtection(t *testing.T) {
	cfg := &config.Config{Language: "fr", DefaultTimeoutSeconds: 5, Clusters: []config.Cluster{
		{URL: "https://es-prod.example.test:9200", AuthType: config.AuthBasic, Protection: config.ProtectionConfirm, Label: "PROD"},
		{URL: "https://es-audit.example.test:9200", AuthType: config.AuthNone, Protection: config.ProtectionReadOnly},
		{URL: "https://es-dev.example.test:9200", AuthType: config.AuthNone, Label: "DEV"},
	}}
	text := screenText(newTestConnectScreen(t, cfg))
	for _, want := range []string{"auth: basic · PROD · confirmation des écritures", "auth: none · LECTURE SEULE"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in the history, got:\n%s", want, text)
		}
	}
	if strings.Contains(text, "DEV") {
		t.Errorf("expected an unprotected cluster's label left out, got:\n%s", text)
	}
}

// TestConfirmWriteShowsTextAsTyped checks that brackets in the label or the
// path are shown as such, not taken for color tags.
func TestConfirmWriteShowsTextAsTyped(t *testing.T) {
	_, url := newRecordingServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: url, protection: config.ProtectionConfirm, label: "[red]PROD"})
	execute(t, app, screen, "DELETE /my-index?q=[x]")
	if !confirmVisible(app) {
		t.Fatal("expected a confirmation")
	}
	if text := screenText(screen); !strings.Contains(text, "[red]PROD — ") || !strings.Contains(text, "DELETE /my-index?q=[x]") {
		t.Errorf("expected the label and path shown as typed, got:\n%s", text)
	}
}

// screenRow returns the text of row y of the simulated screen.
func screenRow(screen tcell.SimulationScreen, y int) string {
	cells, w, _ := screenContents(screen)
	var b strings.Builder
	for x := 0; x < w; x++ {
		b.WriteString(string(cells[y*w+x].Runes))
	}
	return b.String()
}

// screenBackground returns the background color of the cell at x, y.
func screenBackground(screen tcell.SimulationScreen, x, y int) tcell.Color {
	cells, w, _ := screenContents(screen)
	_, background, _ := cells[y*w+x].Style.Decompose()
	return background
}

// TestNoBannerWithoutProtection checks that the header of an unprotected
// cluster says nothing of a protection, its label included.
func TestNoBannerWithoutProtection(t *testing.T) {
	for _, protection := range []string{"", config.ProtectionNone} {
		_, screen := newTestAppWith(t, testAppOptions{protection: protection, label: "PROD"})
		if row := screenRow(screen, 0); !strings.Contains(row, "TermDevTools") || strings.Contains(row, "PROD") {
			t.Errorf("protection %q: expected the plain header on the first row, got %q", protection, row)
		}
		if got := screenBackground(screen, 79, 0); got != themeSurface {
			t.Errorf("protection %q: expected the plain header's background, got %v", protection, got)
		}
	}
}

// TestProtectionBanner checks what a protected cluster's header says across
// the top of the main screen: its text, with and without a label, the color
// of the whole line, and the panels right under it, on the same row as on an
// unprotected cluster.
func TestProtectionBanner(t *testing.T) {
	cases := []struct {
		protection, label string
		want              string
		background        tcell.Color
	}{
		{config.ProtectionConfirm, "PROD", "PROD · confirmation des écritures", themeDanger},
		{config.ProtectionConfirm, "", "CONFIRMATION DES ÉCRITURES", themeDanger},
		{config.ProtectionReadOnly, "PROD", "PROD · lecture seule", themeYellow},
		{config.ProtectionReadOnly, "", "LECTURE SEULE", themeYellow},
		// The form hands over a level it has already normalized; a label
		// written by hand may still span lines or hold control characters.
		{config.ProtectionReadOnly, "PROD\n  eu-west\x1b[31m", "PROD eu-west [31m · lecture seule", themeYellow},
	}
	for _, c := range cases {
		_, screen := newTestAppWith(t, testAppOptions{protection: c.protection, label: c.label})
		row := screenRow(screen, 0)
		if !strings.Contains(row, c.want) {
			t.Errorf("%s/%q: expected %q on the first row, got %q", c.protection, c.label, c.want, row)
		}
		for _, x := range []int{0, 1, 78, 79} {
			if got := screenBackground(screen, x, 0); got != c.background {
				t.Errorf("%s/%q: expected background %v at column %d, got %v", c.protection, c.label, c.background, x, got)
			}
		}
		if next := screenRow(screen, 1); !strings.Contains(next, "Requêtes") {
			t.Errorf("%s/%q: expected the panels right under the banner, got %q", c.protection, c.label, next)
		}
	}
}

// TestProtectionBannerFollowsLanguage checks that the banner switches
// language with the rest of the interface (F3), label untouched.
func TestProtectionBannerFollowsLanguage(t *testing.T) {
	_, screen := newTestAppWith(t, testAppOptions{protection: config.ProtectionReadOnly, label: "PROD"})
	screen.InjectKey(tcell.KeyF3, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if row := screenRow(screen, 0); !strings.Contains(row, "PROD · read-only") {
		t.Errorf("expected the banner in English after F3, got %q", row)
	}
}
