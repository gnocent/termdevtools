package ui

import (
	"fmt"
	"time"

	"github.com/rivo/tview"

	"termdevtools/i18n"
)

// StatusBar displays the current state (cluster, request in progress,
// result) and a help bar reminding the shortcuts. See SPEC.md §3.1 and §4.
type StatusBar struct {
	cluster string
	user    string
	msgs    *i18n.Strings

	status *tview.TextView
	help   *tview.TextView
}

// NewStatusBar creates the status bar for the given cluster/user.
func NewStatusBar(cluster, user string, msgs *i18n.Strings) *StatusBar {
	sb := &StatusBar{
		cluster: cluster,
		user:    user,
		msgs:    msgs,
		status:  tview.NewTextView().SetDynamicColors(true),
		help:    tview.NewTextView().SetDynamicColors(true).SetText(msgs.ShortcutsHelpBar),
	}
	sb.SetIdle()
	return sb
}

// Widget returns the container (status + help) to insert into the layout.
func (sb *StatusBar) Widget() tview.Primitive {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(sb.status, 1, 0, false).
		AddItem(sb.help, 1, 0, false)
}

func (sb *StatusBar) render(message, color string) {
	sb.status.SetText(fmt.Sprintf(
		sb.msgs.StatusBarTemplate,
		tview.Escape(sb.cluster), tview.Escape(sb.user), color, tview.Escape(message),
	))
}

// SetIdle displays the default "ready" state.
func (sb *StatusBar) SetIdle() {
	sb.render(sb.msgs.StatusIdle, "white")
}

// SetRunning indicates that a request is currently executing.
func (sb *StatusBar) SetRunning() {
	sb.render(sb.msgs.StatusRunning, "yellow")
}

// SetInfo displays a confirmation (e.g. successful save/export).
func (sb *StatusBar) SetInfo(message string) {
	sb.render(message, "green")
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
