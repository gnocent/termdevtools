package ui

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestMain(m *testing.M) {
	// The tests check what F2 and F9 send to the terminal (OSC 52). On a Mac
	// they would go to the real pasteboard instead — and replace what the
	// developer running them had copied.
	pasteboard = nil
	os.Exit(m.Run())
}

func TestLocalPasteboard(t *testing.T) {
	for _, c := range []struct {
		name string
		goos string
		env  map[string]string
		want bool
	}{
		{"on a Mac", "darwin", nil, true},
		{"on Linux", "linux", nil, false},
		{"on Windows", "windows", nil, false},
		{"through SSH (SSH_CONNECTION)", "darwin", map[string]string{"SSH_CONNECTION": "10.0.0.1 52000 10.0.0.2 22"}, false},
		{"through SSH (SSH_CLIENT)", "darwin", map[string]string{"SSH_CLIENT": "10.0.0.1 52000 22"}, false},
		{"through SSH (SSH_TTY)", "darwin", map[string]string{"SSH_TTY": "/dev/ttys003"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := localPasteboard(c.goos, func(name string) string { return c.env[name] }) != nil
			if got != c.want {
				t.Errorf("expected a pasteboard: %v, got %v", c.want, got)
			}
		})
	}
}

// usePasteboard makes the tests' app copy through fake instead of the
// terminal, as on a Mac.
func usePasteboard(t *testing.T, fake func(string) error) {
	t.Helper()
	pasteboard = fake
	t.Cleanup(func() { pasteboard = nil })
}

func TestCopyGoesToThePasteboard(t *testing.T) {
	var copied []string
	usePasteboard(t, func(text string) error {
		copied = append(copied, text)
		return nil
	})
	app, screen := newTestApp(t)

	onUI(app, func() { app.result.Show("GET", "_cluster/health", nil, []byte(`{"status":"green"}`)) })
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF2, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if want := uiValue(app, app.result.PlainText); len(copied) != 1 || copied[0] != want {
		t.Errorf("expected F2 to copy %q to the pasteboard, got %q", want, copied)
	}
	if got := screen.GetClipboardData(); len(got) != 0 {
		t.Errorf("expected nothing sent to the terminal once the pasteboard has it, got %q", got)
	}
	// The pasteboard confirms the copy: the message doesn't hedge.
	if status := statusText(app); !strings.Contains(status, app.msgs.InfoCopied) || strings.Contains(status, "OSC 52") {
		t.Errorf("expected the plain copy confirmation, got %q", status)
	}

	injectText(screen, "GET _cat/health?v")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF9, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if want := "curl -X GET 'http://127.0.0.1:1/_cat/health?v'"; len(copied) != 2 || copied[1] != want {
		t.Errorf("expected F9 to copy %q to the pasteboard, got %q", want, copied)
	}
	if status := statusText(app); !strings.Contains(status, app.msgs.InfoCurlCopied) || strings.Contains(status, "OSC 52") {
		t.Errorf("expected the plain copy confirmation, got %q", status)
	}
}

// TestCopyFallsBackOnTheTerminal: a pasteboard that fails (pbcopy missing,
// say) leaves the copy to the terminal, as anywhere else.
func TestCopyFallsBackOnTheTerminal(t *testing.T) {
	usePasteboard(t, func(string) error { return errors.New("pbcopy: not found") })
	app, screen := newTestApp(t)

	onUI(app, func() { app.result.Show("GET", "_cluster/health", nil, []byte(`{"status":"green"}`)) })
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF2, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := string(screen.GetClipboardData()), uiValue(app, app.result.PlainText); got != want {
		t.Errorf("expected the terminal to get %q, got %q", want, got)
	}
	if status, want := statusText(app), app.msgs.InfoCopied+app.msgs.InfoCopiedViaTerminal; !strings.Contains(status, want) {
		t.Errorf("expected the status bar to say %q, got %q", want, status)
	}
}
