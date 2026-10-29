package ui

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// pasteboard copies text to the clipboard of the machine the program runs
// on; nil where that isn't the user's own clipboard or can't be reached
// without a dependency — see localPasteboard. F2 and F9 fall back on the
// terminal (OSC 52) without it.
var pasteboard = localPasteboard(runtime.GOOS, os.Getenv)

// localPasteboard is pbcopy on a Mac the program runs on directly, where it
// is always there and works in any terminal — macOS Terminal included,
// which ignores OSC 52. Nil anywhere else, and through SSH, where pbcopy
// would fill the remote Mac's pasteboard instead of the user's: only the
// terminal reaches theirs.
func localPasteboard(goos string, getenv func(string) string) func(string) error {
	if goos != "darwin" {
		return nil
	}
	for _, name := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		if getenv(name) != "" {
			return nil
		}
	}
	return pbcopy
}

// pbcopy hands text to macOS's pbcopy command.
func pbcopy(text string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	// pbcopy reads its input in the locale's encoding: under a locale that
	// isn't UTF-8 (LANG unset, say), anything but ASCII would arrive garbled.
	cmd.Env = append(os.Environ(), "LC_ALL=en_US.UTF-8")
	return cmd.Run()
}

// copyToClipboard implements the copy of F2 and F9: text goes to the
// pasteboard when there is one (see pasteboard), else through the terminal
// (OSC 52, SPEC.md §3.3), which can't confirm the copy — message
// (InfoCopied, InfoCurlCopied) then says it depends on the terminal. A
// pasteboard that fails also falls back on the terminal.
func (a *App) copyToClipboard(text, message string) {
	if pasteboard != nil && pasteboard(text) == nil {
		a.status.SetInfo(message)
		return
	}
	if a.screen != nil {
		a.screen.SetClipboard([]byte(text))
	}
	a.status.SetInfo(message + a.msgs.InfoCopiedViaTerminal)
}
