package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// moreBelowMark is what a panel shows on its bottom border, next to the
// right corner, while there is more to see below (SPEC.md §3.1).
const moreBelowMark = '▼'

// markMoreBelow draws moreBelowMark on panel's bottom border.
func markMoreBelow(screen tcell.Screen, panel tview.Primitive) {
	x, y, width, height := panel.GetRect()
	if width < 3 || height < 2 {
		return
	}
	style := tcell.StyleDefault.Background(themeBackground).Foreground(themeText).Bold(true)
	screen.SetContent(x+width-2, y+height-1, moreBelowMark, nil, style)
}

// scrollPanel is a TextView with a border that follows the focus, which a
// border's color doesn't do by itself, and marks what is left below.
type scrollPanel struct {
	*tview.TextView
}

func (p *scrollPanel) Draw(screen tcell.Screen) {
	p.SetBorderColor(panelBorderColor(p.HasFocus()))
	p.TextView.Draw(screen)
	if p.moreBelow(screen) {
		markMoreBelow(screen, p)
	}
}

// moreBelow tells whether the panel can scroll further down, by trying to:
// TextView only says how many rows its text takes after wrapping all of it
// on the spot, which would freeze the interface on a large result, and not
// at the width it is drawn at. Drawing goes no further than the rows shown.
func (p *scrollPanel) moreBelow(screen tcell.Screen) bool {
	row, column := p.GetScrollOffset()
	p.ScrollTo(row+1, column)
	p.TextView.Draw(screen)
	reached, _ := p.GetScrollOffset()
	p.ScrollTo(row, column)
	p.TextView.Draw(screen)
	return reached > row
}
