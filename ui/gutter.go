package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

// gutterPadding is the blank margin (in columns) added after the line
// number itself, purely visual — separates the numbers from the text.
// Needs the gutter's own TextView left at its default (left) alignment: a
// number formatted to a fixed digit width (gutterText's use of "%*d") and
// then just... not filling the rest of the box already leaves this margin
// blank on its own. An earlier version paired right-alignment with this
// same padding, which put the blank column on the wrong side — flush
// against the border instead of between the numbers and the text, which is
// what actually prompted adding it.
const gutterPadding = 2

// editorLayout wraps the editor's container Flex (gutter + TextArea) so the
// gutter's line numbers can be refreshed right before every draw — needed
// because TextArea has no change notification for scrolling or resizing on
// its own (SPEC.md §7 backlog #5).
//
// Read from the CONTAINER's own inner rect (editor.refreshGutter), not the
// TextArea's: tview.Flex sets every child's rect immediately, in order, but
// defers the *draw* call of whichever child currently has focus — since the
// gutter is never focused, its own Draw could run before the (focused)
// TextArea's Draw, at which point the TextArea's rect from a mid-resize
// frame could already be set but the gutter would have no direct signal of
// that. Reading the container's own rect sidesteps the question entirely:
// it's always set by the *container's own parent* before this Draw ever
// runs, regardless of which child inside has focus.
type editorLayout struct {
	*tview.Flex
	editor *Editor
}

func (l *editorLayout) Draw(screen tcell.Screen) {
	l.editor.refreshGutter()
	l.Flex.Draw(screen)
}

// refreshGutter recomputes the gutter's displayed line numbers to match the
// editor's current scroll position and content.
//
// TextArea exposes no public API to learn where it wrapped, so
// wrapRowStarts reimplements that computation using the same underlying
// library (uniseg) TextArea itself uses internally (see its own
// extendLines) — the width passed in must exactly match what TextArea
// renders at, i.e. the container's own inner width minus the gutter's own.
func (e *Editor) refreshGutter() {
	_, _, totalWidth, height := e.container.GetInnerRect()
	if totalWidth <= 0 || height <= 0 {
		return
	}

	text := e.Text()
	totalLines := strings.Count(text, "\n") + 1
	digits := len(strconv.Itoa(totalLines))
	gutterWidth := digits + gutterPadding
	e.container.ResizeItem(e.gutter, gutterWidth, 0)

	textWidth := totalWidth - gutterWidth
	if textWidth < 1 {
		textWidth = 1
	}
	// This runs before every draw — each keystroke, each answer received —
	// and wrapping the whole buffer is by far its most expensive part: only
	// redone when the text or the width actually changed (comparing two
	// strings costs next to nothing next to it).
	if e.wrappedStarts == nil || textWidth != e.wrappedWidth || text != e.wrappedText {
		e.wrappedText, e.wrappedWidth, e.wrappedStarts = text, textWidth, wrapRowStarts(text, textWidth)
	}
	rowOffset, _ := e.view.GetOffset()

	e.gutter.SetText(gutterText(text, e.wrappedStarts, rowOffset, height, digits))
}

// gutterText renders the gutter's content for the display rows [rowOffset,
// rowOffset+height) out of rowStarts (see wrapRowStarts): one line per
// visible row, either the logical line number (the row where that logical
// line actually starts), right-aligned within digits columns, or blank (a
// word-wrapped continuation row) — the usual editor convention, so a long
// wrapped line doesn't look like several separate ones. Colored gray,
// echoing the "#" comment convention used elsewhere (SPEC.md §3.2, §3.3),
// so the numbers read as chrome rather than part of the request text next
// to them — reported as hard to tell apart otherwise.
//
// Kept separate from refreshGutter so this numbering logic can be tested
// directly against a hand-built rowStarts, independently of wrapRowStarts'
// own correctness.
func gutterText(text string, rowStarts []int, rowOffset, height, digits int) string {
	blank := strings.Repeat(" ", digits)

	var b strings.Builder
	b.WriteString("[gray]")
	logicalLine := 0
	wroteAny := false
	for i, start := range rowStarts {
		isLineStart := i == 0 || text[start-1] == '\n'
		if isLineStart {
			logicalLine++
		}
		if i < rowOffset {
			continue
		}
		if i >= rowOffset+height {
			break
		}
		if wroteAny {
			b.WriteByte('\n')
		}
		wroteAny = true
		if isLineStart {
			fmt.Fprintf(&b, "%*d", digits, logicalLine)
		} else {
			b.WriteString(blank)
		}
	}
	return b.String()
}

// wrapRowStarts returns the byte offsets in text where each display row
// begins once word-wrapped at width columns — a port of
// tview.TextArea's own internal word-wrap algorithm (its extendLines,
// unexported and tightly coupled to TextArea's own edit-span bookkeeping,
// so not reusable directly) onto a plain string, using the same
// underlying library (github.com/rivo/uniseg) for grapheme/line-break
// analysis so the two stay in step. Assumes wrap and word-wrap are both
// enabled, matching this editor's own NewEditor setup — TextArea defaults
// word-wrap to true and this code never turns it off.
//
// The first element is always 0. Tabs are counted as tview.TabSize columns
// wide, matching TextArea.step's own special-case (not a real tab-stop
// computation — same simplification TextArea itself makes).
func wrapRowStarts(text string, width int) []int {
	rowStarts := []int{0}
	if width <= 0 || text == "" {
		return rowStarts
	}

	var (
		pos                              int
		state                            = -1
		lineWidth, widthSinceLastBreak   int
		lastGraphemeBreak, lastLineBreak = -1, -1
	)

	for pos < len(text) {
		cluster, rest, boundaries, newState := uniseg.StepString(text[pos:], state)
		state = newState

		clusterWidth := boundaries >> uniseg.ShiftWidth
		if cluster == "\t" {
			clusterWidth = tview.TabSize
		}
		clusterEnd := pos + len(cluster)

		lineWidth += clusterWidth
		widthSinceLastBreak += clusterWidth

		if lineWidth <= width {
			if boundaries&uniseg.MaskLine == uniseg.LineMustBreak && (len(rest) > 0 || uniseg.HasTrailingLineBreakInString(cluster)) {
				rowStarts = append(rowStarts, clusterEnd)
				lineWidth, widthSinceLastBreak = 0, 0
				lastGraphemeBreak, lastLineBreak = -1, -1
				pos = clusterEnd
				continue
			}
		} else { // lineWidth > width: this cluster doesn't fit, break somewhere before it.
			if lastLineBreak == -1 {
				if lastGraphemeBreak != -1 { // At least one character already on this row.
					rowStarts = append(rowStarts, lastGraphemeBreak)
					lineWidth = clusterWidth
					lastLineBreak = -1
				}
			} else {
				rowStarts = append(rowStarts, lastLineBreak)
				lineWidth = widthSinceLastBreak
				lastLineBreak = -1
			}
		}

		if boundaries&uniseg.MaskLine == uniseg.LineCanBreak {
			lastLineBreak = clusterEnd
			widthSinceLastBreak = 0
		}
		lastGraphemeBreak = clusterEnd

		pos = clusterEnd
	}

	return rowStarts
}
