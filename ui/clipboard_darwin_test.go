package ui

import (
	"os"
	"os/exec"
	"testing"
)

// pbpaste reads the pasteboard back as UTF-8, whatever the locale.
func pbpaste(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("pbpaste")
	cmd.Env = append(os.Environ(), "LC_ALL=en_US.UTF-8")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("pbpaste: %v", err)
	}
	return string(out)
}

// TestPbcopyKeepsUnicode copies through the real pasteboard, under a locale
// that isn't UTF-8 — the case where pbcopy left to itself garbles anything
// but ASCII. The pasteboard's previous text is put back afterwards.
func TestPbcopyKeepsUnicode(t *testing.T) {
	saved := pbpaste(t)
	t.Cleanup(func() { _ = pbcopy(saved) })
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		t.Setenv(name, "")
	}

	text := `{"index":"logs-été","node":"nœud-1 ✓"}`
	if err := pbcopy(text); err != nil {
		t.Fatalf("pbcopy: %v", err)
	}
	if got := pbpaste(t); got != text {
		t.Errorf("expected the pasteboard to hold %q, got %q", text, got)
	}
}
