package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The interface's palette (SPEC.md §3.1): fixed colors on a dark background
// of its own, so that it reads the same whatever the terminal's theme. Red
// and yellow say something about the cluster or the last request and nothing
// else; the rest of the chrome keeps to blue and gray. A terminal without
// 24-bit colors gets the closest colors it has, from tcell.
var (
	themeBackground = tcell.NewHexColor(0x1D1E24)
	themeSurface    = tcell.NewHexColor(0x25262E) // the header
	themeField      = tcell.NewHexColor(0x343741) // form fields, buttons
	themeText       = tcell.NewHexColor(0xDFE5EF)
	themeMuted      = tcell.NewHexColor(0x98A2B3)
	themeBorder     = tcell.NewHexColor(0x69707D)
	themeAccent     = tcell.NewHexColor(0x1BA9F5)
	themeTeal       = tcell.NewHexColor(0x00BFB3)
	themeYellow     = tcell.NewHexColor(0xFEC514)
	themePink       = tcell.NewHexColor(0xF04E98)
	themeRed        = tcell.NewHexColor(0xF86B63) // on the dark background
	themeDanger     = tcell.NewHexColor(0xBD271E) // behind white text
)

func init() {
	tview.Styles = tview.Theme{
		PrimitiveBackgroundColor:    themeBackground,
		ContrastBackgroundColor:     themeField,
		MoreContrastBackgroundColor: themeField,
		BorderColor:                 themeBorder,
		TitleColor:                  themeText,
		GraphicsColor:               themeBorder,
		PrimaryTextColor:            themeText,
		SecondaryTextColor:          themeAccent,
		TertiaryTextColor:           themeMuted,
		InverseTextColor:            themeBackground,
		ContrastSecondaryTextColor:  themeMuted,
	}

	// The color tags, here and in the i18n catalogs, keep the names that say
	// what they mean ("[red]" an error, "[gray]" what stays in the
	// background): the theme defines what the names stand for, "[white]"
	// being the color of the text.
	tcell.ColorNames["white"] = themeText
	tcell.ColorNames["gray"] = themeMuted
	tcell.ColorNames["aqua"] = themeAccent
	tcell.ColorNames["green"] = themeTeal
	tcell.ColorNames["yellow"] = themeYellow
	tcell.ColorNames["fuchsia"] = themePink
	tcell.ColorNames["red"] = themeRed

	// The panel with the focus is told apart by its shape as well as by its
	// color (panelBorderColor), which a terminal may not show (NO_COLOR).
	tview.Borders.HorizontalFocus = tview.BoxDrawingsHeavyHorizontal
	tview.Borders.VerticalFocus = tview.BoxDrawingsHeavyVertical
	tview.Borders.TopLeftFocus = tview.BoxDrawingsHeavyDownAndRight
	tview.Borders.TopRightFocus = tview.BoxDrawingsHeavyDownAndLeft
	tview.Borders.BottomLeftFocus = tview.BoxDrawingsHeavyUpAndRight
	tview.Borders.BottomRightFocus = tview.BoxDrawingsHeavyUpAndLeft
}

// panelBorderColor is the border color of a panel with or without the focus.
func panelBorderColor(focused bool) tcell.Color {
	if focused {
		return themeAccent
	}
	return themeBorder
}

// methodColor is the color of an HTTP method in the editor (paintRequests):
// what can destroy data stands out from what only reads.
func methodColor(method string) tcell.Color {
	switch method {
	case "GET":
		return themeTeal
	case "DELETE":
		return themeRed
	}
	return themeAccent
}
