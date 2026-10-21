package config

import (
	"os"
	"strings"
	"testing"
)

func TestNormalizeProtection(t *testing.T) {
	cases := []struct {
		value, level string
		known        bool
	}{
		{"", ProtectionNone, true},
		{"none", ProtectionNone, true},
		{"confirm", ProtectionConfirm, true},
		{"read_only", ProtectionReadOnly, true},
		{" Read_Only ", ProtectionReadOnly, true},
		{"CONFIRM", ProtectionConfirm, true},
		{"readonly", ProtectionReadOnly, false},
		{"read-only", ProtectionReadOnly, false},
		{"off", ProtectionReadOnly, false},
		{"yes", ProtectionReadOnly, false},
	}
	for _, c := range cases {
		level, known := NormalizeProtection(c.value)
		if level != c.level || known != c.known {
			t.Errorf("NormalizeProtection(%q) = (%q, %v), expected (%q, %v)", c.value, level, known, c.level, c.known)
		}
	}
}

// TestProtectionSurvivesSave checks that a cluster's protection level and
// label, as the connection form sets them, are saved and read back — and
// that a cluster without protection, whether written by hand or by the
// program, gets no "protection" key at all.
func TestProtectionSurvivesSave(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := Load(); err != nil { // first launch: writes the self-documented file
		t.Fatalf("Load: %v", err)
	}
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	byHand := strings.Replace(readConfigFile(t), "clusters: []", `clusters:
  - url: https://es-staging.example.com:9200
    auth_type: none
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
	staging := Cluster{URL: "https://es-staging.example.com:9200", AuthType: AuthNone, TLS: TLS{Verify: true}}
	if len(cfg.Clusters) != 1 || cfg.Clusters[0] != staging {
		t.Fatalf("expected %+v to be read, got %+v", staging, cfg.Clusters)
	}

	prod := Cluster{
		URL: "https://es-prod.example.com:9200", Protection: ProtectionConfirm, Label: "PROD",
		AuthType: AuthBasic, Username: "svc", TLS: TLS{Verify: true},
	}
	cfg.Promote(prod)
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	saved := readConfigFile(t)
	t.Logf("config.yaml after Save:\n%s", saved)
	for _, want := range []string{"protection: confirm", "label: PROD"} {
		if !strings.Contains(saved, want) {
			t.Errorf("expected %q in the saved file", want)
		}
	}
	keys := 0
	for _, line := range strings.Split(saved, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "protection:") {
			keys++
		}
	}
	if keys != 1 {
		t.Errorf("expected a protection key for the protected cluster only, found %d", keys)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if len(reloaded.Clusters) != 2 || reloaded.Clusters[0] != prod || reloaded.Clusters[1] != staging {
		t.Errorf("expected %+v then %+v after reload, got %+v", prod, staging, reloaded.Clusters)
	}
}
