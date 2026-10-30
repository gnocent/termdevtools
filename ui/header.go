package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/gnocent/termdevtools/config"
	"github.com/gnocent/termdevtools/i18n"
	"github.com/gnocent/termdevtools/refdata"
)

// headerBrand opens the header of an unprotected cluster.
const headerBrand = "TermDevTools"

// headerUserGap is the fewest columns between the URL and the user.
const headerUserGap = 2

// minHeaderURLWidth is the fewest columns a URL cut short keeps in the
// header, "…" included: fewer wouldn't say which cluster it is.
const minHeaderURLWidth = 12

// header is the line across the top of the main screen (SPEC.md §3.1, §5):
// which cluster this is and, on a protected cluster, how it is protected.
type header struct {
	line              *composedLine
	target            refdata.Target
	cluster, user     string
	protection, label string
	msgs              *i18n.Strings
}

// newHeader creates the header of the given cluster. protection is a
// normalized level (config.NormalizeProtection).
func newHeader(target refdata.Target, cluster, user, protection, label string, msgs *i18n.Strings) *header {
	h := &header{target: target, cluster: cluster, user: user, protection: protection, label: label, msgs: msgs}
	background, foreground := h.colors()
	view := tview.NewTextView().SetDynamicColors(true).SetWrap(false).
		SetTextStyle(tcell.StyleDefault.Background(background).Foreground(foreground))
	view.SetBackgroundColor(background).SetBorderPadding(0, 0, 1, 1)
	h.line = &composedLine{TextView: view, compose: h.compose}
	return h
}

// Widget returns the line to insert into the layout.
func (h *header) Widget() tview.Primitive {
	return h.line
}

// SetLanguage re-applies the header in msgs' language.
func (h *header) SetLanguage(msgs *i18n.Strings) {
	h.msgs = msgs
	h.line.SetText(h.compose(h.line.width))
}

// protected tells whether the cluster has a protection to show.
func (h *header) protected() bool {
	return h.protection == config.ProtectionConfirm || h.protection == config.ProtectionReadOnly
}

// colors are the header's background and text: the whole line says how the
// cluster is protected, red when writes can still be sent once confirmed,
// yellow when none can.
func (h *header) colors() (background, foreground tcell.Color) {
	switch h.protection {
	case config.ProtectionConfirm:
		return themeDanger, tcell.ColorWhite
	case config.ProtectionReadOnly:
		return themeYellow, themeBackground
	}
	return themeSurface, themeText
}

// badgeColors are the colors of the badge naming the product: told apart
// from one distribution to the other, dark on a protected cluster to stand
// out from the line's own color.
func (h *header) badgeColors() (background, foreground tcell.Color) {
	switch {
	case h.protected():
		return themeBackground, themeText
	case h.target.Distribution == refdata.Elasticsearch:
		return themeTeal, themeBackground
	case h.target.Distribution == refdata.OpenSearch:
		return themeAccent, themeBackground
	}
	return themeField, themeText
}

// badge is the product in short form ("ES 9.5.4"), or the generic caption
// when it isn't known.
func (h *header) badge() string {
	product := h.target.Label()
	if product == "" {
		product = h.msgs.StatusClusterCaption
	}
	return " " + product + " "
}

// compose builds the header for width columns: what opens it, the badge, the
// URL, and the user at the far right. When the whole line doesn't fit, the
// brand goes first — unlike a protection, which stays whatever happens —,
// then the user, then the URL is cut short, and last it goes too. A width of
// 0 (not drawn yet) gives the whole line.
func (h *header) compose(width int) string {
	lead, keepLead := headerBrand, false
	if h.protected() {
		lead, keepLead = bannerText(h.protection, h.label, h.msgs), true
	}
	used := func(lead, url, user string) int {
		columns := uniseg.StringWidth(h.badge())
		if lead != "" {
			columns += uniseg.StringWidth(lead) + 1
		}
		if url != "" {
			columns += 1 + uniseg.StringWidth(url)
		}
		if user != "" {
			columns += headerUserGap + uniseg.StringWidth(user)
		}
		return columns
	}

	switch {
	case width <= 0 || used(lead, h.cluster, h.user) <= width:
		return h.render(lead, h.cluster, h.user, width)
	case !keepLead && used("", h.cluster, h.user) <= width:
		return h.render("", h.cluster, h.user, width)
	}
	if !keepLead {
		lead = ""
	}
	room := width - used(lead, "", "") - 1
	if room >= minHeaderURLWidth {
		return h.render(lead, cutShort(h.cluster, room), "", width)
	}
	return h.render(lead, "", "", width)
}

// render lays the given parts out on a line of width columns, the user
// pushed to its end.
func (h *header) render(lead, url, user string, width int) string {
	var b strings.Builder
	columns := 0
	if lead != "" {
		fmt.Fprintf(&b, "[::b]%s[::-] ", tview.Escape(lead))
		columns += uniseg.StringWidth(lead) + 1
	}
	background, foreground := h.badgeColors()
	fmt.Fprintf(&b, "[%s:%s:b]%s[-:-:-]", foreground.CSS(), background.CSS(), tview.Escape(h.badge()))
	columns += uniseg.StringWidth(h.badge())
	if url != "" {
		b.WriteString(" " + tview.Escape(url))
		columns += 1 + uniseg.StringWidth(url)
	}
	if user != "" {
		gap := max(headerUserGap, width-columns-uniseg.StringWidth(user))
		b.WriteString(strings.Repeat(" ", gap))
		if h.protected() {
			b.WriteString(tview.Escape(user))
		} else {
			b.WriteString("[gray]" + tview.Escape(user) + "[-]")
		}
	}
	return b.String()
}
