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
// and en catalogs below are values of this same struct. The compiler doesn't
// notice a field given a value in one and forgotten in the other — it is
// simply empty there: the package's tests do, along with format verbs that
// differ between the two.
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
	ErrURLCredentials          string // a user:password@ part in the URL, which is saved and displayed as is
	StatusConnecting           string
	ErrConnectFailedFmt        string // "%s" — why the connection failed
	ErrClusterHTTPFmt          string // "%d" — the HTTP status the cluster answered with
	WarnConnectedSaveFailedFmt string // "%s" — why config.yaml couldn't be saved, though connected
	WarnTargetOverrideFmt      string // "%s" — why config.yaml's distribution/version override was ignored
	DisplayUserNoAuth          string

	// Certificate picker popup (connect.go): Enter on the CA/client-cert/
	// client-key fields browses the configured default_ca_dir/
	// default_client_cert_dir instead of typing a filename from memory.
	BrowseHint                string
	CertPickerTitleFmt        string
	ErrNoCertDirConfiguredFmt string
	ErrNoCertFilesInDirFmt    string
	ErrCertDirNotFoundFmt     string // "the %s directory doesn't exist"

	// Main layout (app.go, editor.go, result.go)
	EditorTitle     string
	ResultTitle     string
	CompletionTitle string
	HelpViewTitle   string
	SearchLabel     string

	// Status bar (statusbar.go)
	StatusBarTemplate    string // caption, cluster URL, user, message color, message
	StatusClusterCaption string // caption in front of the URL when the cluster's product isn't known
	StatusIdle           string
	StatusRunning        string
	StatusResultFmt      string // "%d", "%s" — HTTP status, duration
	ShortcutsHelpBar     string

	// App-level status/error messages (app.go)
	ErrLoadFailedFmt   string // "%s", "%s" — the file, why it couldn't be loaded
	ErrNoMatchFound    string
	ErrNoCompletion    string
	ErrSaveFailedFmt   string
	InfoSavedFmt       string
	ErrExportFailedFmt string
	InfoExportedFmt    string
	ErrNothingToCopy   string
	InfoCopied         string
	ErrNothingToExport string // result.go's Export()
	ErrNoBodyToFormat  string // F4 (reformat JSON body, SPEC.md §7 backlog #1): nothing under the cursor to reformat
	InfoCurlCopied     string // F9 ("copy as cURL", SPEC.md §7 backlog #3)
	// F4: the body can't be re-indented because of "#" lines in or before it
	ErrFormatCommentsInBody string
	// Ctrl+C: "%s" — why the left panel wasn't saved; the next Ctrl+C quits anyway
	ErrExitSaveFailedFmt string
	// the reason given to ErrExitSaveFailedFmt when the saved requests couldn't be read at startup
	ErrSavedRequestsUnread string

	// Reference data (endpoints, recipes: built in, extended by the team's
	// and user's files — refdata package)
	WarnMoreProblemsFmt   string // "%s (+%d more)" — first problem found in those files, count of the others
	WarnCatColumnsBuiltIn string // _cat columns offered from the built-in table: the cluster couldn't be asked
	InfoReloadedFmt       string // F7: "%d variable(s), %d recipe(s), %d endpoint(s)" reloaded

	// Recipe palette (F8, recipes.go)
	RecipeFilterLabel     string
	RecipesTitleFmt       string // "Recipes (%d)" — how many are listed
	RecipesHint           string // keys reminder shown in the palette's border
	RecipePreviewTitle    string
	RecipesNoMatch        string
	RecipeOwnMark         string // flags a recipe that comes from a team's or user's file
	InfoRecipeInsertedFmt string // "%s" — the recipe's title

	// Reusable variables (${name}, SPEC.md §3.2)
	ErrUnknownVariablesFmt string // "unknown variable(s): %s"

	// Help popup content (F1)
	HelpContent string

	// Startup errors shown before the UI is up (main.go)
	ErrConfigLoadFmt             string
	ErrExecDirFmt                string
	ErrFatalFmt                  string
	ErrCrashedFmt                string // "%v" — the recovered panic value
	InfoCrashReportFmt           string // "%s" — path to the written crash log
	ErrCrashReportWriteFailedFmt string // "%s" — the write error

	// Command line (main.go)
	UsageExportDefaults     string // description of the --export-defaults flag
	UsageVersion            string // description of the --version flag
	InfoExportedDefaultsFmt string // "%d", "%s" — number of files written, directory
	ErrExportDefaultsFmt    string // "%s" — why the export failed
	ErrExportIntoSourceFmt  string // "%s" — the directory, one that reference files are read from

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
	ErrURLCredentials:          "Pas d'identifiants dans l'URL (elle est enregistrée et affichée) : choisissez l'authentification Basic Auth.",
	StatusConnecting:           "Connexion en cours...",
	ErrConnectFailedFmt:        "Échec de connexion : %s",
	ErrClusterHTTPFmt:          "Le cluster a répondu HTTP %d",
	WarnConnectedSaveFailedFmt: "Connecté, mais échec de sauvegarde de config.yaml : %s",
	WarnTargetOverrideFmt:      "config.yaml : distribution/version forcée ignorée (%s)",
	DisplayUserNoAuth:          "(aucune auth)",
	BrowseHint:                 " (Entrée : parcourir)",
	CertPickerTitleFmt:         " Choisir un fichier — %s (Entrée: ouvrir, Retour: dossier parent, Echap: annuler) ",
	ErrNoCertDirConfiguredFmt:  "aucun dossier configuré (%s dans config.yaml)",
	ErrNoCertFilesInDirFmt:     "aucun fichier dans %s",
	ErrCertDirNotFoundFmt:      "le dossier %s n'existe pas",

	EditorTitle:     " Requêtes ",
	ResultTitle:     " Résultat ",
	CompletionTitle: " Compléter (Entrée, Echap pour annuler) ",
	HelpViewTitle:   " Aide (Echap pour fermer) ",
	SearchLabel:     "Rechercher : ",

	StatusBarTemplate:    "[white]%s: [green]%s[white]  |  Utilisateur: [green]%s[white]  |  [%s]%s[white]",
	StatusClusterCaption: "Cluster",
	StatusIdle:           "prêt",
	StatusRunning:        "requête en cours...",
	StatusResultFmt:      "HTTP %d en %s",
	// Deliberate order: the most essential commands (help, language, quit)
	// come first, so they stay visible even on a narrow terminal where the
	// end of the line gets cut off (only 1 row is allocated for this bar).
	// "Ctrl(/Opt)" flags the shortcuts where Option also works, on macOS,
	// as an alternative to Ctrl (see HelpContent for why and its limits).
	ShortcutsHelpBar: "[gray]F1[white] aide   [gray]F3[white] langue   [gray]Ctrl+C[white] quitter   " +
		"[gray]Ctrl+E[white] exécuter   [gray]F8[white] recettes   [gray]Tab/F10[white] compléter   [gray]Ctrl(/Opt)+←/→[white] changer de panneau   " +
		"[gray]F5/F6[white] redimensionner   [gray]Ctrl+F[white] rechercher   [gray]Ctrl+S[white] sauvegarder/exporter   " +
		"[gray]F2[white] copier   [gray]F4[white] reformater le JSON   [gray]F9[white] copier en cURL   [gray]F7[white] recharger vos fichiers",

	ErrLoadFailedFmt:        "échec du chargement de %s : %s",
	ErrNoMatchFound:         "aucune occurrence trouvée",
	ErrNoCompletion:         "aucune complétion",
	ErrSaveFailedFmt:        "échec de sauvegarde : %s",
	InfoSavedFmt:            "requêtes sauvegardées dans %s",
	ErrExportFailedFmt:      "échec d'export : %s",
	InfoExportedFmt:         "résultat exporté dans %s",
	ErrNothingToCopy:        "aucun résultat à copier",
	InfoCopied:              "résultat copié (OSC 52 — nécessite un terminal compatible)",
	ErrNothingToExport:      "aucun résultat à exporter",
	ErrNoBodyToFormat:       "aucun corps JSON à reformater",
	InfoCurlCopied:          "commande cURL copiée (OSC 52 — nécessite un terminal compatible)",
	ErrFormatCommentsInBody: "reformatage impossible : ligne # dans ou avant le corps JSON",
	ErrExitSaveFailedFmt:    "non sauvegardé, Ctrl+C pour quitter quand même : %s",
	ErrSavedRequestsUnread:  "requêtes existantes illisibles au démarrage (Ctrl+S pour les remplacer)",
	WarnMoreProblemsFmt:     "%s (+%d autre(s))",
	WarnCatColumnsBuiltIn:   "colonnes de la table intégrée (cluster muet)",
	InfoReloadedFmt:         "rechargé : %d variable(s), %d recette(s), %d endpoint(s)",
	RecipeFilterLabel:       "Filtrer : ",
	RecipesTitleFmt:         "Recettes (%d)",
	RecipesHint:             "Entrée: insérer, Echap: fermer",
	RecipePreviewTitle:      " Aperçu ",
	RecipesNoMatch:          "aucune recette ne correspond",
	RecipeOwnMark:           "(ajoutée)",
	InfoRecipeInsertedFmt:   "recette insérée : %s",
	ErrUnknownVariablesFmt:  "variable(s) inconnue(s) : %s",

	HelpContent: `[yellow]TermDevTools[white] — client Elasticsearch et OpenSearch en mode terminal

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
  [aqua]F8[white]               Catalogue de recettes : requêtes prêtes à l'emploi,
                     choisies selon la version du cluster (taper pour filtrer)
  [aqua]Tab/F10[white]          Compléter un endpoint ou une colonne _cat en cours de frappe
                     (aussi : Ctrl+Espace)
  [aqua]Ctrl+←/→[white]         Changer de panneau
                     (aussi : Option/Alt+←/→ sur macOS)
  [aqua]F5/F6[white]            Redimensionner le split gauche/droite
                     (aussi : Ctrl+Maj+←/→, Option/Alt+Maj+←/→ sur macOS — non garanti partout)
  [aqua]Ctrl+F[white]           Rechercher dans le panneau actif
  [aqua]Ctrl+S[white]           Sauvegarder (gauche) / exporter (droite)
  [aqua]F4[white]               Reformater (indenter) le JSON de la requête sous le curseur
  [aqua]F9[white]               Copier la requête sous le curseur en commande cURL (secrets non inclus)
  [aqua]F7[white]               Recharger vos fichiers édités à la main : variables ${nom},
                     recettes, endpoints
  [aqua]F2[white]               Copier le résultat (panneau droit) dans le presse-papier
  [aqua]F1[white]               Afficher cette aide
  [aqua]F3[white]               Changer la langue de l'interface (fr/en)
  [aqua]Ctrl+C[white]           Quitter (sauvegarde automatiquement le panneau gauche)

[yellow]Fichiers[white]
[gray]Rien n'est requis à côté du binaire : recettes et endpoints y sont
intégrés. Ces fichiers, tous facultatifs, s'y ajoutent.[white]

  [aqua]~/.config/termdevtools/[white]  (personnel)
    config.yaml      clusters connus
    queries_*.txt    sauvegarde par cluster (Ctrl+S)
    variables_*.txt  variables ${nom} par cluster
    recipes/*.txt    vos recettes
    endpoints.txt    vos endpoints pour la complétion
  [aqua]<dossier du binaire>/[white]  (équipe, partagé)
    recipes/*.txt, endpoints.txt   ceux de l'équipe
    cheatsheet.txt   contenu initial de l'éditeur
    exports/         résultats exportés (Ctrl+S)

[gray]Echap pour fermer cette aide.[white]`,

	ErrConfigLoadFmt:             "Erreur de chargement de config.yaml : %s",
	ErrExecDirFmt:                "Impossible de déterminer le dossier de l'exécutable : %s",
	ErrFatalFmt:                  "Erreur fatale : %s",
	ErrCrashedFmt:                "TermDevTools a planté : %v",
	InfoCrashReportFmt:           "Rapport complet écrit dans : %s",
	ErrCrashReportWriteFailedFmt: "Impossible d'écrire le rapport de plantage : %s",

	UsageExportDefaults:     "écrit dans ce `dossier` les recettes et endpoints intégrés au binaire (à lire, ou comme exemples), puis quitte",
	UsageVersion:            "affiche la version, puis quitte",
	InfoExportedDefaultsFmt: "%d fichier(s) écrit(s) dans %s",
	ErrExportDefaultsFmt:    "Export impossible : %s",
	ErrExportIntoSourceFmt:  "Export refusé : %s est un dossier où TermDevTools lit vos recettes et endpoints. Les fichiers exportés y seraient relus comme les vôtres et masqueraient ceux des versions suivantes. Choisissez un autre dossier.",

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
	ErrURLCredentials:          "No credentials in the URL (it is saved and displayed): choose the Basic Auth authentication instead.",
	StatusConnecting:           "Connecting...",
	ErrConnectFailedFmt:        "Connection failed: %s",
	ErrClusterHTTPFmt:          "The cluster responded HTTP %d",
	WarnConnectedSaveFailedFmt: "Connected, but failed to save config.yaml: %s",
	WarnTargetOverrideFmt:      "config.yaml: forced distribution/version ignored (%s)",
	DisplayUserNoAuth:          "(no auth)",
	BrowseHint:                 " (Enter: browse)",
	CertPickerTitleFmt:         " Choose a file — %s (Enter: open, Backspace: parent dir, Esc: cancel) ",
	ErrNoCertDirConfiguredFmt:  "no directory configured (%s in config.yaml)",
	ErrNoCertFilesInDirFmt:     "no files in %s",
	ErrCertDirNotFoundFmt:      "the %s directory doesn't exist",

	EditorTitle:     " Requests ",
	ResultTitle:     " Result ",
	CompletionTitle: " Complete (Enter, Esc to cancel) ",
	HelpViewTitle:   " Help (Esc to close) ",
	SearchLabel:     "Search: ",

	StatusBarTemplate:    "[white]%s: [green]%s[white]  |  User: [green]%s[white]  |  [%s]%s[white]",
	StatusClusterCaption: "Cluster",
	StatusIdle:           "ready",
	StatusRunning:        "request in progress...",
	StatusResultFmt:      "HTTP %d in %s",
	// Deliberate order: the most essential commands (help, language, quit)
	// come first, so they stay visible even on a narrow terminal where the
	// end of the line gets cut off (only 1 row is allocated for this bar).
	// "Ctrl(/Opt)" flags the shortcuts where Option also works, on macOS,
	// as an alternative to Ctrl (see HelpContent for why and its limits).
	ShortcutsHelpBar: "[gray]F1[white] help   [gray]F3[white] language   [gray]Ctrl+C[white] quit   " +
		"[gray]Ctrl+E[white] execute   [gray]F8[white] recipes   [gray]Tab/F10[white] complete   [gray]Ctrl(/Opt)+←/→[white] switch panel   " +
		"[gray]F5/F6[white] resize   [gray]Ctrl+F[white] search   [gray]Ctrl+S[white] save/export   " +
		"[gray]F2[white] copy   [gray]F4[white] reformat JSON   [gray]F9[white] copy as cURL   [gray]F7[white] reload your files",

	ErrLoadFailedFmt:        "failed to load %s: %s",
	ErrNoMatchFound:         "no match found",
	ErrNoCompletion:         "no completion",
	ErrSaveFailedFmt:        "save failed: %s",
	InfoSavedFmt:            "requests saved to %s",
	ErrExportFailedFmt:      "export failed: %s",
	InfoExportedFmt:         "result exported to %s",
	ErrNothingToCopy:        "nothing to copy",
	InfoCopied:              "result copied (OSC 52 — requires a compatible terminal)",
	ErrNothingToExport:      "nothing to export",
	ErrNoBodyToFormat:       "no JSON body to reformat",
	InfoCurlCopied:          "cURL command copied (OSC 52 — requires a compatible terminal)",
	ErrFormatCommentsInBody: "cannot reformat: # line inside or before the JSON body",
	ErrExitSaveFailedFmt:    "not saved, Ctrl+C again to quit anyway: %s",
	ErrSavedRequestsUnread:  "saved requests could not be read at startup (Ctrl+S to replace them)",
	WarnMoreProblemsFmt:     "%s (+%d more)",
	WarnCatColumnsBuiltIn:   "built-in column list (no answer from cluster)",
	InfoReloadedFmt:         "reloaded: %d variable(s), %d recipe(s), %d endpoint(s)",
	RecipeFilterLabel:       "Filter: ",
	RecipesTitleFmt:         "Recipes (%d)",
	RecipesHint:             "Enter: insert, Esc: close",
	RecipePreviewTitle:      " Preview ",
	RecipesNoMatch:          "no recipe matches",
	RecipeOwnMark:           "(added)",
	InfoRecipeInsertedFmt:   "recipe inserted: %s",
	ErrUnknownVariablesFmt:  "unknown variable(s): %s",

	HelpContent: `[yellow]TermDevTools[white] — terminal-mode Elasticsearch and OpenSearch client

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
  [aqua]F8[white]               Recipe catalog: ready-made requests, selected for the
                     cluster's version (type to filter)
  [aqua]Tab/F10[white]          Complete an endpoint or a _cat column while typing
                     (also: Ctrl+Space)
  [aqua]Ctrl+←/→[white]         Switch panel
                     (also: Option/Alt+←/→ on macOS)
  [aqua]F5/F6[white]            Resize the left/right split
                     (also: Ctrl+Shift+←/→, Option/Alt+Shift+←/→ on macOS — not guaranteed everywhere)
  [aqua]Ctrl+F[white]           Search in the active panel
  [aqua]Ctrl+S[white]           Save (left) / export (right)
  [aqua]F4[white]               Reformat (indent) the JSON body of the request under the cursor
  [aqua]F9[white]               Copy the request under the cursor as a cURL command (secrets not included)
  [aqua]F7[white]               Reload your hand-edited files: ${name} variables, recipes,
                     endpoints
  [aqua]F2[white]               Copy the result (right panel) to the clipboard
  [aqua]F1[white]               Show this help
  [aqua]F3[white]               Switch the interface language (fr/en)
  [aqua]Ctrl+C[white]           Quit (auto-saves the left panel)

[yellow]Files[white]
[gray]Nothing is required next to the binary: recipes and endpoints are
built in. These files, all optional, add to them.[white]

  [aqua]~/.config/termdevtools/[white]  (personal)
    config.yaml      known clusters
    queries_*.txt    per-cluster save (Ctrl+S)
    variables_*.txt  ${name} variables, per cluster
    recipes/*.txt    your recipes
    endpoints.txt    your endpoints for completion
  [aqua]<binary's directory>/[white]  (team, shared)
    recipes/*.txt, endpoints.txt   the team's
    cheatsheet.txt   editor's initial content
    exports/         exported results (Ctrl+S)

[gray]Esc to close this help.[white]`,

	ErrConfigLoadFmt:             "Error loading config.yaml: %s",
	ErrExecDirFmt:                "Could not determine the executable's directory: %s",
	ErrFatalFmt:                  "Fatal error: %s",
	ErrCrashedFmt:                "TermDevTools crashed: %v",
	InfoCrashReportFmt:           "Full report written to: %s",
	ErrCrashReportWriteFailedFmt: "Could not write crash report: %s",

	UsageExportDefaults:     "write the recipes and endpoints built into the binary to this `directory` (to read them, or as examples), then exit",
	UsageVersion:            "print the version, then exit",
	InfoExportedDefaultsFmt: "%d file(s) written to %s",
	ErrExportDefaultsFmt:    "Export failed: %s",
	ErrExportIntoSourceFmt:  "Export refused: %s is a directory TermDevTools reads your recipes and endpoints from. The exported files would be read back as your own and hide those of later versions. Choose another directory.",

	LanguageName:            "English",
	InfoLanguageSwitchedFmt: "Language: %s",
}

// Normalize maps lang ("fr"/"en", case-insensitive, possibly padded with
// whitespace) to exactly FR or EN, defaulting to EN for anything else
// (empty, unrecognized): English is the interface's default language, like
// the rest of what the project publishes.
func Normalize(lang string) string {
	if strings.ToLower(strings.TrimSpace(lang)) == FR {
		return FR
	}
	return EN
}

// For returns the message catalog for lang (see Normalize).
func For(lang string) *Strings {
	if Normalize(lang) == FR {
		return &fr
	}
	return &en
}
