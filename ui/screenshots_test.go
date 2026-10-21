//go:build screenshots

// Captures of the protection's screens, for a visual review: each one is the
// simulated 80x24 screen the tests drive, cell by cell, rendered to HTML with
// the colors of a terminal palette.
//
//	go test -tags screenshots -run TestScreenshots ./ui/
//
// writes ../plans/recette-garde-fous/captures.html (plans/ isn't tracked).
package ui

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/gnocent/termdevtools/config"
	"github.com/gnocent/termdevtools/i18n"
)

const screenshotsFile = "../plans/recette-garde-fous/captures.html"

type capture struct {
	title, check, screen string
}

func TestScreenshots(t *testing.T) {
	var captures []capture
	shoot := func(screen tcell.SimulationScreen, title, check string) {
		t.Helper()
		waitForDraw(t, screen)
		captures = append(captures, capture{title, check, renderScreen(screen)})
	}

	known := []config.Cluster{
		{URL: "https://es-prod.example.test:9200", AuthType: config.AuthBasic, Username: "svc_devtools", Protection: config.ProtectionConfirm, Label: "PROD"},
		{URL: "https://es-audit.example.test:9200", AuthType: config.AuthNone, Protection: config.ProtectionReadOnly},
		{URL: "https://es-dev.example.test:9200", AuthType: config.AuthNone, Label: "DEV"},
	}

	screen := connectScreenForCapture(t, known)
	shoot(screen, "Historique des clusters",
		"Le texte sous chaque URL protégée (« auth: basic · PROD · confirmation des écritures », « auth: none · LECTURE SEULE ») est-il lisible et compris d'un coup d'œil ? Le cluster de dev, non protégé, n'affiche pas son libellé.")

	for range known {
		screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	}
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // + Nouvelle connexion
	injectText(screen, "https://es-new.example.test:9200")
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to Protection
	shoot(screen, "Nouvelle connexion, protection « aucune »",
		"Focus sur Protection. Désactivé, le champ « Libellé (bandeau) » est maintenant grisé, intitulé et case, au lieu de n'afficher que son intitulé. Se voit-il assez, tout en paraissant inactif ?")

	kept := connectScreenForCapture(t, []config.Cluster{{URL: "https://es-dev.example.test:9200", AuthType: config.AuthNone, Label: "DEV"}})
	kept.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	shoot(kept, "Cluster connu sans protection, libellé « DEV » conservé",
		"Le libellé enregistré reste affiché, grisé. Lisible, et clairement inactif ?")

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	shoot(screen, "Liste Protection ouverte",
		"Les trois intitulés (« aucune », « confirmation des écritures », « lecture seule ») sont-ils clairs ?")

	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // lecture seule
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)   // to the label, now enabled
	injectText(screen, "PROD")
	shoot(screen, "« Lecture seule » choisie, libellé saisi",
		"Le libellé, réactivé par ce choix, est-il lisible pendant la saisie ?")

	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to the authentication
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	for range 4 {
		screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	}
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // certificat client (mTLS)
	shoot(screen, "Formulaire le plus long : https et certificat client, en 80×24",
		"Tout tient, boutons compris, au prix des lignes vides entre les champs (retirées seulement dans ce cas). Est-ce encore lisible ?")

	unknown := []config.Cluster{{URL: "https://es-legacy.example.test:9200", AuthType: config.AuthNone, Protection: "readonly"}}
	screen = connectScreenForCapture(t, unknown)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	shoot(screen, "Cluster connu, protection inconnue dans config.yaml (« readonly »)",
		"L'avertissement en bas est-il clair, et la liste affiche-t-elle bien « lecture seule » ?")

	const requests = "# Nettoyage des index de logs\nGET _cat/indices/logs-*?v&s=index\n\nDELETE /logs-2026.10"

	app, screen := newTestAppWith(t, testAppOptions{clusterURL: "https://es-prod.example.test:9200", protection: config.ProtectionConfirm, label: "PROD"})
	execute(t, app, screen, requests)
	shoot(screen, "Confirmation d'une écriture (« confirmation des écritures », libellé PROD)",
		"Fenêtre lisible ? « Annuler », qui a le focus, est en blanc et « Envoyer » en bleu (le style des boutons de tview) : devine-t-on que c'est « Annuler » qui est actif ? Le bandeau rouge en haut se remarque-t-il sans gêner ?")

	const cut = "Le message passe désormais avant le reste de la barre d'état : l'utilisateur s'efface, puis l'URL est raccourcie par « … », puis le cluster. "
	fr, en := i18n.For(i18n.FR), i18n.For(i18n.EN)

	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	shoot(screen, "Après « Annuler »",
		cut+"Le message « "+fr.InfoWriteCancelled+" » est-il clair, et l'URL raccourcie reste-t-elle reconnaissable ?")

	app, screen = newTestAppWith(t, testAppOptions{clusterURL: "https://es-audit.example.test:9200", protection: config.ProtectionReadOnly})
	execute(t, app, screen, requests)
	shoot(screen, "Écriture refusée en « lecture seule », sans libellé",
		"Le bandeau jaune « LECTURE SEULE » est-il lisible ? "+cut+"Le refus, trop long pour 80 colonnes, s'affiche donc seul, coupé par « … ». Son début suffit-il à comprendre ? Texte complet : « "+fmt.Sprintf(fr.ErrReadOnlyWriteFmt, "DELETE", "/logs-2026.10")+" ».")

	app, screen = newTestAppWith(t, testAppOptions{lang: i18n.EN, clusterURL: "https://es-prod.example.test:9200", protection: config.ProtectionConfirm, label: "PROD"})
	execute(t, app, screen, requests)
	shoot(screen, "En anglais : confirmation d'une écriture",
		"Formulation anglaise du titre, du texte et des boutons, et du bandeau (« PROD · writes need confirmation ») ?")

	app, screen = newTestAppWith(t, testAppOptions{lang: i18n.EN, clusterURL: "https://es-audit.example.test:9200", protection: config.ProtectionReadOnly, label: "AUDIT"})
	execute(t, app, screen, requests)
	shoot(screen, "En anglais : écriture refusée en lecture seule, libellé AUDIT",
		"Formulation anglaise du bandeau (« AUDIT · read-only ») et du refus, visible ici en partie ? Texte complet : « "+fmt.Sprintf(en.ErrReadOnlyWriteFmt, "DELETE", "/logs-2026.10")+" » ; et de l'annulation : « "+en.InfoWriteCancelled+" » ?")

	if err := os.MkdirAll(filepath.Dir(screenshotsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(screenshotsFile, []byte(capturesPage(captures)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d captures written to %s", len(captures), screenshotsFile)
}

// connectScreenForCapture starts the connection screen on an 80x24
// simulated screen, with clusters as its history.
func connectScreenForCapture(t *testing.T, clusters []config.Cluster) tcell.SimulationScreen {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.Config{Language: i18n.FR, DefaultTimeoutSeconds: 5, Clusters: clusters}
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(80, 24)
	tapp := tview.NewApplication().SetScreen(screen)
	tapp.SetRoot(BuildConnectPage(tapp, cfg, func(ConnectResult) {}), true)
	startTestApplication(t, tapp, screen)
	waitForDraw(t, screen)
	return screen
}

// renderScreen renders screen as HTML: one span per run of cells of the same
// style, the 16 palette colors as CSS variables the page defines (a terminal
// decides what they look like), the cursor as a block.
func renderScreen(screen tcell.SimulationScreen) string {
	cells, width, height := screenContents(screen)
	cursorX, cursorY, cursorVisible := screen.GetCursor()
	var b strings.Builder
	open := ""
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cell := cells[y*width+x]
			fg, bg, attr := cell.Style.Decompose()
			color, background := cssColor(fg, "--fg"), cssColor(bg, "--bg")
			if attr&tcell.AttrReverse != 0 {
				color, background = background, color
			}
			if cursorVisible && x == cursorX && y == cursorY {
				color, background = background, color
			}
			style := "color:" + color + ";background:" + background
			if attr&tcell.AttrBold != 0 {
				style += ";font-weight:bold"
			}
			if attr&tcell.AttrUnderline != 0 {
				style += ";text-decoration:underline"
			}
			if attr&tcell.AttrDim != 0 {
				style += ";opacity:.6"
			}
			if style != open {
				if open != "" {
					b.WriteString("</span>")
				}
				fmt.Fprintf(&b, `<span style="%s">`, style)
				open = style
			}
			text := string(cell.Runes)
			if strings.TrimRight(text, "\x00") == "" {
				text = " "
			}
			b.WriteString(html.EscapeString(text))
		}
		if y < height-1 {
			b.WriteString("\n")
		}
	}
	if open != "" {
		b.WriteString("</span>")
	}
	return b.String()
}

func cssColor(c tcell.Color, defaultVar string) string {
	if c == tcell.ColorDefault {
		return "var(" + defaultVar + ")"
	}
	for i := 0; i < 16; i++ {
		if c == tcell.PaletteColor(i) {
			return fmt.Sprintf("var(--p%d)", i)
		}
	}
	return fmt.Sprintf("#%06x", c.Hex())
}

func capturesPage(captures []capture) string {
	var b strings.Builder
	b.WriteString(`<!doctype html>
<html lang="fr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Recette visuelle — protection</title>
<style>
body { background: #1b1b1b; color: #e6e6e6; font: 15px/1.5 system-ui, sans-serif; margin: 0 auto; max-width: 860px; padding: 16px; }
h1 { font-size: 1.4em; } h2 { font-size: 1.1em; margin: 2em 0 .3em; }
.check { margin: 0 0 .6em; }
.answer { color: #9ad; }
pre { font: 14px/1.18 "Cascadia Mono", Consolas, "DejaVu Sans Mono", monospace; display: inline-block; margin: 0; padding: 6px; border: 1px solid #444; overflow-x: auto; max-width: 100%; }
.campbell pre { --fg:#cccccc; --bg:#0c0c0c; --p0:#0c0c0c; --p1:#c50f1f; --p2:#13a10e; --p3:#c19c00; --p4:#0037da; --p5:#881798; --p6:#3a96dd; --p7:#cccccc; --p8:#767676; --p9:#e74856; --p10:#16c60c; --p11:#f9f1a5; --p12:#3b78ff; --p13:#b4009e; --p14:#61d6d6; --p15:#f2f2f2; }
.xterm pre { --fg:#e5e5e5; --bg:#000000; --p0:#000000; --p1:#cd0000; --p2:#00cd00; --p3:#cdcd00; --p4:#0000ee; --p5:#cd00cd; --p6:#00cdcd; --p7:#e5e5e5; --p8:#7f7f7f; --p9:#ff0000; --p10:#00ff00; --p11:#ffff00; --p12:#5c5cff; --p13:#ff00ff; --p14:#00ffff; --p15:#ffffff; }
pre { background: var(--bg); }
</style>
</head>
<body class="campbell">
<h1>Recette visuelle — protection par cluster</h1>
<p>`)
	fmt.Fprintf(&b, "%d captures de l'écran 80×24, environ 10 minutes. ", len(captures))
	b.WriteString(`Chaque capture est l'écran simulé que pilotent les tests, cellule par cellule : texte, mise en page et couleurs de chaque case sont exactement ceux que l'application dessine. Seul le rendu des 16 couleurs de base dépend du terminal :
<label>palette <select onchange="document.body.className=this.value">
<option value="campbell">Windows Terminal (Campbell)</option>
<option value="xterm">xterm</option>
</select></label>. L'utilisateur « test » de la barre d'état vient du harnais de test.</p>
<p class="answer">Réponse attendue : un OK ou KO par numéro, par exemple « 1 OK, 2 KO : libellé trop discret, 3 OK… ».</p>
`)
	for i, c := range captures {
		fmt.Fprintf(&b, "<h2>%d. %s</h2>\n<p class=\"check\">%s</p>\n<pre>%s</pre>\n", i+1, html.EscapeString(c.title), html.EscapeString(c.check), c.screen)
	}
	b.WriteString("</body>\n</html>\n")
	return b.String()
}
