package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/gnocent/termdevtools/config"
	"github.com/gnocent/termdevtools/i18n"
)

// bannerText is what the header says of a protected cluster (SPEC.md §5): its
// label and its level ("PROD · read-only"), or the level alone, in capitals,
// without a label.
func bannerText(level, label string, msgs *i18n.Strings) string {
	name := msgs.ProtectionConfirmName
	if level == config.ProtectionReadOnly {
		name = msgs.ProtectionReadOnlyName
	}
	if label = bannerLabel(label); label == "" {
		return strings.ToUpper(name)
	}
	return label + " · " + name
}

// confirmPageName is the tview.Pages key of the popup asking to confirm a
// write (confirmWrite).
const confirmPageName = "confirm"

// confirmPathMax bounds how much of the path the confirmation popup shows:
// the request itself stays in sight in the editor.
const confirmPathMax = 120

// confirmPath fits path in confirmPathMax for the confirmation popup without
// losing its API, what says what the request does: the query string goes
// first, then what precedes the API — a long list of indices, typically — is
// cut in its middle, so that its first and last names stay:
// "logs-2026.10.01,…,logs-2026.10.15/_delete_by_query?…". An API too long
// to leave room for anything else (a document ID, a scroll ID) is cut at its
// end.
func confirmPath(path string) string {
	if len([]rune(path)) <= confirmPathMax {
		return path
	}
	target, _, hasQuery := strings.Cut(path, "?")
	suffix := ""
	if hasQuery {
		if len([]rune(target)) < confirmPathMax-1 {
			return cutShort(path, confirmPathMax)
		}
		suffix = "?…"
	}
	api := ""
	segments := strings.Split(target, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, "_") && segment != "_all" {
			target, api = strings.Join(segments[:i], "/"), "/"+strings.Join(segments[i:], "/")
			break
		}
	}
	room := confirmPathMax - len([]rune(api)) - len([]rune(suffix))
	if room < confirmPathMinTarget {
		return cutShort(path, confirmPathMax)
	}
	return cutMiddle(target, room) + api + suffix
}

// confirmPathMinTarget is the fewest columns confirmPath leaves what precedes
// the API: fewer wouldn't name any index.
const confirmPathMinTarget = 16

// cutShort returns s cut to width columns, ending with "…" when it was cut.
func cutShort(s string, width int) string {
	if uniseg.StringWidth(s) <= width {
		return s
	}
	var kept []rune
	used := 0
	for _, r := range s {
		w := uniseg.StringWidth(string(r))
		if used+w > width-1 {
			break
		}
		kept = append(kept, r)
		used += w
	}
	return string(kept) + "…"
}

// cutMiddle returns s cut to width runes in its middle, "…" in place of
// what was cut.
func cutMiddle(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	head := width / 2
	tail := width - 1 - head
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}

// Size of the confirmation popup: wide enough for a cluster's URL and a
// path on a line of their own — not tview.Modal, which takes a third of the
// screen, 26 columns on an 80-column terminal — and narrower than 80
// columns, like the help popup. The text gets 7 rows: the cluster, the
// request (two rows when its path is long), the question, blank lines
// between; the buttons 3, the border 2.
const (
	confirmWidth      = 70
	confirmTextHeight = 7
	confirmHeight     = confirmTextHeight + 3 + 2
)

// confirmWrite asks, in a popup, before sending a request that writes to a
// cluster in confirm protection (SPEC.md §5). The focus starts on Cancel,
// so that Enter pressed out of habit sends nothing; Escape cancels too. What
// is sent once confirmed is the request as it was resolved on Ctrl+E.
func (a *App) confirmWrite(method, path string, body []byte) {
	cluster := a.clusterURL
	if label := bannerLabel(a.label); label != "" {
		cluster = label + " — " + cluster
	}
	// No dynamic colors: the label and the path are shown as typed. Its
	// margins are its own, not the frame's: a tview.Flex never clears what
	// it holds, and margins of the frame would let the panels behind show
	// through.
	text := tview.NewTextView().SetWordWrap(true).
		SetText(fmt.Sprintf(a.msgs.ConfirmWriteFmt, cluster, method, confirmPath(path)))
	text.SetBorderPadding(0, 0, 1, 1)
	// Not to be clicked into: the keys would go to the text, Escape included.
	text.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		return tview.MouseConsumed, nil
	})

	done := func(confirmed bool) {
		a.closeConfirm()
		if confirmed {
			a.send(method, path, body)
			return
		}
		a.status.SetInfo(a.msgs.InfoWriteCancelled)
	}
	buttons := tview.NewForm().SetButtonsAlign(tview.AlignCenter).
		AddButton(a.msgs.ButtonCancel, func() { done(false) }).
		AddButton(a.msgs.ButtonSend, func() { done(true) })
	buttons.SetCancelFunc(func() { done(false) })
	buttons.SetFocus(0)
	// The arrows move between the buttons too, as in most dialogs: a
	// tview.Form only moves on Tab and Backtab. Without going round, unlike
	// Backtab: Left on Cancel then Enter would send.
	buttons.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		_, button := buttons.GetFocusedItemIndex()
		switch event.Key() {
		case tcell.KeyLeft:
			if button <= 0 {
				return nil
			}
			return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
		case tcell.KeyRight:
			if button >= buttons.GetButtonCount()-1 {
				return nil
			}
			return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
		}
		return event
	})

	frame := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(text, confirmTextHeight, 0, false).
		AddItem(buttons, 0, 1, true)
	frame.SetBorder(true).SetBorderColor(themeRed).
		SetTitle(a.msgs.ConfirmWriteTitle).SetTitleColor(themeRed)

	overlay := modalPage{tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(frame, confirmWidth, 0, true).
			AddItem(nil, 0, 1, false),
			confirmHeight, 0, true).
		AddItem(nil, 0, 1, false)}

	a.confirmVisible = true
	a.confirmButtons = buttons
	a.pages.AddPage(confirmPageName, overlay, true, true)
	a.tapp.SetFocus(buttons)
}

func (a *App) closeConfirm() {
	a.confirmVisible = false
	a.pages.RemovePage(confirmPageName)
	a.focusEditor()
}

// bannerLabel makes a label fit on the header's single line: a label written
// by hand in config.yaml may span several lines, or hold control characters
// that have no business reaching the terminal.
func bannerLabel(label string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, label)), " ")
}
