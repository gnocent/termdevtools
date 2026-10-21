package ui

import (
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/gnocent/termdevtools/config"
	"github.com/gnocent/termdevtools/i18n"
)

// TestSecretFromAVariableFillsTheForm checks the form of a cluster whose
// entry names a variable, on a standard 80x24 terminal: the message says
// where the secret was read, and Enter alone connects with it — the focus
// is on Connect, the password field holds the variable's value. The entry
// still names the variable afterwards, as it did.
func TestSecretFromAVariableFillsTheForm(t *testing.T) {
	t.Setenv("ES_PW", "x")
	msgs := i18n.For(i18n.FR)
	received := make(chan string, 1)
	_, cfg := connectFilling(t, config.Cluster{AuthType: config.AuthBasic, Username: "elastic", SecretEnv: "ES_PW"}, recordAuthorization(received),
		func(screen tcell.SimulationScreen) {
			screen.SetSize(80, 24)
			waitForScreen(t, screen, fmt.Sprintf(msgs.InfoSecretEnvFmt, "ES_PW"))
			t.Logf("the form, its secret read from a variable:\n%s", screenText(screen))
		})

	if got, want := <-received, "Basic "+base64.StdEncoding.EncodeToString([]byte("elastic:x")); got != want {
		t.Errorf("expected the cluster to be sent %q, got %q", want, got)
	}
	if got := cfg.Clusters[0].SecretEnv; got != "ES_PW" {
		t.Errorf("expected the entry to keep secret_env, got %q", got)
	}
	if saved := savedConfig(t); !strings.Contains(saved, "secret_env: ES_PW") {
		t.Errorf("expected secret_env to be saved, got:\n%s", saved)
	}
}

// TestSecretFromAFileFillsTheForm checks the same from a file, its line end
// left out — and that the secret, used for the connection, is written
// nowhere: not in config.yaml, which keeps only where it is, nor in the
// command F9 copies, which keeps its placeholder.
func TestSecretFromAFileFillsTheForm(t *testing.T) {
	const token = "bm90LWEtcmVhbC10b2tlbg.read-from-a-file"
	home := withHomeDirectory(t)
	if err := os.WriteFile(filepath.Join(home, "es.secret"), []byte(token+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	msgs := i18n.For(i18n.FR)
	received := make(chan string, 1)
	cr, cfg := connectFilling(t, config.Cluster{AuthType: config.AuthBearer, SecretFile: "~/es.secret"}, recordAuthorization(received),
		func(screen tcell.SimulationScreen) {
			waitForScreen(t, screen, fmt.Sprintf(msgs.InfoSecretFileFmt, "~/es.secret"))
		})

	if got, want := <-received, "Bearer "+token; got != want {
		t.Errorf("expected the cluster to be sent %q, got %q", want, got)
	}
	if got := cfg.Clusters[0].SecretFile; got != "~/es.secret" {
		t.Errorf("expected the entry to keep secret_file, got %q", got)
	}
	saved := savedConfig(t)
	if !strings.Contains(saved, "secret_file: ~/es.secret") {
		t.Errorf("expected secret_file to be saved, got:\n%s", saved)
	}
	if strings.Contains(saved, token) {
		t.Errorf("the token was written to config.yaml:\n%s", saved)
	}
	if curl := cr.Client.CurlCommand("GET", "/", nil); strings.Contains(curl, token) || !strings.Contains(curl, "<token>") {
		t.Errorf("expected F9's command to hold the placeholder, not the token: %s", curl)
	}
}

// TestSecretNotFoundLeavesTheFormAsUsual checks every way of not finding
// the secret the entry points to: the message says which, nothing is sent
// on opening the form, whose focus is where it always is — the
// authentication, then Tab to each field in turn — and the password field
// is empty: what Connect sends is what the user types, nothing here.
func TestSecretNotFoundLeavesTheFormAsUsual(t *testing.T) {
	msgs := i18n.For(i18n.FR)
	withHomeDirectory(t)
	os.Unsetenv("TDT_TEST_ABSENT")

	for name, c := range map[string]struct {
		cluster config.Cluster
		message string
	}{
		"variable not set": {config.Cluster{SecretEnv: "TDT_TEST_ABSENT"}, fmt.Sprintf(msgs.ErrSecretEnvFmt, "TDT_TEST_ABSENT")},
		"missing file":     {config.Cluster{SecretFile: "~/absent"}, "Fichier ~/absent illisible"},
		"both keys":        {config.Cluster{SecretEnv: "PATH", SecretFile: "~/absent"}, msgs.ErrSecretBothKeys},
		"relative path":    {config.Cluster{SecretFile: "es.secret"}, fmt.Sprintf(msgs.ErrSecretFileRelativeFmt, "es.secret")},
	} {
		t.Run(name, func(t *testing.T) {
			c.cluster.AuthType, c.cluster.Username = config.AuthBasic, "elastic"
			received := make(chan string, 1)
			connectFilling(t, c.cluster, recordAuthorization(received), func(screen tcell.SimulationScreen) {
				waitForScreen(t, screen, c.message)
				time.Sleep(200 * time.Millisecond)
				if len(received) != 0 {
					t.Fatal("expected nothing to be sent before Connect")
				}
				for range 3 { // the username, the password, Connect
					screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
				}
				waitForDraw(t, screen)
			})

			if got, want := <-received, "Basic "+base64.StdEncoding.EncodeToString([]byte("elastic:")); got != want {
				t.Errorf("expected an empty password, got %q", got)
			}
		})
	}
}

// withHomeDirectory gives the test a home directory of its own, for
// secret_file's "~/", and returns it.
func withHomeDirectory(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)        // os.UserHomeDir on Unix
	t.Setenv("USERPROFILE", home) // and on Windows
	return home
}

// writeSecretFile writes content to a file of its own in a temporary
// directory, readable by its owner only, and returns its path.
func writeSecretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// TestClusterSecretFromTheEnvironment checks secret_env: the variable's value
// as it is, and the variable named either way — found, or why not.
func TestClusterSecretFromTheEnvironment(t *testing.T) {
	msgs := i18n.For(i18n.EN)
	cluster := config.Cluster{AuthType: config.AuthBasic, SecretEnv: "TDT_TEST_SECRET"}

	t.Setenv("TDT_TEST_SECRET", " x y\n")
	secret, message, warn := clusterSecret(cluster, msgs)
	if secret != " x y\n" || warn || message != fmt.Sprintf(msgs.InfoSecretEnvFmt, "TDT_TEST_SECRET") {
		t.Errorf("set: got secret %q, message %q, warning %v", secret, message, warn)
	}

	t.Setenv("TDT_TEST_SECRET", "")
	secret, message, warn = clusterSecret(cluster, msgs)
	if secret != "" || !warn || message != fmt.Sprintf(msgs.ErrSecretEnvFmt, "TDT_TEST_SECRET") {
		t.Errorf("empty: got secret %q, message %q, warning %v", secret, message, warn)
	}

	os.Unsetenv("TDT_TEST_SECRET") // put back by t.Setenv's cleanup
	secret, message, warn = clusterSecret(cluster, msgs)
	if secret != "" || !warn || message != fmt.Sprintf(msgs.ErrSecretEnvFmt, "TDT_TEST_SECRET") {
		t.Errorf("not set: got secret %q, message %q, warning %v", secret, message, warn)
	}
}

// TestClusterSecretFromAFile checks secret_file: the whole file, but for the
// line ends an editor leaves at its end — not the spaces, which may belong to
// a password.
func TestClusterSecretFromAFile(t *testing.T) {
	msgs := i18n.For(i18n.EN)
	for content, want := range map[string]string{
		"tok":                                "tok",
		"tok\n":                              "tok",
		"tok\r\n":                            "tok",
		"tok\n\n":                            "tok",
		" pass word \n":                      " pass word ",
		"\xef\xbb\xbftok\r\n":                "tok", // UTF-8 with a byte order mark
		"line one\nline two\n":               "line one\nline two",
		strings.Repeat("k", 64<<10):          strings.Repeat("k", 64<<10),
		strings.Repeat("k", 64<<10-1) + "\n": strings.Repeat("k", 64<<10-1),
	} {
		path := writeSecretFile(t, content)
		secret, message, warn := clusterSecret(config.Cluster{AuthType: config.AuthBearer, SecretFile: path}, msgs)
		if secret != want || warn || message != fmt.Sprintf(msgs.InfoSecretFileFmt, path) {
			t.Errorf("%.20q: got secret %.20q, message %q, warning %v", content, secret, message, warn)
		}
	}
}

// TestClusterSecretNotFound checks every way of not finding the secret the
// entry points to: no secret, a message that names where it was looked for,
// as a warning — and the secret is typed as usual.
func TestClusterSecretNotFound(t *testing.T) {
	msgs := i18n.For(i18n.FR)
	missing := filepath.Join(t.TempDir(), "absent")
	tooLarge := writeSecretFile(t, strings.Repeat("k", 64<<10+1))
	empty := writeSecretFile(t, "\n")
	// "pw" and a line end as Windows PowerShell 5.1's ">" writes them.
	utf16 := writeSecretFile(t, "\xff\xfep\x00w\x00\r\x00\n\x00")
	directory := t.TempDir()
	device := os.DevNull // the shape a terminal or a pipe would have
	if runtime.GOOS == "windows" {
		device = `\\.\NUL` // os.DevNull, "NUL", isn't an absolute path
	}
	t.Setenv("TDT_TEST_SECRET", "x")

	for name, c := range map[string]struct {
		cluster config.Cluster
		want    string // in the message
	}{
		"both keys":      {config.Cluster{SecretEnv: "TDT_TEST_SECRET", SecretFile: missing}, msgs.ErrSecretBothKeys},
		"missing file":   {config.Cluster{SecretFile: missing}, missing},
		"directory":      {config.Cluster{SecretFile: directory}, fmt.Sprintf(msgs.ErrSecretFileFmt, directory, errNotRegularFile)},
		"device":         {config.Cluster{SecretFile: device}, fmt.Sprintf(msgs.ErrSecretFileFmt, device, errNotRegularFile)},
		"UTF-16":         {config.Cluster{SecretFile: utf16}, fmt.Sprintf(msgs.ErrSecretFileEncodingFmt, utf16)},
		"relative path":  {config.Cluster{SecretFile: "es.secret"}, fmt.Sprintf(msgs.ErrSecretFileRelativeFmt, "es.secret")},
		"another's home": {config.Cluster{SecretFile: "~elastic/es.secret"}, fmt.Sprintf(msgs.ErrSecretFileRelativeFmt, "~elastic/es.secret")},
		"file too large": {config.Cluster{SecretFile: tooLarge}, fmt.Sprintf(msgs.ErrSecretFileSizeFmt, tooLarge)},
		"empty file":     {config.Cluster{SecretFile: empty}, fmt.Sprintf(msgs.ErrSecretFileSizeFmt, empty)},
	} {
		c.cluster.AuthType = config.AuthAPIKey
		secret, message, warn := clusterSecret(c.cluster, msgs)
		if secret != "" || !warn || !strings.Contains(message, c.want) {
			t.Errorf("%s: got secret %q, message %q, warning %v; expected no secret and a warning with %q", name, secret, message, warn, c.want)
		}
	}
}

// TestClusterSecretOnlyForASecretField checks that the secret is looked for
// only where the form has a field for it: not without authentication, nor
// for a client certificate over http, which has no passphrase to ask.
func TestClusterSecretOnlyForASecretField(t *testing.T) {
	msgs := i18n.For(i18n.EN)
	t.Setenv("TDT_TEST_SECRET", "x")
	for _, c := range []struct {
		authType, url string
		wanted        bool
	}{
		{config.AuthNone, "https://es:9200", false},
		{config.AuthMTLS, "http://es:9200", false},
		{config.AuthMTLS, "https://es:9200", true},
		{config.AuthBasic, "http://es:9200", true},
		{config.AuthAPIKey, "http://es:9200", true},
		{config.AuthBearer, "http://es:9200", true},
	} {
		secret, message, _ := clusterSecret(config.Cluster{URL: c.url, AuthType: c.authType, SecretEnv: "TDT_TEST_SECRET"}, msgs)
		if got := secret == "x" && message != ""; got != c.wanted {
			t.Errorf("%s on %s: expected the secret to be read: %v, got secret %q, message %q", c.authType, c.url, c.wanted, secret, message)
		}
	}
}

// TestOthersCanRead checks which access rights earn a secret file a warning:
// any read right of the group or of everyone, on every system but Windows.
func TestOthersCanRead(t *testing.T) {
	for _, c := range []struct {
		goos string
		mode fs.FileMode
		want bool
	}{
		{"linux", 0o600, false},
		{"linux", 0o400, false},
		{"linux", 0o700, false},
		{"linux", 0o644, true},
		{"linux", 0o640, true},
		{"linux", 0o604, true},
		{"darwin", 0o644, true},
		{"darwin", 0o600, false},
		{"windows", 0o666, false},
	} {
		if got := othersCanRead(c.goos, c.mode); got != c.want {
			t.Errorf("%s, %o: got %v, want %v", c.goos, c.mode, got, c.want)
		}
	}
}

// TestSecretFileReadableByOthers checks the warning on a real file: one other
// users can read is used all the same, with a warning; one kept to its owner
// gets none. Unix only: Windows' access rights aren't these bits.
func TestSecretFileReadableByOthers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix access rights")
	}
	msgs := i18n.For(i18n.EN)
	path := writeSecretFile(t, "tok\n")
	cluster := config.Cluster{AuthType: config.AuthBearer, SecretFile: path}

	if secret, message, warn := clusterSecret(cluster, msgs); secret != "tok" || warn || message != fmt.Sprintf(msgs.InfoSecretFileFmt, path) {
		t.Errorf("0600: got secret %q, message %q, warning %v", secret, message, warn)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	if secret, message, warn := clusterSecret(cluster, msgs); secret != "tok" || !warn || message != fmt.Sprintf(msgs.WarnSecretFileReadableFmt, path) {
		t.Errorf("0644: got secret %q, message %q, warning %v", secret, message, warn)
	}
}
