// Package i18n provides the message catalogs (French, English) for
// TermDevTools' user interface, selected via config.Config.Language. See
// SPEC.md §3 for the screens this covers.
package i18n

import "strings"

const (
	FR = "fr"
	EN = "en"
)

// Strings is the full set of user-facing text in the interface. Both the fr
// and en catalogs below are values of this same struct, so a field added to
// one and forgotten in the other is a compile error in whichever file still
// misses it once code references the field — not a silently missing
// translation at runtime.
type Strings struct {
	// Connect screen (connect.go)
	ConnectListTitle           string
	NewConnectionLabel         string
	NewConnectionSecondary     string
	QuitLabel                  string
	NewConnectionFormTitle     string
	ConnectFormTitleFmt        string // " Connecting to %s "
	AuthFieldLabel             string
	AuthNone                   string
	AuthBasic                  string
	AuthAPIKey                 string
	AuthMTLS                   string
	FieldURL                   string
	FieldURLReadOnly           string
	FieldUsername              string
	FieldPassword              string
	FieldAPIKeyID              string
	FieldAPIKeySecret          string
	FieldClientCert            string
	FieldClientKey             string
	FieldKeyPassphrase         string
	FieldCAFile                string
	FieldVerifyTLS             string
	ButtonConnect              string
	ButtonCancel               string
	ErrURLRequired             string
	StatusConnecting           string
	ErrConnectFailedFmt        string // "Échec de connexion : %s"
	ErrClusterHTTPFmt          string // "Le cluster a répondu HTTP %d"
	WarnConnectedSaveFailedFmt string // "Connecté, mais échec de sauvegarde de config.yaml : %s"
	DisplayUserNoAuth          string

	// Main layout (app.go, editor.go, result.go)
	EditorTitle     string
	ResultTitle     string
	CompletionTitle string
	HelpViewTitle   string
	SearchLabel     string

	// Status bar (statusbar.go)
	StatusBarTemplate string // "[white]Cluster: [green]%s[white]  |  ...: [green]%s[white]  |  [%s]%s[white]"
	StatusIdle        string
	StatusRunning     string
	StatusResultFmt   string // "HTTP %d en %s"
	ShortcutsHelpBar  string

	// App-level status/error messages (app.go)
	ErrLoadFailedFmt   string // "échec du chargement de %s : %s"
	ErrNoMatchFound    string
	ErrNoCompletion    string
	ErrSaveFailedFmt   string
	InfoSavedFmt       string
	ErrExportFailedFmt string
	InfoExportedFmt    string
	ErrNothingToCopy   string
	InfoCopied         string
	ErrNothingToExport string // result.go's Export()

	// Help popup content (F1)
	HelpContent string

	// Startup errors shown before the UI is up (main.go)
	ErrConfigLoadFmt             string
	ErrExecDirFmt                string
	ErrFatalFmt                  string
	ErrCrashedFmt                string // "%v" — the recovered panic value
	InfoCrashReportFmt           string // "%s" — path to the written crash log
	ErrCrashReportWriteFailedFmt string // "%s" — the write error

	// Language switch (F3, app.go)
	LanguageName            string // this catalog's own language, in its own language ("Français"/"English")
	InfoLanguageSwitchedFmt string
}

var fr = Strings{
	ConnectListTitle:           " TermDevTools — connexion ",
	NewConnectionLabel:         "+ Nouvelle connexion",
	NewConnectionSecondary:     "saisir une nouvelle connexion",
	QuitLabel:                  "Quitter",
	NewConnectionFormTitle:     " Nouvelle connexion ",
	ConnectFormTitleFmt:        " Connexion à %s ",
	AuthFieldLabel:             "Authentification",
	AuthNone:                   "aucune",
	AuthBasic:                  "Basic Auth",
	AuthAPIKey:                 "API Key",
	AuthMTLS:                   "certificat client (mTLS)",
	FieldURL:                   "URL (https://host:port)",
	FieldURLReadOnly:           "URL",
	FieldUsername:              "Username",
	FieldPassword:              "Mot de passe",
	FieldAPIKeyID:              "API Key ID",
	FieldAPIKeySecret:          "API Key secret",
	FieldClientCert:            "Certificat client",
	FieldClientKey:             "Clé privée client",
	FieldKeyPassphrase:         "Passphrase de la clé (si chiffrée)",
	FieldCAFile:                "Fichier CA",
	FieldVerifyTLS:             "Vérifier le certificat serveur (TLS)",
	ButtonConnect:              "Se connecter",
	ButtonCancel:               "Annuler",
	ErrURLRequired:             "L'URL est obligatoire.",
	StatusConnecting:           "Connexion en cours...",
	ErrConnectFailedFmt:        "Échec de connexion : %s",
	ErrClusterHTTPFmt:          "Le cluster a répondu HTTP %d",
	WarnConnectedSaveFailedFmt: "Connecté, mais échec de sauvegarde de config.yaml : %s",
	DisplayUserNoAuth:          "(aucune auth)",

	EditorTitle:     " Requêtes ",
	ResultTitle:     " Résultat ",
	CompletionTitle: " Compléter (Entrée, Echap pour annuler) ",
	HelpViewTitle:   " Aide (Echap pour fermer) ",
	SearchLabel:     "Rechercher : ",

	StatusBarTemplate: "[white]Cluster: [green]%s[white]  |  Utilisateur: [green]%s[white]  |  [%s]%s[white]",
	StatusIdle:        "prêt",
	StatusRunning:     "requête en cours...",
	StatusResultFmt:   "HTTP %d en %s",
	// Deliberate order: the most essential commands (help, language, quit)
	// come first, so they stay visible even on a narrow terminal where the
	// end of the line gets cut off (only 1 row is allocated for this bar).
	// "Ctrl(/Opt)" flags the shortcuts where Option also works, on macOS,
	// as an alternative to Ctrl (see HelpContent for why and its limits).
	ShortcutsHelpBar: "[gray]F1[white] aide   [gray]F3[white] langue   [gray]Ctrl+C[white] quitter   " +
		"[gray]Ctrl+E[white] exécuter   [gray]Tab/F10[white] compléter   [gray]Ctrl(/Opt)+←/→[white] changer de panneau   " +
		"[gray]F5/F6[white] redimensionner   [gray]Ctrl+F[white] rechercher   [gray]Ctrl+S[white] sauvegarder/exporter   " +
		"[gray]F2[white] copier",

	ErrLoadFailedFmt:   "échec du chargement de %s : %s",
	ErrNoMatchFound:    "aucune occurrence trouvée",
	ErrNoCompletion:    "aucune complétion",
	ErrSaveFailedFmt:   "échec de sauvegarde : %s",
	InfoSavedFmt:       "requêtes sauvegardées dans %s",
	ErrExportFailedFmt: "échec d'export : %s",
	InfoExportedFmt:    "résultat exporté dans %s",
	ErrNothingToCopy:   "aucun résultat à copier",
	InfoCopied:         "résultat copié (OSC 52 — nécessite un terminal compatible)",
	ErrNothingToExport: "aucun résultat à exporter",

	HelpContent: `[yellow]TermDevTools[white] — client Elasticsearch en mode terminal

[green]Panneau gauche[white] : requêtes "MÉTHODE endpoint" + JSON optionnel
sur les lignes suivantes. Lignes [gray]#[white] = commentaires.

[green]Panneau droit[white] : résultat de la dernière requête — JSON coloré,
ou texte brut (ex. réponses _cat/*).

[yellow]Raccourcis clavier[white]
[gray]PuTTY est connu pour mal gérer certains raccourcis : fortement
déconseillé. Aucun souci sous macOS/Windows 10+ natifs. En cas de
doute, préférez Ctrl+E, F5/F6 et Tab/F10.[white]

  [aqua]Ctrl+E[white]           Exécuter la requête sous le curseur
                     (aussi : Ctrl+Entrée, Option/Alt+Entrée sur macOS — non garanti partout)
  [aqua]Tab/F10[white]          Compléter un endpoint en cours de frappe
                     (aussi : Ctrl+Espace)
  [aqua]Ctrl+←/→[white]         Changer de panneau
                     (aussi : Option/Alt+←/→ sur macOS)
  [aqua]F5/F6[white]            Redimensionner le split gauche/droite
                     (aussi : Ctrl+Maj+←/→, Option/Alt+Maj+←/→ sur macOS — non garanti partout)
  [aqua]Ctrl+F[white]           Rechercher dans le panneau actif
  [aqua]Ctrl+S[white]           Sauvegarder (gauche) / exporter (droite)
  [aqua]F2[white]               Copier le résultat (panneau droit) dans le presse-papier
  [aqua]F1[white]               Afficher cette aide
  [aqua]F3[white]               Changer la langue de l'interface (fr/en)
  [aqua]Ctrl+C[white]           Quitter (sauvegarde automatiquement le panneau gauche)

[yellow]Fichiers[white]

  [aqua]~/.config/termdevtools/[white]  (personnel)
    config.yaml     clusters connus
    queries_*.txt   sauvegarde par cluster (Ctrl+S)
  [aqua]<dossier du binaire>/[white]  (équipe, partagé)
    cheatsheet.txt  requêtes par défaut de l'éditeur
    endpoints.txt   liste proposée par la complétion Tab
    exports/        résultats exportés (Ctrl+S)

[gray]Echap pour fermer cette aide.[white]`,

	ErrConfigLoadFmt:             "Erreur de chargement de config.yaml : %s",
	ErrExecDirFmt:                "Impossible de déterminer le dossier de l'exécutable : %s",
	ErrFatalFmt:                  "Erreur fatale : %s",
	ErrCrashedFmt:                "TermDevTools a planté : %v",
	InfoCrashReportFmt:           "Rapport complet écrit dans : %s",
	ErrCrashReportWriteFailedFmt: "Impossible d'écrire le rapport de plantage : %s",

	LanguageName:            "Français",
	InfoLanguageSwitchedFmt: "Langue : %s",
}

var en = Strings{
	ConnectListTitle:           " TermDevTools — connect ",
	NewConnectionLabel:         "+ New connection",
	NewConnectionSecondary:     "enter a new connection",
	QuitLabel:                  "Quit",
	NewConnectionFormTitle:     " New connection ",
	ConnectFormTitleFmt:        " Connecting to %s ",
	AuthFieldLabel:             "Authentication",
	AuthNone:                   "none",
	AuthBasic:                  "Basic Auth",
	AuthAPIKey:                 "API Key",
	AuthMTLS:                   "client certificate (mTLS)",
	FieldURL:                   "URL (https://host:port)",
	FieldURLReadOnly:           "URL",
	FieldUsername:              "Username",
	FieldPassword:              "Password",
	FieldAPIKeyID:              "API Key ID",
	FieldAPIKeySecret:          "API Key secret",
	FieldClientCert:            "Client certificate",
	FieldClientKey:             "Client private key",
	FieldKeyPassphrase:         "Key passphrase (if encrypted)",
	FieldCAFile:                "CA file",
	FieldVerifyTLS:             "Verify server certificate (TLS)",
	ButtonConnect:              "Connect",
	ButtonCancel:               "Cancel",
	ErrURLRequired:             "The URL is required.",
	StatusConnecting:           "Connecting...",
	ErrConnectFailedFmt:        "Connection failed: %s",
	ErrClusterHTTPFmt:          "The cluster responded HTTP %d",
	WarnConnectedSaveFailedFmt: "Connected, but failed to save config.yaml: %s",
	DisplayUserNoAuth:          "(no auth)",

	EditorTitle:     " Requests ",
	ResultTitle:     " Result ",
	CompletionTitle: " Complete (Enter, Esc to cancel) ",
	HelpViewTitle:   " Help (Esc to close) ",
	SearchLabel:     "Search: ",

	StatusBarTemplate: "[white]Cluster: [green]%s[white]  |  User: [green]%s[white]  |  [%s]%s[white]",
	StatusIdle:        "ready",
	StatusRunning:     "request in progress...",
	StatusResultFmt:   "HTTP %d in %s",
	// Deliberate order: the most essential commands (help, language, quit)
	// come first, so they stay visible even on a narrow terminal where the
	// end of the line gets cut off (only 1 row is allocated for this bar).
	// "Ctrl(/Opt)" flags the shortcuts where Option also works, on macOS,
	// as an alternative to Ctrl (see HelpContent for why and its limits).
	ShortcutsHelpBar: "[gray]F1[white] help   [gray]F3[white] language   [gray]Ctrl+C[white] quit   " +
		"[gray]Ctrl+E[white] execute   [gray]Tab/F10[white] complete   [gray]Ctrl(/Opt)+←/→[white] switch panel   " +
		"[gray]F5/F6[white] resize   [gray]Ctrl+F[white] search   [gray]Ctrl+S[white] save/export   " +
		"[gray]F2[white] copy",

	ErrLoadFailedFmt:   "failed to load %s: %s",
	ErrNoMatchFound:    "no match found",
	ErrNoCompletion:    "no completion",
	ErrSaveFailedFmt:   "save failed: %s",
	InfoSavedFmt:       "requests saved to %s",
	ErrExportFailedFmt: "export failed: %s",
	InfoExportedFmt:    "result exported to %s",
	ErrNothingToCopy:   "nothing to copy",
	InfoCopied:         "result copied (OSC 52 — requires a compatible terminal)",
	ErrNothingToExport: "nothing to export",

	HelpContent: `[yellow]TermDevTools[white] — terminal-mode Elasticsearch client

[green]Left panel[white]: "METHOD endpoint" requests + optional JSON
on the following lines. Lines starting with [gray]#[white] = comments.

[green]Right panel[white]: result of the last request — colorized JSON,
or plain text (e.g. _cat/* responses).

[yellow]Keyboard shortcuts[white]
[gray]PuTTY is known to mishandle some shortcuts: strongly discouraged.
No issues on native macOS/Windows 10+ terminals. When in doubt, prefer
Ctrl+E, F5/F6, and Tab/F10.[white]

  [aqua]Ctrl+E[white]           Execute the request under the cursor
                     (also: Ctrl+Enter, Option/Alt+Enter on macOS — not guaranteed everywhere)
  [aqua]Tab/F10[white]          Complete an endpoint while typing
                     (also: Ctrl+Space)
  [aqua]Ctrl+←/→[white]         Switch panel
                     (also: Option/Alt+←/→ on macOS)
  [aqua]F5/F6[white]            Resize the left/right split
                     (also: Ctrl+Shift+←/→, Option/Alt+Shift+←/→ on macOS — not guaranteed everywhere)
  [aqua]Ctrl+F[white]           Search in the active panel
  [aqua]Ctrl+S[white]           Save (left) / export (right)
  [aqua]F2[white]               Copy the result (right panel) to the clipboard
  [aqua]F1[white]               Show this help
  [aqua]F3[white]               Switch the interface language (fr/en)
  [aqua]Ctrl+C[white]           Quit (auto-saves the left panel)

[yellow]Files[white]

  [aqua]~/.config/termdevtools/[white]  (personal)
    config.yaml     known clusters
    queries_*.txt   per-cluster save (Ctrl+S)
  [aqua]<binary's directory>/[white]  (team, shared)
    cheatsheet.txt  editor's default requests
    endpoints.txt   list offered by Tab completion
    exports/        exported results (Ctrl+S)

[gray]Esc to close this help.[white]`,

	ErrConfigLoadFmt:             "Error loading config.yaml: %s",
	ErrExecDirFmt:                "Could not determine the executable's directory: %s",
	ErrFatalFmt:                  "Fatal error: %s",
	ErrCrashedFmt:                "TermDevTools crashed: %v",
	InfoCrashReportFmt:           "Full report written to: %s",
	ErrCrashReportWriteFailedFmt: "Could not write crash report: %s",

	LanguageName:            "English",
	InfoLanguageSwitchedFmt: "Language: %s",
}

// Normalize maps lang ("fr"/"en", case-insensitive, possibly padded with
// whitespace) to exactly FR or EN, defaulting to FR for anything else
// (empty, unrecognized) — preserves the interface's original language for
// configs predating this setting.
func Normalize(lang string) string {
	if strings.ToLower(strings.TrimSpace(lang)) == EN {
		return EN
	}
	return FR
}

// For returns the message catalog for lang (see Normalize).
func For(lang string) *Strings {
	if Normalize(lang) == EN {
		return &en
	}
	return &fr
}
