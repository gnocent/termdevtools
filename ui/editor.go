package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rivo/tview"
)

// Editor est le panneau gauche : l'éditeur de requêtes. Voir SPEC.md §3.2.
type Editor struct {
	view *tview.TextArea
}

// NewEditor crée un éditeur vide.
func NewEditor() *Editor {
	area := tview.NewTextArea().SetWrap(true)
	area.SetBorder(true).SetTitle(" Requêtes ")
	return &Editor{view: area}
}

// Widget renvoie le composant tview à insérer dans le layout.
func (e *Editor) Widget() tview.Primitive {
	return e.view
}

// Primitive renvoie le TextArea sous-jacent (pour SetFocus, comparaisons de focus...).
func (e *Editor) Primitive() tview.Primitive {
	return e.view
}

// Text renvoie le contenu intégral de l'éditeur.
func (e *Editor) Text() string {
	return e.view.GetText()
}

// CursorOffset renvoie la position du curseur en décalage absolu (runes)
// dans le texte complet. Contrairement à TextArea.GetCursor (dont la
// "ligne" devient une ligne d'affichage, pas une ligne logique, dès que le
// retour à la ligne automatique est actif — cf. SPEC.md §3.1), GetSelection
// renvoie une position indépendante du rendu visuel : "si aucune sélection,
// start et end valent la position du curseur" (doc officielle de tview).
// En cas de sélection active, renvoie l'extrémité la plus proche de la
// borne mouvante — sans incidence en pratique puisque Ctrl+Entrée et Tab ne
// s'en servent qu'en l'absence de sélection.
func (e *Editor) CursorOffset() int {
	_, _, end := e.view.GetSelection()
	return end
}

// lineColAt renvoie la ligne logique (0-indexée, séparée par de vrais "\n")
// et la colonne (en runes) correspondant à l'offset absolu offset dans
// text. Purement textuel, donc indépendant du retour à la ligne visuel.
func lineColAt(text string, offset int) (row, col int) {
	lines := strings.Split(text, "\n")
	consumed := 0
	for i, line := range lines {
		lineLen := len([]rune(line))
		if offset <= consumed+lineLen {
			return i, offset - consumed
		}
		consumed += lineLen + 1 // +1 pour le "\n" séparateur
	}
	last := len(lines) - 1
	return last, len([]rune(lines[last]))
}

// CursorLine renvoie l'index (0-indexé) de la ligne logique où se trouve le
// curseur — la ligne du texte réel, pas la ligne d'affichage après retour
// à la ligne automatique (SetWrap, cf. SPEC.md §3.1).
func (e *Editor) CursorLine() int {
	row, _ := lineColAt(e.Text(), e.CursorOffset())
	return row
}

// LoadFile charge path dans l'éditeur s'il existe et renvoie true si un
// contenu a effectivement été chargé. Son absence n'est pas une erreur —
// utilisé aussi bien pour la cheatsheet que pour la sauvegarde personnelle
// (Ctrl+S), toutes deux optionnelles (cf. SPEC.md §3.2 et §9.1).
func (e *Editor) LoadFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	e.view.SetText(string(data), false)
	return true, nil
}

// SaveToFile écrit l'intégralité du contenu de l'éditeur dans path,
// créant le dossier parent si besoin (Ctrl+S, cf. SPEC.md §3.2).
func (e *Editor) SaveToFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(e.Text()), 0o600)
}

// SelectRange positionne le curseur/la sélection entre les offsets (en
// runes) start et end du texte complet — utilisé par la recherche (Ctrl+F).
func (e *Editor) SelectRange(start, end int) {
	e.view.Select(start, end)
}

// FindNext cherche la prochaine occurrence (insensible à la casse) de query
// après l'offset in-rune after, avec retour au début si nécessaire. Renvoie
// (start, end, true) si trouvé.
func (e *Editor) FindNext(query string, after int) (int, int, bool) {
	return findNext(e.Text(), query, after)
}

// completionLineRe détecte une ligne "MÉTHODE chemin_partiel" en cours de
// frappe, jusqu'au curseur (pas de fin de ligne ancrée : ce qui suit le
// curseur, s'il y a quelque chose, n'entre pas en jeu).
var completionLineRe = regexp.MustCompile(`(?i)^(GET|POST|PUT|DELETE)[ \t]+(\S*)$`)

// CompletionPrefix renvoie, si le curseur est en train de taper un endpoint
// (ligne "MÉTHODE chemin_partiel", cf. SPEC.md §3.2), le préfixe à
// compléter ainsi que ses bornes (en runes, texte complet — utilisables
// avec ApplyCompletion). ok=false en dehors de ce contexte (ex. dans un
// corps JSON), auquel cas Tab garde son comportement standard (insérer une
// tabulation).
func (e *Editor) CompletionPrefix() (prefix string, start, end int, ok bool) {
	text := e.Text()
	offset := e.CursorOffset()
	lines := strings.Split(text, "\n")

	row, col := lineColAt(text, offset)
	if row < 0 || row >= len(lines) {
		return "", 0, 0, false
	}

	lineRunes := []rune(lines[row])
	if col < 0 {
		col = 0
	}
	if col > len(lineRunes) {
		col = len(lineRunes)
	}

	m := completionLineRe.FindStringSubmatch(string(lineRunes[:col]))
	if m == nil {
		return "", 0, 0, false
	}
	prefix = m[2]

	lineStart := offset - col // décalage absolu du début de la ligne logique
	end = lineStart + col
	start = end - len([]rune(prefix))
	return prefix, start, end, true
}

// ApplyCompletion remplace le texte entre start et end (offsets en runes,
// cf. CompletionPrefix) par replacement, et place le curseur juste après.
func (e *Editor) ApplyCompletion(start, end int, replacement string) {
	e.view.Replace(start, end, replacement)
	newPos := start + len([]rune(replacement))
	e.view.Select(newPos, newPos)
}
