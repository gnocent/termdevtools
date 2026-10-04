package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadCreatesCommentedDefaultConfigOnFirstLaunch checks that, when
// config.yaml doesn't exist yet, Load writes one documenting every setting
// (see writeDefaultConfigFile) — the whole point being that a user can
// discover what's configurable by opening the file.
func TestLoadCreatesCommentedDefaultConfigOnFirstLaunch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultTimeoutSeconds != defaultTimeoutSeconds {
		t.Errorf("expected DefaultTimeoutSeconds=%d, got %d", defaultTimeoutSeconds, cfg.DefaultTimeoutSeconds)
	}
	if cfg.DefaultCADir != defaultCertDir || cfg.DefaultClientCertDir != defaultCertDir {
		t.Errorf("expected both cert dirs to default to %q, got DefaultCADir=%q DefaultClientCertDir=%q", defaultCertDir, cfg.DefaultCADir, cfg.DefaultClientCertDir)
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected config.yaml to have been created on first Load, but: %v", err)
	}
	content := string(data)

	for _, want := range []string{
		"default_timeout_seconds: 120",
		"language: en",
		"mouse: false",
		"default_ca_dir: " + defaultCertDir,
		"default_client_cert_dir: " + defaultCertDir,
		"clusters: []",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected the generated config.yaml to contain %q, got:\n%s", want, content)
		}
	}
	// Every setting must be documented, not just present.
	if strings.Count(content, "#") < 6 {
		t.Errorf("expected an explanatory comment before each setting, got:\n%s", content)
	}
}

// TestLoadPreservesExistingValues checks that Load never touches a setting
// the user already has — only ever appends what's genuinely missing (see
// TestLoadBackfillsMissingSettings).
func TestLoadPreservesExistingValues(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const custom = "default_timeout_seconds: 5\nlanguage: en\n"
	if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultTimeoutSeconds != 5 || cfg.Language != "en" {
		t.Errorf("expected the existing config to be loaded as-is, got timeout=%d language=%q", cfg.DefaultTimeoutSeconds, cfg.Language)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.HasPrefix(string(data), custom) {
		t.Errorf("expected the existing lines to be preserved as-is (only additions after them), got:\n%s", data)
	}
}

// TestLoadBackfillsMissingSettings checks the scenario this feature exists
// for: a config.yaml written before some setting existed (or hand-trimmed
// down to a couple of keys) still ends up documenting every setting a
// current version supports, once Load runs — so the user can discover them
// without reading SPEC.md.
func TestLoadBackfillsMissingSettings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Simulates a config.yaml predating "mouse" and the two directory
	// settings: only default_timeout_seconds and language are present.
	const partial = "default_timeout_seconds: 30\nlanguage: en\n"
	if err := os.WriteFile(path, []byte(partial), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	if !strings.HasPrefix(content, partial) {
		t.Fatalf("expected the original lines to stay first, untouched, got:\n%s", content)
	}
	for _, want := range []string{"mouse: false", "default_ca_dir: " + defaultCertDir, "default_client_cert_dir: " + defaultCertDir, "clusters: []"} {
		if !strings.Contains(content, want) {
			t.Errorf("expected the missing setting %q to be backfilled, got:\n%s", want, content)
		}
	}

	// A second Load (as on the next launch) must be a no-op: every setting
	// is now present, nothing left to backfill.
	if _, err := Load(); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	data2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after second Load: %v", err)
	}
	if string(data2) != content {
		t.Errorf("expected a second Load to be a no-op once every setting is present, but the file changed:\n--- before ---\n%s\n--- after ---\n%s", content, data2)
	}
}

// TestLoadRespectsExplicitEmptyCertDir checks that a user can opt out of
// defaultCertDir by explicitly writing an empty value — unlike
// DefaultTimeoutSeconds (where 0/negative is never valid and gets corrected
// back to the default), an empty path is a legitimate "no default" choice
// and must be left alone.
func TestLoadRespectsExplicitEmptyCertDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const custom = "default_ca_dir: \"\"\ndefault_client_cert_dir: \"\"\n"
	if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultCADir != "" || cfg.DefaultClientCertDir != "" {
		t.Errorf("expected an explicit empty value to be respected, not defaulted, got DefaultCADir=%q DefaultClientCertDir=%q", cfg.DefaultCADir, cfg.DefaultClientCertDir)
	}
}

func TestQueriesPathForURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/home/test/.config")

	path, err := QueriesPathForURL("https://es-prod.example.com:9200")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantDir := filepath.Join("/home/test/.config", appDirName)
	if dir := filepath.Dir(path); dir != wantDir {
		t.Errorf("expected dir %q, got %q", wantDir, dir)
	}

	base := filepath.Base(path)
	if !strings.HasPrefix(base, queriesFilePrefix) || !strings.HasSuffix(base, queriesFileSuffix) {
		t.Errorf("unexpected filename shape: %q", base)
	}
	// No character that's problematic for a filename (":", "/", ...).
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			continue
		}
		t.Errorf("unexpected character %q in filename %q", r, base)
	}
}

func TestQueriesPathForURLDeterministic(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/home/test/.config")

	p1, err := QueriesPathForURL("https://es-prod.example.com:9200")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p2, err := QueriesPathForURL("https://es-prod.example.com:9200")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p1 != p2 {
		t.Errorf("expected same URL to always map to the same path, got %q vs %q", p1, p2)
	}

	other, err := QueriesPathForURL("https://es-staging.example.com:9200")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if other == p1 {
		t.Errorf("expected different URLs to map to different paths, both got %q", p1)
	}
}

// TestVariablesPathForURL mirrors TestQueriesPathForURL: same
// one-file-per-cluster-per-user scheme, same filename sanitization, for the
// reusable ${name} variables store (SPEC.md §7 backlog #4) instead of the
// query save.
func TestVariablesPathForURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/home/test/.config")

	path, err := VariablesPathForURL("https://es-prod.example.com:9200")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantDir := filepath.Join("/home/test/.config", appDirName)
	if dir := filepath.Dir(path); dir != wantDir {
		t.Errorf("expected dir %q, got %q", wantDir, dir)
	}

	base := filepath.Base(path)
	if !strings.HasPrefix(base, variablesFilePrefix) || !strings.HasSuffix(base, variablesFileSuffix) {
		t.Errorf("unexpected filename shape: %q", base)
	}
}

// TestCertDirForOS is a regression test for the actual bug reported: the
// RHEL-specific /etc/pki/tls/certs default_ca_dir/default_client_cert_dir
// default was applied unconditionally, including on Windows and macOS,
// where it points nowhere — pressing Enter to browse (ui/connect.go's
// certificate picker) failed with a raw "directory not found" OS error
// instead of the friendlier "nothing configured" message an empty default
// already produces. Every target platform is checked here directly (not
// just whichever one happens to be running the tests) since certDirForOS
// takes the OS name as a plain argument rather than reading runtime.GOOS
// itself.
func TestCertDirForOS(t *testing.T) {
	cases := []struct {
		goos string
		want string
	}{
		{"linux", "/etc/pki/tls/certs"},
		{"windows", ""},
		{"darwin", ""},
	}
	for _, c := range cases {
		if got := certDirForOS(c.goos); got != c.want {
			t.Errorf("certDirForOS(%q) = %q, want %q", c.goos, got, c.want)
		}
	}
}
