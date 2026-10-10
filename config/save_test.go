package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withDefaultCertDir gives defaultCertDir a non-empty value for the test,
// whatever the platform it runs on (it is only non-empty on Linux).
func withDefaultCertDir(t *testing.T, dir string) {
	t.Helper()
	previous := defaultCertDir
	defaultCertDir = dir
	t.Cleanup(func() { defaultCertDir = previous })
}

func readConfigFile(t *testing.T) string {
	t.Helper()
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(data)
}

func writeConfigFile(t *testing.T, content string) {
	t.Helper()
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(strings.TrimSuffix(path, fileName), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "queries.txt")

	for _, content := range []string{"first", "second, longer than the first", ""} {
		if err := WriteFileAtomic(path, []byte(content)); err != nil {
			t.Fatalf("WriteFileAtomic(%q): %v", content, err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != content {
			t.Fatalf("expected %q in the file, got %q (%v)", content, got, err)
		}
	}

	// A write that can't complete leaves what was there, and no temporary
	// file behind: here the target is a directory.
	blocked := filepath.Join(dir, "blocked")
	if err := os.MkdirAll(filepath.Join(blocked, "inside"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := WriteFileAtomic(blocked, []byte("x")); err == nil {
		t.Error("expected writing over a directory to fail")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temporary file left behind: %s", entry.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(blocked, "inside")); err != nil {
		t.Errorf("expected the directory in the way to be left alone: %v", err)
	}
}

// TestSaveKeepsTheFileDocumented checks that what every successful
// connection does — move the cluster to the top of the history and save —
// leaves the comments explaining each setting in place.
func TestSaveKeepsTheFileDocumented(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load() // first launch: writes the self-documented file
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	before := readConfigFile(t)

	cfg.Promote(Cluster{URL: "https://es.example.com:9200", AuthType: AuthBasic, Username: "svc", TLS: TLS{Verify: true}})
	cfg.Language = "en"
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	after := readConfigFile(t)
	t.Logf("config.yaml after Save:\n%s", after)

	for _, line := range strings.Split(before, "\n") {
		if strings.HasPrefix(line, "#") && !strings.Contains(after, line) {
			t.Errorf("comment lost by Save: %q", line)
		}
	}
	for _, want := range []string{"language: en", "https://es.example.com:9200", "username: svc"} {
		if !strings.Contains(after, want) {
			t.Errorf("expected %q in the saved file", want)
		}
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if reloaded.Language != "en" || len(reloaded.Clusters) != 1 || reloaded.Clusters[0].Username != "svc" || !reloaded.Clusters[0].TLS.Verify {
		t.Errorf("unexpected configuration after reload: %+v", reloaded)
	}
	if again := readConfigFile(t); again != after {
		t.Errorf("expected a reload to leave the saved file alone, it became:\n%s", again)
	}
}

// TestSaveKeepsClusterSettingsWrittenByHand checks the per-cluster settings
// that have no form field — the proxy, the distribution and version
// overrides: written by hand in config.yaml, they are read, and still there
// once the history has been rewritten by a connection to another cluster.
func TestSaveKeepsClusterSettingsWrittenByHand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := Load(); err != nil { // first launch: writes the self-documented file
		t.Fatalf("Load: %v", err)
	}
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	byHand := strings.Replace(readConfigFile(t), "clusters: []", `clusters:
  - url: https://es-dmz.example.com:9200
    auth_type: none
    proxy: socks5://127.0.0.1:1080
    distribution: opensearch
    version: "2.19"
    tls:
      verify: true
`, 1)
	if err := os.WriteFile(path, []byte(byHand), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Cluster{
		URL: "https://es-dmz.example.com:9200", AuthType: AuthNone, TLS: TLS{Verify: true},
		Proxy: "socks5://127.0.0.1:1080", Distribution: "opensearch", Version: "2.19",
	}
	if len(cfg.Clusters) != 1 || cfg.Clusters[0] != want {
		t.Fatalf("expected %+v to be read, got %+v", want, cfg.Clusters)
	}

	cfg.Promote(Cluster{URL: "https://other.example.com:9200", AuthType: AuthNone, TLS: TLS{Verify: true}})
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reloaded, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if len(reloaded.Clusters) != 2 || reloaded.Clusters[1] != want {
		t.Errorf("expected %+v to survive the save, got %+v", want, reloaded.Clusters)
	}
}

// TestSaveKeepsExplicitEmptyCertDir checks that a default directory
// deliberately set to "" (no pre-filled path) survives a save: written back
// from the structure alone, the empty value was left out of the file, and the
// built-in default came back on the next start.
func TestSaveKeepsExplicitEmptyCertDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	withDefaultCertDir(t, "/etc/pki/tls/certs")

	writeConfigFile(t, `default_timeout_seconds: 60
language: fr
mouse: false
# No default directory here.
default_ca_dir: ""
default_client_cert_dir: ""
clusters: []
`)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultCADir != "" {
		t.Fatalf("test setup: expected the explicit empty value to be read, got %q", cfg.DefaultCADir)
	}

	cfg.Promote(Cluster{URL: "http://localhost:9200", AuthType: AuthNone})
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if reloaded.DefaultCADir != "" || reloaded.DefaultClientCertDir != "" {
		t.Errorf("expected the directories to stay empty, got %q and %q", reloaded.DefaultCADir, reloaded.DefaultClientCertDir)
	}
	if reloaded.DefaultTimeoutSeconds != 60 || len(reloaded.Clusters) != 1 {
		t.Errorf("unexpected configuration after reload: %+v", reloaded)
	}
	if saved := readConfigFile(t); !strings.Contains(saved, "# No default directory here.") {
		t.Errorf("expected the user's own comment to be kept, got:\n%s", saved)
	}
}

// TestSaveKeepsWhatItDoesNotKnow checks that a key this version doesn't
// know (written by a later one, or by hand) isn't dropped by a save.
func TestSaveKeepsWhatItDoesNotKnow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfigFile(t, `default_timeout_seconds: 30
future_setting: kept
clusters:
  - url: http://old:9200
    auth_type: none
    tls:
      verify: false
`)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.Promote(Cluster{URL: "http://new:9200", AuthType: AuthNone})
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	saved := readConfigFile(t)
	if !strings.Contains(saved, "future_setting: kept") {
		t.Errorf("expected the unknown key to be kept, got:\n%s", saved)
	}
	reloaded, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if len(reloaded.Clusters) != 2 || reloaded.Clusters[0].URL != "http://new:9200" || reloaded.Clusters[1].URL != "http://old:9200" {
		t.Errorf("unexpected history after reload: %+v", reloaded.Clusters)
	}
}

// TestSaveWithoutAFile checks the save of a configuration that was never
// read from disk: the file is created, documented like a first launch's.
func TestSaveWithoutAFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &Config{DefaultTimeoutSeconds: 5, Clusters: []Cluster{{URL: "http://localhost:9200", AuthType: AuthNone}}}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	saved := readConfigFile(t)
	if !strings.Contains(saved, "default_timeout_seconds: 5") || !strings.Contains(saved, "http://localhost:9200") {
		t.Errorf("expected the values to be written, got:\n%s", saved)
	}
	if !strings.Contains(saved, "# ") {
		t.Errorf("expected the new file to be documented, got:\n%s", saved)
	}
}
