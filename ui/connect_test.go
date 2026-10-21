package ui

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/gnocent/termdevtools/config"
	"github.com/gnocent/termdevtools/i18n"
	"github.com/gnocent/termdevtools/refdata"
)

// newTestConnectScreen starts the connection screen (pre-connection, see
// BuildConnectPage) on a simulated screen — used to test the certificate
// picker popup (openCertPicker) without a real terminal or cluster.
func newTestConnectScreen(t *testing.T, cfg *config.Config) tcell.SimulationScreen {
	return newTestConnectScreenWithCallback(t, cfg, func(ConnectResult) {})
}

func newTestConnectScreenWithCallback(t *testing.T, cfg *config.Config, onConnected func(ConnectResult)) tcell.SimulationScreen {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(100, 30)

	tapp := tview.NewApplication().SetScreen(screen)
	root := BuildConnectPage(tapp, cfg, onConnected)
	tapp.SetRoot(root, true)

	startTestApplication(t, tapp, screen)

	waitForDraw(t, screen)
	return screen
}

// TestFormStaysInsideItsBorderOn80Columns checks that no field of the
// connection form draws over the form's right border on a standard 80x24
// terminal, in either language: the widest forms — a new connection, whose
// URL is typed, and a cluster in https with a client certificate — included.
func TestFormStaysInsideItsBorderOn80Columns(t *testing.T) {
	for _, lang := range []string{i18n.FR, i18n.EN} {
		for name, clusters := range map[string][]config.Cluster{
			"new connection": nil, // then first in the list
			"https and client certificate": {{
				URL: "https://es.example.test:9200", AuthType: config.AuthMTLS, TLS: config.TLS{Verify: true},
			}},
		} {
			t.Run(lang+", "+name, func(t *testing.T) {
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				cfg := &config.Config{Language: lang, DefaultTimeoutSeconds: 5, Clusters: clusters}
				screen := newTestConnectScreen(t, cfg)
				screen.SetSize(80, 24)
				waitForDraw(t, screen)
				screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // the first entry
				waitForDraw(t, screen)
				checkFormRightBorder(t, screen)
			})
		}
	}
}

// TestNarrowedFieldTakesALongValue checks that a field narrowed to fit an
// 80-column terminal still takes a value longer than itself, whole: a URL
// typed there reaches the cluster in full, the border intact meanwhile.
func TestNarrowedFieldTakesALongValue(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	paths := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(elasticsearchRoot))
	}))
	defer srv.Close()
	prefix := "/" + strings.Repeat("p", 80)

	results := make(chan ConnectResult, 1)
	screen := newTestConnectScreenWithCallback(t, &config.Config{Language: i18n.FR, DefaultTimeoutSeconds: 5},
		func(cr ConnectResult) { results <- cr })
	screen.SetSize(80, 24)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // New connection
	waitForDraw(t, screen)
	injectText(screen, srv.URL+prefix)
	waitForDraw(t, screen)
	checkFormRightBorder(t, screen)

	// In http: the protection, the authentication (the label, disabled
	// without protection, is skipped), then Connect.
	for range 3 {
		screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	}
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)

	select {
	case <-results:
	case <-time.After(5 * time.Second):
		t.Fatalf("no connection; screen:\n%s", screenText(screen))
	}
	if got := <-paths; !strings.HasPrefix(got, prefix) {
		t.Errorf("expected the whole URL to be used, the cluster got the path %q", got)
	}
}

// checkFormRightBorder fails the test if anything covers the right border
// of the connection form, as drawn above the message area.
func checkFormRightBorder(t *testing.T, screen tcell.SimulationScreen) {
	t.Helper()
	cells, width, height := screenContents(screen)
	bottom := height - connectMessageLines - 1 // the form's bottom border
	for y := 1; y < bottom; y++ {
		if got := string(cells[y*width+width-1].Runes); got != "║" {
			t.Errorf("row %d: expected the form's right border, got %q:\n%s", y, got, screenText(screen))
			return
		}
	}
}

func TestNewConnectionCertificateDirectoriesAreOnlyPickerDefaults(t *testing.T) {
	for _, secure := range []bool{false, true} {
		name := "HTTP"
		if secure {
			name = "HTTPS"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(elasticsearchRoot)) })
			var srv *httptest.Server
			if secure {
				srv = httptest.NewTLSServer(handler)
			} else {
				srv = httptest.NewServer(handler)
			}
			t.Cleanup(srv.Close)
			dir := t.TempDir()
			caFile := filepath.Join(dir, "ca.pem")
			if secure {
				if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &config.Config{Language: i18n.EN, DefaultTimeoutSeconds: 5, DefaultCADir: dir, DefaultClientCertDir: dir}
			connected := make(chan ConnectResult, 1)
			screen := newTestConnectScreenWithCallback(t, cfg, func(cr ConnectResult) { connected <- cr })
			screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
			waitForDraw(t, screen)
			injectText(screen, srv.URL)
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to Protection (the label, disabled, is skipped)
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
			if secure {
				// Pick the CA from its configured directory and retain verification.
				screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
				waitForDraw(t, screen)
				screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
				screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
				screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
			}
			screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
			select {
			case cr := <-connected:
				wantCA := ""
				if secure {
					wantCA = caFile
				}
				if cr.Cluster.TLS.CAFile != wantCA || cr.Cluster.TLS.ClientCert != "" || cr.Cluster.TLS.ClientKey != "" || !cr.Cluster.TLS.Verify {
					t.Fatalf("unexpected selected TLS files: %+v", cr.Cluster.TLS)
				}
			case <-time.After(3 * time.Second):
				t.Fatalf("connection failed with configured certificate directories: %s", flatScreen(screen))
			}
		})
	}
}

// TestCertEntriesIn checks that certEntriesIn returns a directory's regular
// files and subdirectories — no dotfiles — subdirectories first then files,
// each group sorted alphabetically: what openCertPicker's file browser
// offers to navigate into or pick from.
func TestCertEntriesIn(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.pem", "a.crt", ".hidden"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	for _, name := range []string{"zsubdir", "asubdir", ".hiddendir"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
	}

	got, err := certEntriesIn(dir)
	if err != nil {
		t.Fatalf("certEntriesIn: %v", err)
	}
	want := []certEntry{
		{name: "asubdir", isDir: true},
		{name: "zsubdir", isDir: true},
		{name: "a.crt", isDir: false},
		{name: "b.pem", isDir: false},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected %v, got %v", want, got)
			break
		}
	}
}

// TestCertPickerFillsFieldFromConfiguredDir exercises the actual feature:
// Enter on the CA field opens a popup listing default_ca_dir's files, and
// picking one closes the popup — confirms the real form field (built by
// buildForm/refreshDynamicFields, via attachCertPicker) actually opens the
// picker on Enter instead of Form's usual "confirm and advance focus"
// behavior. What the field's text ends up as is checked precisely, without
// screen-width truncation getting in the way, by
// TestOpenCertPickerFillsFieldWithFullPath below — a long t.TempDir() path
// wouldn't reliably show its tail in this test's rendered 60-column field.
func TestCertPickerFillsFieldFromConfiguredDir(t *testing.T) {
	certDir := t.TempDir()
	for _, name := range []string{"server-ca.pem", "other-ca.pem"} {
		if err := os.WriteFile(filepath.Join(certDir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	cfg := &config.Config{Language: i18n.FR, DefaultTimeoutSeconds: 5, DefaultCADir: certDir}

	screen := newTestConnectScreen(t, cfg)

	// Cluster list: "+ New connection" is the only real option besides
	// Quit, since cfg.Clusters is empty — highlighted by default.
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)

	// Field order on a fresh form (empty URL => isHTTPS defaults true,
	// AuthNone): URL, Protection, Label (disabled, skipped), Authentication,
	// CA file, Verify checkbox.
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if text := screenText(screen); !strings.Contains(text, "server-ca.pem") || !strings.Contains(text, "other-ca.pem") {
		t.Fatalf("expected the cert picker to list the directory's files, got:\n%s", text)
	}

	// "other-ca.pem" sorts first: Enter on the freshly opened list (first
	// item highlighted by default) picks it.
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if text := screenText(screen); strings.Contains(text, "server-ca.pem") {
		t.Errorf("expected the cert picker popup to have closed after picking a file (the other candidate should no longer be listed), got:\n%s", text)
	}
}

// TestCertPickerReportsUnconfiguredDir checks that Enter on a cert field
// reports a clear error instead of opening an empty popup when the
// corresponding config.yaml directory setting isn't set.
func TestCertPickerReportsUnconfiguredDir(t *testing.T) {
	cfg := &config.Config{Language: i18n.FR, DefaultTimeoutSeconds: 5} // DefaultCADir left empty

	screen := newTestConnectScreen(t, cfg)

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to Protection (the label, disabled, is skipped)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if text := screenText(screen); !strings.Contains(text, "default_ca_dir") {
		t.Errorf("expected an error naming the unconfigured setting (default_ca_dir), got:\n%s", text)
	}
}

// TestCertPickerFallsBackToHomeDirWhenConfiguredDirMissing is a regression
// test for the actual bug reported: a *configured* (non-empty) directory
// that doesn't exist on this machine — e.g. default_ca_dir defaulting to
// /etc/pki/tls/certs (config.certDirForOS) on a platform where that
// RHEL-specific path simply isn't there — used to dead-end on an error
// instead of still opening a browsable popup. The configured directory is a
// convenience shortcut, not a gate: when it's missing, the picker now falls
// back to browsing the user's home directory (HOME/USERPROFILE, overridden
// here for a deterministic, hermetic test) so a certificate kept anywhere
// else can still be found.
func TestCertPickerFallsBackToHomeDirWhenConfiguredDirMissing(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "home-ca.pem"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	cs, field, screen := newTestCertPickerField(t)

	cs.tapp.QueueUpdateDraw(func() { cs.openCertPicker(missing, "default_ca_dir", field) })
	waitForDraw(t, screen)

	if msg := connectValue(cs, func() string { return cs.message.GetText(true) }); msg != "" {
		t.Errorf("expected no error message when falling back to the home directory, got: %q", msg)
	}

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // single candidate, highlighted by default
	waitForDraw(t, screen)

	want := filepath.Join(home, "home-ca.pem")
	if got := connectValue(cs, field.GetText); got != want {
		t.Errorf("expected the field to contain the home-directory file %q, got %q", want, got)
	}
}

// TestCertPickerReportsErrorWhenHomeFallbackAlsoMissing checks that the
// original friendly "directory doesn't exist" message (see
// TestCertPickerFallsBackToHomeDirWhenConfiguredDirMissing) still appears —
// instead of a raw OS error — in the rare case where even the
// home-directory fallback isn't usable.
func TestCertPickerReportsErrorWhenHomeFallbackAlsoMissing(t *testing.T) {
	missingHome := filepath.Join(t.TempDir(), "does-not-exist-home")
	t.Setenv("HOME", missingHome)
	t.Setenv("USERPROFILE", missingHome)

	missingConfigured := filepath.Join(t.TempDir(), "does-not-exist-configured")
	cs, field, screen := newTestCertPickerField(t)

	cs.tapp.QueueUpdateDraw(func() { cs.openCertPicker(missingConfigured, "default_ca_dir", field) })
	waitForDraw(t, screen)

	got := connectValue(cs, func() string { return cs.message.GetText(true) })
	if strings.Contains(got, "cannot find") || strings.Contains(got, "no such file") {
		t.Errorf("expected a clean error message, not a raw OS error, got: %q", got)
	}
	if !strings.Contains(got, "n'existe pas") {
		t.Errorf("expected the \"directory doesn't exist\" message, got: %q", got)
	}
}

// TestCertPickerEscapeCancelsWithoutChange checks that Escape closes the
// popup and leaves the field untouched, same cancel semantics as the
// completion list (SPEC.md §3.2).
func TestCertPickerEscapeCancelsWithoutChange(t *testing.T) {
	certDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(certDir, "ca.pem"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := &config.Config{Language: i18n.FR, DefaultTimeoutSeconds: 5, DefaultCADir: certDir}

	screen := newTestConnectScreen(t, cfg)

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to Protection (the label, disabled, is skipped)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if text := screenText(screen); !strings.Contains(text, "ca.pem") {
		t.Fatalf("test setup sanity check failed: expected the picker to list ca.pem, got:\n%s", text)
	}

	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if text := screenText(screen); strings.Contains(text, "ca.pem") {
		t.Errorf("expected Escape to close the picker without filling the field, got:\n%s", text)
	}
}

// newTestCertPickerField sets up a bare *tview.InputField, on its own
// tview.Pages root, wired to a connectScreen just enough to call
// openCertPicker/closeCertPicker directly — bypassing buildForm entirely so
// the picked file can be read back precisely via field.GetText(), unaffected
// by a long t.TempDir() path scrolling out of a rendered field's visible
// width (see TestCertPickerFillsFieldFromConfiguredDir).
// connectValue returns what f evaluates to on the connection screen's own
// application goroutine — the only place its widgets may be read from.
func connectValue[T any](cs *connectScreen, f func() T) T {
	var value T
	cs.tapp.QueueUpdate(func() { value = f() })
	return value
}

func newTestCertPickerField(t *testing.T) (*connectScreen, *tview.InputField, tcell.SimulationScreen) {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(100, 30)

	tapp := tview.NewApplication().SetScreen(screen)
	field := tview.NewInputField()
	pages := tview.NewPages().AddPage("field", field, true, true)
	tapp.SetRoot(pages, true)

	startTestApplication(t, tapp, screen)
	waitForDraw(t, screen)

	cs := &connectScreen{tapp: tapp, cfg: &config.Config{Language: i18n.FR}, msgs: i18n.For(i18n.FR), pages: pages, message: tview.NewTextView()}
	return cs, field, screen
}

// TestOpenCertPickerFillsFieldWithFullPath checks precisely what
// TestCertPickerFillsFieldFromConfiguredDir can't (screen-width truncation):
// picking a file sets the field's text to the file's full path, joined with
// the configured directory.
func TestOpenCertPickerFillsFieldWithFullPath(t *testing.T) {
	certDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(certDir, "ca.pem"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cs, field, screen := newTestCertPickerField(t)

	cs.tapp.QueueUpdateDraw(func() { cs.openCertPicker(certDir, "default_ca_dir", field) })
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // single candidate, highlighted by default
	waitForDraw(t, screen)

	want := filepath.Join(certDir, "ca.pem")
	if got := connectValue(cs, field.GetText); got != want {
		t.Errorf("expected the field to contain %q, got %q", want, got)
	}
	if cs.tapp.GetFocus() != field {
		t.Error("expected focus to return to the field after picking a file")
	}
}

// TestCertPickerNavigatesIntoSubdirectoryAndBack checks the file-browser
// behavior added on top of the flat single-directory list: Enter on a
// subdirectory entry (listed first, suffixed with the OS path separator —
// see certEntriesIn) browses into it instead of picking it as a file, and
// Backspace goes back up to the parent directory.
func TestCertPickerNavigatesIntoSubdirectoryAndBack(t *testing.T) {
	topDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(topDir, "top-ca.pem"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	subDir := filepath.Join(topDir, "sub")
	if err := os.Mkdir(subDir, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "sub-ca.pem"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cs, field, screen := newTestCertPickerField(t)

	cs.tapp.QueueUpdateDraw(func() { cs.openCertPicker(topDir, "default_ca_dir", field) })
	waitForDraw(t, screen)
	if text := screenText(screen); !strings.Contains(text, "sub"+string(filepath.Separator)) || !strings.Contains(text, "top-ca.pem") {
		t.Fatalf("expected the top-level listing (subdirectory + file), got:\n%s", text)
	}

	// "sub/" sorts first (subdirectories before files — certEntriesIn):
	// Enter on the highlighted first item browses into it instead of
	// picking it.
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if text := screenText(screen); !strings.Contains(text, "sub-ca.pem") || strings.Contains(text, "top-ca.pem") {
		t.Fatalf("expected to be browsing sub/ now (only sub-ca.pem listed), got:\n%s", text)
	}
	if got := connectValue(cs, field.GetText); got != "" {
		t.Errorf("expected the field to stay empty while merely browsing, got %q", got)
	}

	// Backspace goes back up: top-level listing again.
	screen.InjectKey(tcell.KeyBackspace, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if text := screenText(screen); !strings.Contains(text, "top-ca.pem") {
		t.Fatalf("expected Backspace to return to the parent listing, got:\n%s", text)
	}

	// Back into sub/, then pick its file: the full path should be joined
	// off the subdirectory actually browsed, not the original top-level one.
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // single candidate now: sub-ca.pem
	waitForDraw(t, screen)

	want := filepath.Join(subDir, "sub-ca.pem")
	if got := connectValue(cs, field.GetText); got != want {
		t.Errorf("expected the field to contain %q, got %q", want, got)
	}
}

// connectToTestServer drives the real connection flow against handler: the
// (single) known cluster is picked from the list, its form confirmed, and
// the ConnectResult handed to the main application is returned.
func connectToTestServer(t *testing.T, cluster config.Cluster, handler http.HandlerFunc) (ConnectResult, *config.Config) {
	t.Helper()
	cluster.AuthType = config.AuthNone
	// An http:// cluster with no authentication: the form holds the
	// authentication dropdown (focused), then the Connect button.
	return connectFilling(t, cluster, handler, func(screen tcell.SimulationScreen) {
		screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
		waitForDraw(t, screen)
	})
}

// connectFilling is connectToTestServer for a cluster whose form has fields
// to fill in: fill is handed the screen with the form open — the focus on
// its authentication dropdown — and must leave it on the Connect button.
func connectFilling(t *testing.T, cluster config.Cluster, handler http.HandlerFunc, fill func(tcell.SimulationScreen)) (ConnectResult, *config.Config) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cluster.URL = srv.URL
	cfg := &config.Config{Language: i18n.FR, DefaultTimeoutSeconds: 5, Clusters: []config.Cluster{cluster}}

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(100, 30)

	results := make(chan ConnectResult, 1)
	tapp := tview.NewApplication().SetScreen(screen)
	tapp.SetRoot(BuildConnectPage(tapp, cfg, func(cr ConnectResult) { results <- cr }), true)
	startTestApplication(t, tapp, screen)
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // the cluster, first in the list
	waitForDraw(t, screen)
	fill(screen)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)

	select {
	case cr := <-results:
		return cr, cfg
	case <-time.After(5 * time.Second):
		t.Fatalf("no connection result; screen:\n%s", screenText(screen))
		return ConnectResult{}, nil
	}
}

func rootHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// TestConnectDetectsTarget checks that a successful connection reports the
// cluster's distribution and version, read from the same "GET /" that
// validates the connection.
func TestConnectDetectsTarget(t *testing.T) {
	cr, _ := connectToTestServer(t, config.Cluster{},
		rootHandler(`{"version": {"distribution": "opensearch", "number": "2.19.6"}, "tagline": "The OpenSearch Project: https://opensearch.org/"}`))

	want := refdata.Target{Distribution: refdata.OpenSearch, Version: refdata.Version{Major: 2, Minor: 19, Patch: 6}, HasVersion: true}
	if cr.Target != want {
		t.Errorf("got target %+v, want %+v", cr.Target, want)
	}
	if cr.Warning != "" {
		t.Errorf("unexpected warning: %q", cr.Warning)
	}
}

// TestConnectAppliesAndKeepsOverride checks config.yaml's per-cluster
// distribution/version override: it wins over detection, and — not being a
// form field — survives the rewrite of the cluster's entry that every
// successful connection performs.
func TestConnectAppliesAndKeepsOverride(t *testing.T) {
	cr, cfg := connectToTestServer(t, config.Cluster{Distribution: "opensearch", Version: "2.11"},
		rootHandler(`{"version": {"number": "7.10.2"}, "tagline": "You Know, for Search"}`))

	want := refdata.Target{Distribution: refdata.OpenSearch, Version: refdata.Version{Major: 2, Minor: 11}, HasVersion: true}
	if cr.Target != want {
		t.Errorf("got target %+v, want %+v", cr.Target, want)
	}
	if got := cfg.Clusters[0]; got.Distribution != "opensearch" || got.Version != "2.11" {
		t.Errorf("override lost from the saved cluster entry: %+v", got)
	}
}

// TestConnectIgnoresInvalidOverride checks that a typo in the override
// doesn't lock the user out: the connection succeeds on the detected target,
// with a warning.
func TestConnectIgnoresInvalidOverride(t *testing.T) {
	cr, _ := connectToTestServer(t, config.Cluster{Distribution: "elastik"},
		rootHandler(`{"version": {"number": "9.5.4", "build_flavor": "default"}, "tagline": "You Know, for Search"}`))

	want := refdata.Target{Distribution: refdata.Elasticsearch, Version: refdata.Version{Major: 9, Minor: 5, Patch: 4}, HasVersion: true}
	if cr.Target != want {
		t.Errorf("got target %+v, want %+v", cr.Target, want)
	}
	if !strings.Contains(cr.Warning, "elastik") {
		t.Errorf("expected a warning naming the rejected value, got %q", cr.Warning)
	}
}

// TestConnectUnrecognizedRootStillConnects checks that a cluster whose
// "GET /" body can't be interpreted (a proxy in front of it, another fork)
// connects all the same, with nothing detected.
func TestConnectUnrecognizedRootStillConnects(t *testing.T) {
	cr, _ := connectToTestServer(t, config.Cluster{}, rootHandler(`<html>ok</html>`))
	if cr.Target != (refdata.Target{}) {
		t.Errorf("got target %+v, want none", cr.Target)
	}
}
