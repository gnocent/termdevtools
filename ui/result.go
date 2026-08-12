package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/rivo/tview"
)

// searchRegionID est l'unique région tview utilisée pour surligner/scroller
// vers l'occurrence courante de la recherche (Ctrl+F, cf. HighlightLine) —
// un seul résultat mis en évidence à la fois, pas besoin d'IDs distincts.
const searchRegionID = "match"

// ResultView est le panneau droit : le résultat de la dernière requête.
// JSON prettifié et coloré si la réponse est du JSON valide, texte brut
// sinon (ex. réponses `_cat`). Voir SPEC.md §3.3.
type ResultView struct {
	view          *tview.TextView
	plain         string // texte affiché sans les tags de couleur, pour la recherche et l'export
	displayedText string // dernier texte réellement envoyé à SetText (avec tags de couleur), pour HighlightLine
	isJSON        bool
}

// NewResultView crée un panneau résultat vide.
func NewResultView() *ResultView {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetScrollable(true).
		SetRegions(true)
	view.SetBorder(true).SetTitle(" Résultat ")
	return &ResultView{view: view}
}

// Widget renvoie le composant tview à insérer dans le layout.
func (r *ResultView) Widget() tview.Primitive {
	return r.view
}

// Primitive renvoie le TextView sous-jacent.
func (r *ResultView) Primitive() tview.Primitive {
	return r.view
}

// PlainText renvoie le contenu actuellement affiché, sans les tags de
// couleur — utilisé pour l'export (Ctrl+S) et la copie presse-papier (F2).
func (r *ResultView) PlainText() string {
	return r.plain
}

// Clear vide le panneau (état initial avant toute exécution).
func (r *ResultView) Clear() {
	r.plain = ""
	r.displayedText = ""
	r.isJSON = false
	r.view.Clear()
	r.view.ScrollToBeginning()
}

// Show affiche le corps de réponse body : JSON prettifié et coloré s'il est
// valide, texte brut (police fixe) sinon.
func (r *ResultView) Show(body []byte) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err == nil {
		r.plain = buf.String()
		r.isJSON = true
		r.displayedText = colorizeJSON(r.plain)
	} else {
		r.plain = string(body)
		r.isJSON = false
		r.displayedText = tview.Escape(r.plain)
	}
	r.view.SetText(r.displayedText)
	r.view.ScrollToBeginning()
}

// ShowError affiche un message d'erreur en rouge.
func (r *ResultView) ShowError(message string) {
	r.plain = message
	r.isJSON = false
	r.displayedText = "[red]" + tview.Escape(message) + "[white]"
	r.view.SetText(r.displayedText)
	r.view.ScrollToBeginning()
}

// Export écrit le résultat actuellement affiché dans un fichier horodaté du
// dossier dir (créé si besoin), en .json si c'est du JSON valide, .txt
// sinon. Renvoie le chemin du fichier créé. Voir SPEC.md §3.3 et §9.1.
func (r *ResultView) Export(dir string) (string, error) {
	if r.plain == "" {
		return "", errors.New("aucun résultat à exporter")
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	ext := ".txt"
	if r.isJSON {
		ext = ".json"
	}
	path := filepath.Join(dir, time.Now().Format("20060102-150405")+ext)

	if err := os.WriteFile(path, []byte(r.plain), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// HighlightLine positionne le panneau sur la ligne donnée (0-indexée),
// utilisé par la recherche (Ctrl+F). Passe par le mécanisme de régions de
// tview (ancré au contenu) plutôt que TextView.ScrollTo(ligne, ...), dont
// le numéro de ligne redevient une ligne d'affichage — pas logique — dès
// que le retour à la ligne automatique est actif (SPEC.md §3.1) : un
// simple ScrollTo viserait le mauvais endroit dès qu'une ligne précédente
// aurait débordé sur plusieurs lignes affichées.
func (r *ResultView) HighlightLine(line int) {
	lines := strings.Split(r.displayedText, "\n")
	if line < 0 || line >= len(lines) {
		return
	}

	// Les tokens JSON ne contiennent jamais de "\n" (colorizeJSON ne fait
	// que reformater des tokens, jamais insérer/retirer de saut de ligne),
	// donc displayedText a exactement le même découpage en lignes que
	// plain : envelopper la ligne ciblée d'une région est sûr.
	tagged := make([]string, len(lines))
	copy(tagged, lines)
	tagged[line] = `["` + searchRegionID + `"]` + tagged[line] + `[""]`

	r.view.SetText(strings.Join(tagged, "\n"))
	r.view.Highlight(searchRegionID)
	r.view.ScrollToHighlight()
}

// FindNext cherche la prochaine occurrence (insensible à la casse) de query
// dans le texte brut affiché, à partir de la ligne afterLine (exclue).
// Renvoie le numéro de ligne du prochain résultat, avec retour au début.
func (r *ResultView) FindNext(query string, afterLine int) (int, bool) {
	if query == "" || r.plain == "" {
		return 0, false
	}
	lines := strings.Split(r.plain, "\n")
	lowerQuery := strings.ToLower(query)

	search := func(from, to int) (int, bool) {
		for i := from; i < to; i++ {
			if strings.Contains(strings.ToLower(lines[i]), lowerQuery) {
				return i, true
			}
		}
		return 0, false
	}

	if line, ok := search(afterLine+1, len(lines)); ok {
		return line, true
	}
	return search(0, afterLine+1)
}

var jsonTokenRe = regexp.MustCompile(`"(?:\\.|[^"\\])*"|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|true|false|null|[{}\[\],:]`)

// colorizeJSON colore un texte JSON déjà indenté (json.Indent) en tags de
// couleur tview. Toute construction manuelle de tag n'est possible que si
// les crochets "[" du contenu littéral sont échappés (convention tview).
func colorizeJSON(text string) string {
	var b strings.Builder
	last := 0
	for _, loc := range jsonTokenRe.FindAllStringIndex(text, -1) {
		start, end := loc[0], loc[1]
		b.WriteString(tview.Escape(text[last:start]))
		tok := text[start:end]

		switch {
		case strings.HasPrefix(tok, `"`):
			if isKeyToken(text, end) {
				b.WriteString("[#5fafff]")
			} else {
				b.WriteString("[#87d787]")
			}
			b.WriteString(tview.Escape(tok))
			b.WriteString("[white]")
		case len(tok) == 1 && strings.ContainsRune("{}[],:", rune(tok[0])):
			b.WriteString(tview.Escape(tok))
		case tok == "true" || tok == "false":
			b.WriteString("[#d78700]")
			b.WriteString(tok)
			b.WriteString("[white]")
		case tok == "null":
			b.WriteString("[gray]")
			b.WriteString(tok)
			b.WriteString("[white]")
		default: // nombre
			b.WriteString("[#af87ff]")
			b.WriteString(tok)
			b.WriteString("[white]")
		}
		last = end
	}
	b.WriteString(tview.Escape(text[last:]))
	return b.String()
}

// isKeyToken indique si la chaîne se terminant à pos est suivie (après
// d'éventuels espaces) d'un ':', auquel cas il s'agit d'une clé JSON.
func isKeyToken(text string, pos int) bool {
	for pos < len(text) {
		r := rune(text[pos])
		if unicode.IsSpace(r) {
			pos++
			continue
		}
		return text[pos] == ':'
	}
	return false
}
