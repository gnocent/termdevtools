package config

import (
	"path/filepath"
	"strings"
	"testing"
)

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
	// Aucun caractère problématique pour un nom de fichier (":", "/", ...).
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
