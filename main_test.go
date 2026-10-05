package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"termdevtools/refdata"
	"termdevtools/ui"
)

// TestBuildPathsWritesOnlyToTheUserDirectory checks where the interface is
// told to write: in a shared or read-only installation (/opt,
// /usr/local/bin) the binary's directory can't be written to, so exports go
// to the user's own directory. What an installation shares is still read
// from the binary's.
func TestBuildPathsWritesOnlyToTheUserDirectory(t *testing.T) {
	exeDir := filepath.Join(t.TempDir(), "opt", "termdevtools")
	userDir := filepath.Join(t.TempDir(), "config", "termdevtools")
	reference := refdata.Sources{TeamDir: exeDir, UserDir: userDir}

	paths := buildPaths(exeDir, userDir, reference)

	if want := filepath.Join(userDir, ui.ExportsDirName); paths.Exports != want {
		t.Errorf("expected exports in the user's directory (%s), got %s", want, paths.Exports)
	}
	if strings.HasPrefix(paths.Exports, exeDir) {
		t.Errorf("expected nothing to be written under the binary's directory, got exports in %s", paths.Exports)
	}
	if want := filepath.Join(exeDir, ui.CheatsheetFileName); paths.Cheatsheet != want {
		t.Errorf("expected the shared cheatsheet next to the binary (%s), got %s", want, paths.Cheatsheet)
	}
	if paths.Reference != reference {
		t.Errorf("expected the reference sources to be passed through, got %+v", paths.Reference)
	}
	if paths.Starter == "" {
		t.Error("expected the built-in starting content of the editor")
	}
}

// TestWriteCrashReport checks that a crash report lands in the directory it
// is given, even one that doesn't exist (anymore), and that a directory
// which can't be written to is reported rather than ignored: the caller then
// prints the report instead.
func TestWriteCrashReport(t *testing.T) {
	now := time.Date(2026, time.October, 5, 14, 3, 9, 0, time.UTC)
	report := "boom\n\ngoroutine 1 [running]:\n"

	dir := filepath.Join(t.TempDir(), "missing", "termdevtools")
	path, err := writeCrashReport(dir, report, now)
	if err != nil {
		t.Fatalf("writeCrashReport: %v", err)
	}
	if want := filepath.Join(dir, "crash-20261005-140309.log"); path != want {
		t.Errorf("expected the report at %s, got %s", want, path)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != report {
		t.Errorf("expected the report in the file, got %q (%v)", got, err)
	}

	// A file where the directory should be: nothing can be written there.
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if path, err := writeCrashReport(filepath.Join(blocked, "inside"), report, now); err == nil {
		t.Errorf("expected writing under a file to fail, got %s", path)
	}
}
