package ui

import (
	"strings"
	"testing"

	"termdevtools/refdata"
)

// TestStatusBarShowsDetectedTarget checks that the distribution and version
// detected at connection are visible in front of the cluster's URL, in place
// of the generic "Cluster" caption.
func TestStatusBarShowsDetectedTarget(t *testing.T) {
	_, screen := newTestAppWith(t, testAppOptions{
		target: refdata.Target{Distribution: refdata.OpenSearch, Version: refdata.Version{Major: 2, Minor: 19, Patch: 6}, HasVersion: true},
	})

	if text := screenText(screen); !strings.Contains(text, "OS 2.19.6: http://127.0.0.1:1") {
		t.Errorf("expected the status bar to show the target in front of the URL, got:\n%s", text)
	}
}

// TestStatusBarWithoutDetectedTarget checks the fallback when detection
// found nothing: the generic caption.
func TestStatusBarWithoutDetectedTarget(t *testing.T) {
	_, screen := newTestAppWith(t, testAppOptions{undetected: true})

	if text := screenText(screen); !strings.Contains(text, "Cluster: http://127.0.0.1:1") {
		t.Errorf("expected the generic caption in front of the URL, got:\n%s", text)
	}
}

// TestStatusBarKeepsRoomForMessages guards the reason the target replaces
// the caption instead of being appended to the URL: on an 80-column
// terminal, the status message must not lose more than a character to it.
func TestStatusBarKeepsRoomForMessages(t *testing.T) {
	app, screen := newTestApp(t) // detected as ES 9.5.4

	message := "HTTP 200 en 1234ms" // a realistic message, 18 columns
	app.tapp.QueueUpdateDraw(func() { app.status.SetInfo(message) })
	waitForDraw(t, screen)

	if text := screenText(screen); !strings.Contains(text, message) {
		t.Errorf("expected the whole message to fit on an 80-column screen, got:\n%s", text)
	}
}

// TestConnectionWarningShownAtStartup checks that a warning raised during
// connection (ConnectResult.Warning) reaches the main screen's status bar.
func TestConnectionWarningShownAtStartup(t *testing.T) {
	_, screen := newTestAppWith(t, testAppOptions{warning: "override ignored"})

	if text := screenText(screen); !strings.Contains(text, "override ignored") {
		t.Errorf("expected the connection warning in the status bar, got:\n%s", text)
	}
}
