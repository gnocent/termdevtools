package ui

import (
	"fmt"
	"time"

	"github.com/rivo/tview"
)

const helpText = "[gray]Ctrl+Entrée[white] exécuter   [gray]Tab[white] compléter   [gray]Ctrl+←/→[white] changer de panneau   [gray]Ctrl+Maj+←/→[white] redimensionner   [gray]Ctrl+F[white] rechercher   [gray]Ctrl+S[white] sauvegarder/exporter   [gray]F2[white] copier   [gray]F1[white] aide   [gray]Ctrl+C[white] quitter"

// StatusBar affiche l'état courant (cluster, requête en cours, résultat) et
// une barre d'aide rappelant les raccourcis. Voir SPEC.md §3.1 et §4.
type StatusBar struct {
	cluster string
	user    string

	status *tview.TextView
	help   *tview.TextView
}

// NewStatusBar crée la barre de statut pour le cluster/utilisateur donnés.
func NewStatusBar(cluster, user string) *StatusBar {
	sb := &StatusBar{
		cluster: cluster,
		user:    user,
		status:  tview.NewTextView().SetDynamicColors(true),
		help:    tview.NewTextView().SetDynamicColors(true).SetText(helpText),
	}
	sb.SetIdle()
	return sb
}

// Widget renvoie le conteneur (statut + aide) à insérer dans le layout.
func (sb *StatusBar) Widget() tview.Primitive {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(sb.status, 1, 0, false).
		AddItem(sb.help, 1, 0, false)
}

func (sb *StatusBar) render(message, color string) {
	sb.status.SetText(fmt.Sprintf(
		"[white]Cluster: [green]%s[white]  |  Utilisateur: [green]%s[white]  |  [%s]%s[white]",
		tview.Escape(sb.cluster), tview.Escape(sb.user), color, tview.Escape(message),
	))
}

// SetIdle affiche l'état "prêt" par défaut.
func (sb *StatusBar) SetIdle() {
	sb.render("prêt", "white")
}

// SetRunning indique qu'une requête est en cours d'exécution.
func (sb *StatusBar) SetRunning() {
	sb.render("requête en cours...", "yellow")
}

// SetInfo affiche une confirmation (ex. sauvegarde/export réussis).
func (sb *StatusBar) SetInfo(message string) {
	sb.render(message, "green")
}

// SetResult affiche le résultat d'une requête terminée.
func (sb *StatusBar) SetResult(statusCode int, duration time.Duration) {
	color := "green"
	if statusCode >= 400 || statusCode == 0 {
		color = "red"
	}
	sb.render(fmt.Sprintf("HTTP %d en %s", statusCode, duration.Round(time.Millisecond)), color)
}

// SetError affiche un message d'erreur (requête invalide, cluster injoignable...).
func (sb *StatusBar) SetError(message string) {
	sb.render(message, "red")
}
