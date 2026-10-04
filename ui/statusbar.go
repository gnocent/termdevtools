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
	// product is the cluster's detected distribution and version in short
	// form ("ES 9.5.4"), empty when unknown.
	product string
	cluster string
	user    string
	msgs    *i18n.Strings

	status *tview.TextView
	help   *tview.TextView
	// renders counts what was written to the status line, see Version.
	renders int
}

// Version changes every time the status line is written: what lets a
// background operation tell, when it ends, whether the line still shows what
// it put there or something more recent that it must not cover.
func (sb *StatusBar) Version() int {
	return sb.renders
}

// NewStatusBar creates the status bar for the given cluster/user. product
// is the cluster's detected distribution and version ("ES 9.5.4"), or empty.
func NewStatusBar(product, cluster, user string, msgs *i18n.Strings) *StatusBar {
	sb := &StatusBar{
		product: product,
		cluster: cluster,
		user:    user,
		msgs:    msgs,
		status:  tview.NewTextView().SetDynamicColors(true),
		help:    tview.NewTextView().SetDynamicColors(true).SetText(msgs.ShortcutsHelpBar),
	}
	sb.SetIdle()
	return sb
}

// SetLanguage re-applies the shortcuts help bar in msgs' language. The
// current status line is left untouched — it stays in the previous
// language until the next SetIdle/SetRunning/SetInfo/SetResult/SetError
// call re-renders it (the caller typically issues one right after
// SetLanguage to confirm the switch immediately).
func (sb *StatusBar) SetLanguage(msgs *i18n.Strings) {
	sb.msgs = msgs
	sb.help.SetText(msgs.ShortcutsHelpBar)
}

// Widget returns the container (status + help) to insert into the layout.
func (sb *StatusBar) Widget() tview.Primitive {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(sb.status, 1, 0, false).
		AddItem(sb.help, 1, 0, false)
}

// render draws the status line. The detected product, when known, takes the
// place of the generic "Cluster" caption in front of the URL rather than
// being appended to it: on an 80-column terminal the line has no room to
// spare, and whatever it gains in width is taken from the message at its
// end — the part that matters most.
func (sb *StatusBar) render(message, color string) {
	sb.renders++
	caption := sb.product
	if caption == "" {
		caption = sb.msgs.StatusClusterCaption
	}
	sb.status.SetText(fmt.Sprintf(
		sb.msgs.StatusBarTemplate,
		tview.Escape(caption), tview.Escape(sb.cluster), tview.Escape(sb.user), color, tview.Escape(message),
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
