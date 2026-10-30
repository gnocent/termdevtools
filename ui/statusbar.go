package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/gnocent/termdevtools/i18n"
)

// StatusBar is the line at the bottom of the main screen: the state of the
// last operation (request in progress, result, error...) and, in the room
// that leaves, the shortcuts. See SPEC.md §3.1 and §4.
type StatusBar struct {
	msgs *i18n.Strings

	status *composedLine
	// renders counts what was written to the status line, see Version.
	renders int
	// message and color are what the status line currently says.
	message, color string
}

// composedLine is a one-line TextView whose text is composed for the width
// it is drawn at (StatusBar.compose, header.compose): the width is only
// known then.
type composedLine struct {
	*tview.TextView
	compose func(width int) string
	// width is the one the current text was composed for; 0 before the
	// first draw, when the whole line is composed.
	width int
}

func (l *composedLine) Draw(screen tcell.Screen) {
	if _, _, width, _ := l.GetInnerRect(); width != l.width {
		l.width = width
		l.SetText(l.compose(width))
	}
	l.TextView.Draw(screen)
}

// Version changes every time the status line is written: what lets a
// background operation tell, when it ends, whether the line still shows what
// it put there or something more recent that it must not cover.
func (sb *StatusBar) Version() int {
	return sb.renders
}

// NewStatusBar creates the status bar.
func NewStatusBar(msgs *i18n.Strings) *StatusBar {
	sb := &StatusBar{msgs: msgs}
	view := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	view.SetBorderPadding(0, 0, 1, 1)
	sb.status = &composedLine{TextView: view, compose: sb.compose}
	sb.SetIdle()
	return sb
}

// SetLanguage re-applies the shortcuts in msgs' language. The current
// message is left untouched — it stays in the previous language until the
// next SetIdle/SetRunning/SetInfo/SetResult/SetError call re-renders it (the
// caller typically issues one right after SetLanguage to confirm the switch
// immediately).
func (sb *StatusBar) SetLanguage(msgs *i18n.Strings) {
	sb.msgs = msgs
	sb.status.SetText(sb.compose(sb.status.width))
}

// Widget returns the line to insert into the layout.
func (sb *StatusBar) Widget() tview.Primitive {
	return sb.status
}

// render writes message on the status line.
func (sb *StatusBar) render(message, color string) {
	sb.renders++
	sb.message, sb.color = message, color
	sb.status.SetText(sb.compose(sb.status.width))
}

const (
	// shortcutsSeparator separates the entries of msgs.ShortcutsHelpBar.
	shortcutsSeparator = "   "
	// shortcutsGap is what separates them on the line, where room counts.
	shortcutsGap = "  "
	// minStatusGap is the fewest columns between the message and the
	// shortcuts.
	minStatusGap = 3
)

// compose builds the status line for width columns: the message on the left
// — the part that matters most, often the only sign that something happened
// (a write refused on a read-only cluster) — and on the right as many
// shortcuts as the message leaves room for, in the order of the catalog and
// never one cut in two. The message is only cut short when it doesn't fit
// alone. A width of 0 (not drawn yet) gives the whole line.
func (sb *StatusBar) compose(width int) string {
	messageWidth := uniseg.StringWidth(sb.message)
	if width > 0 && messageWidth > width {
		return fmt.Sprintf("[%s]%s[white]", sb.color, tview.Escape(cutShort(sb.message, width)))
	}
	message := fmt.Sprintf("[%s]%s[white]", sb.color, tview.Escape(sb.message))

	var shortcuts []string
	columns := 0
	for _, entry := range strings.Split(sb.msgs.ShortcutsHelpBar, shortcutsSeparator) {
		needed := tview.TaggedStringWidth(entry)
		if len(shortcuts) > 0 {
			needed += len(shortcutsGap)
		}
		if width > 0 && messageWidth+minStatusGap+columns+needed > width {
			break
		}
		shortcuts = append(shortcuts, entry)
		columns += needed
	}
	if len(shortcuts) == 0 {
		return message
	}
	gap := minStatusGap
	if width > 0 {
		gap = width - messageWidth - columns
	}
	return message + strings.Repeat(" ", gap) + strings.Join(shortcuts, shortcutsGap)
}

// SetIdle displays the default "ready" state.
func (sb *StatusBar) SetIdle() {
	sb.render(sb.msgs.StatusIdle, "white")
}

// SetRunning indicates that a request is currently executing.
func (sb *StatusBar) SetRunning() {
	sb.render(sb.msgs.StatusRunning, "yellow")
}

// SetRunningFor indicates that a request has been executing for elapsed,
// and how to abandon it.
func (sb *StatusBar) SetRunningFor(elapsed time.Duration) {
	sb.render(fmt.Sprintf(sb.msgs.StatusRunningForFmt, int(elapsed/time.Second)), "yellow")
}

// SetInfo displays a confirmation (e.g. successful save/export).
func (sb *StatusBar) SetInfo(message string) {
	sb.render(message, "green")
}

// SetWarning displays something worth knowing that isn't a failure (e.g. a
// config.yaml override that had to be ignored).
func (sb *StatusBar) SetWarning(message string) {
	sb.render(message, "yellow")
}

// SetResult displays the result of a completed request.
func (sb *StatusBar) SetResult(statusCode int, duration time.Duration) {
	color := "green"
	if statusCode >= 400 || statusCode == 0 {
		color = "red"
	}
	sb.render(fmt.Sprintf(sb.msgs.StatusResultFmt, statusCode, duration.Round(time.Millisecond)), color)
}

// SetError displays an error message (invalid request, unreachable cluster...).
func (sb *StatusBar) SetError(message string) {
	sb.render(message, "red")
}
