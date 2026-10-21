package ui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/gnocent/termdevtools/i18n"
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

	status *statusLine
	help   *tview.TextView
	// renders counts what was written to the status line, see Version.
	renders int
	// message and color are what the status line currently says, at its end.
	message, color string
}

// statusLine is the status line's TextView, its text composed for the width
// it is drawn at (StatusBar.compose): the width is only known then.
type statusLine struct {
	*tview.TextView
	compose func(width int) string
	// width is the one the current text was composed for; 0 before the
	// first draw, when the whole line is composed.
	width int
}

func (l *statusLine) Draw(screen tcell.Screen) {
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

// NewStatusBar creates the status bar for the given cluster/user. product
// is the cluster's detected distribution and version ("ES 9.5.4"), or empty.
func NewStatusBar(product, cluster, user string, msgs *i18n.Strings) *StatusBar {
	sb := &StatusBar{
		product: product,
		cluster: cluster,
		user:    user,
		msgs:    msgs,
		help:    tview.NewTextView().SetDynamicColors(true).SetText(msgs.ShortcutsHelpBar),
	}
	sb.status = &statusLine{TextView: tview.NewTextView().SetDynamicColors(true), compose: sb.compose}
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

// render writes message at the end of the status line.
func (sb *StatusBar) render(message, color string) {
	sb.renders++
	sb.message, sb.color = message, color
	sb.status.SetText(sb.compose(sb.status.width))
}

// statusSeparator separates the parts of the status line.
const statusSeparator = "  |  "

// minStatusURLWidth is the fewest columns a URL cut short keeps on the status
// line, "…" included: fewer wouldn't say which cluster it is.
const minStatusURLWidth = 12

// compose builds the status line for width columns: the cluster, the user,
// then the message — the part that matters most, often the only sign that
// something happened (a write refused on a read-only cluster). When the
// whole line doesn't fit, the user goes first, then the URL is cut short,
// and last the cluster goes too, the message being cut short only when it
// doesn't fit alone. The detected product, when known, takes the place of
// the generic "Cluster" caption in front of the URL rather than being added
// to the line, for the same reason. A width of 0 (not drawn yet) gives the
// whole line.
func (sb *StatusBar) compose(width int) string {
	caption := sb.product
	if caption == "" {
		caption = sb.msgs.StatusClusterCaption
	}
	cluster := func(url string) string {
		return fmt.Sprintf("[white]%s: [green]%s[white]", tview.Escape(caption), tview.Escape(url))
	}
	user := fmt.Sprintf("%s: [green]%s[white]", tview.Escape(sb.msgs.StatusUserCaption), tview.Escape(sb.user))
	message := func(text string) string {
		return fmt.Sprintf("[%s]%s[white]", sb.color, tview.Escape(text))
	}

	clusterWidth := uniseg.StringWidth(caption + ": " + sb.cluster)
	userWidth := uniseg.StringWidth(sb.msgs.StatusUserCaption + ": " + sb.user)
	messageWidth := uniseg.StringWidth(sb.message)
	separator := len(statusSeparator)
	switch {
	case width <= 0 || clusterWidth+separator+userWidth+separator+messageWidth <= width:
		return cluster(sb.cluster) + statusSeparator + user + statusSeparator + message(sb.message)
	case clusterWidth+separator+messageWidth <= width:
		return cluster(sb.cluster) + statusSeparator + message(sb.message)
	}
	if room := width - separator - messageWidth - uniseg.StringWidth(caption+": "); room >= minStatusURLWidth {
		return cluster(cutShort(sb.cluster, room)) + statusSeparator + message(sb.message)
	}
	return message(cutShort(sb.message, width))
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
