// Package config reads and writes config.yaml, in the current user's
// configuration directory (connection history is personal, not tied to the
// binary's installation). No secret (password, API key secret, private key
// passphrase) is ever stored there — see SPEC.md §5 and §9.2.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	AuthNone   = "none"
	AuthBasic  = "basic"
	AuthAPIKey = "api_key"
	AuthMTLS   = "mtls"

	defaultTimeoutSeconds = 120
	fileName              = "config.yaml"
	queriesFilePrefix     = "queries_"
	queriesFileSuffix     = ".txt"
	variablesFilePrefix   = "variables_"
	variablesFileSuffix   = ".txt"
	appDirName            = "termdevtools"
)

// defaultCertDir pre-fills both DefaultCADir and DefaultClientCertDir when
// config.yaml doesn't set them: the standard system-wide TLS certificate
// directory on RHEL/CentOS (this project's primary deployment target, see
// SPEC.md §1), so the connection form's CA/client-cert/client-key fields
// and the certificate picker popup (ui/connect.go) have something sensible
// to work with out of the box.
//
// Empty on any OS other than Linux: assuming this same RHEL-specific path
// also exists on Windows or macOS was a real bug — it doesn't, and pointing
// the certificate picker at it there produced a raw "directory not found"
// OS error instead of the friendlier "nothing configured" message an empty
// default already routes to (openCertPicker in ui/connect.go).
var defaultCertDir = certDirForOS(runtime.GOOS)

// certDirForOS is defaultCertDir's actual logic, factored out as a plain
// function of a goos string (rather than reading runtime.GOOS directly) so
// it can be tested for every target platform regardless of which one the
// tests happen to run on.
func certDirForOS(goos string) string {
	if goos == "linux" {
		return "/etc/pki/tls/certs"
	}
	return ""
}

// unsafeFilenameChars covers everything a URL can contain that a filename
// can't necessarily support (":", "/", spaces...) — replaced with "_" in
// QueriesPathForURL.
var unsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// TLS groups a cluster's TLS options.
type TLS struct {
	Verify     bool   `yaml:"verify"`
	CAFile     string `yaml:"ca_file,omitempty"`
	ClientCert string `yaml:"client_cert,omitempty"`
	ClientKey  string `yaml:"client_key,omitempty"`
}

// Cluster describes a known connection, with no secret whatsoever. The URL
// serves as the identifier (no separate name: the URL is already the most
// explicit piece of information to recognize a cluster in the history).
type Cluster struct {
	URL      string `yaml:"url"`
	AuthType string `yaml:"auth_type"`
	Username string `yaml:"username,omitempty"`
	APIKeyID string `yaml:"api_key_id,omitempty"`
	TLS      TLS    `yaml:"tls"`
}

// Config is the full content of config.yaml.
type Config struct {
	DefaultTimeoutSeconds int    `yaml:"default_timeout_seconds"`
	DefaultCADir          string `yaml:"default_ca_dir,omitempty"`
	DefaultClientCertDir  string `yaml:"default_client_cert_dir,omitempty"`
	// Language selects the interface language: "fr" (default) or "en". See
	// the i18n package and SPEC.md §3.
	Language string `yaml:"language,omitempty"`
	// Mouse enables mouse support (click to focus/select). Off by default —
	// the zero value already is false, so an absent key naturally means
	// disabled, no defaulting logic needed unlike Language. Keyboard
	// navigation fully covers every mouse interaction (SPEC.md §3, §4);
	// leaving it off keeps the terminal's own native text
	// selection/copy/paste available, which EnableMouse(true) would
	// otherwise capture for the app instead.
	Mouse    bool      `yaml:"mouse,omitempty"`
	Clusters []Cluster `yaml:"clusters"`

	path string `yaml:"-"`
}

// ExecutableDir returns the directory containing the termdevtools binary.
func ExecutableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	return filepath.Dir(resolved), nil
}

// ConfigDir returns the current user's configuration directory
// (~/.config/termdevtools, or $XDG_CONFIG_HOME/termdevtools if that variable
// is set).
func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, appDirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", appDirName), nil
}

// Path returns the expected path of config.yaml, inside ConfigDir().
func Path() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// QueriesPathForURL returns the path to the personal save of the editor
// content for the cluster at url: one file per cluster, per user (Ctrl+S and
// automatic save on program exit, see SPEC.md §3.2 and §9.1), next to
// config.yaml. Characters not compatible with a filename are replaced with
// "_"; two distinct URLs similar enough to produce the same name after this
// normalization would (rarely) share the same file — an accepted limitation
// to keep filenames simple and readable.
func QueriesPathForURL(url string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	safe := unsafeFilenameChars.ReplaceAllString(url, "_")
	return filepath.Join(dir, queriesFilePrefix+safe+queriesFileSuffix), nil
}

// VariablesPathForURL returns the path to the personal store of reusable
// `${name}` variables (SPEC.md §7 backlog #4) for the cluster at url — same
// one-file-per-cluster-per-user scheme, same filename sanitization, as
// QueriesPathForURL, just a different prefix.
func VariablesPathForURL(url string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	safe := unsafeFilenameChars.ReplaceAllString(url, "_")
	return filepath.Join(dir, variablesFilePrefix+safe+variablesFileSuffix), nil
}

// Load reads config.yaml. If the file doesn't exist yet (first launch), an
// empty configuration with default values is returned with no error, and a
// commented config.yaml documenting every setting (see
// writeDefaultConfigFile) is created so it can be discovered and edited. If
// it does exist but predates one or more settings (e.g. saved by an older
// version of the program, or hand-edited down to just a couple of keys),
// the missing ones are appended the same way (backfillMissingSettings) —
// existing keys and values are never touched.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, fmt.Errorf("resolving config.yaml path: %w", err)
	}

	// DefaultCADir/DefaultClientCertDir are pre-set here, before Unmarshal,
	// rather than defaulted afterward like DefaultTimeoutSeconds below:
	// yaml.Unmarshal only overwrites fields actually present in the
	// document, so a config.yaml that omits the key (or predates this
	// default) keeps defaultCertDir, while one that explicitly sets it to
	// "" (opting out of any default) is respected instead of being forced
	// back — unlike a timeout, an empty path is a legitimate "no default"
	// choice, not an invalid value to correct.
	cfg := &Config{
		DefaultTimeoutSeconds: defaultTimeoutSeconds,
		DefaultCADir:          defaultCertDir,
		DefaultClientCertDir:  defaultCertDir,
		path:                  path,
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		writeDefaultConfigFile(path)
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	cfg.path = path
	if cfg.DefaultTimeoutSeconds <= 0 {
		cfg.DefaultTimeoutSeconds = defaultTimeoutSeconds
	}
	backfillMissingSettings(path, string(data))
	return cfg, nil
}

// Save writes the configuration to config.yaml (restricted permissions: the
// file contains no secret, but its visibility is still worth limiting). The
// configuration directory is created if it doesn't exist yet (this user's
// first launch).
func (c *Config) Save() error {
	if c.path == "" {
		path, err := Path()
		if err != nil {
			return err
		}
		c.path = path
	}

	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(c.path), err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("serializing config.yaml: %w", err)
	}
	if err := os.WriteFile(c.path, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", c.path, err)
	}
	return nil
}

// configFileHeader introduces the generated config.yaml (see
// defaultConfigFileContent) — written once, above every setting, on first
// launch only (an existing header is never touched by the backfill in
// backfillMissingSettings).
const configFileHeader = `# Fichier de configuration de TermDevTools — généré automatiquement au
# premier lancement (voir SPEC.md §9.1-§9.2). Propre à cet utilisateur :
# deux personnes lançant le même binaire partagé n'ont pas le même
# historique de connexions. Aucun secret n'y est jamais stocké : mots de
# passe, API key secrets et passphrases de clé privée sont toujours
# redemandés à la connexion.
#
# Chaque paramètre ci-dessous est affiché à sa valeur par défaut ; modifiez
# ou supprimez selon vos besoins.
`

// configSettings lists every top-level config.yaml setting, in the order
// they're written: key is the YAML key used to detect whether a given
// setting is already present in an existing file (backfillMissingSettings),
// block is the comment-plus-value snippet inserted for it — either in the
// fresh file written on first launch, or appended for any of these settings
// still missing from an older file (e.g. one written before "mouse" or
// "default_ca_dir" existed). Each block leads with a blank line, so blocks
// can be concatenated directly one after another (or appended to existing
// content) with consistent spacing.
//
// The two directory settings default to defaultCertDir (RHEL/CentOS'
// standard system-wide TLS certificate directory) rather than being unset,
// so they're shown here as active values like every other setting.
var configSettings = []struct{ key, block string }{
	{"default_timeout_seconds", fmt.Sprintf(`
# Délai maximum (en secondes) avant qu'une requête n'échoue en timeout.
default_timeout_seconds: %d
`, defaultTimeoutSeconds)},
	{"language", `
# Langue de l'interface : "fr" ou "en". Bascule aussi en direct avec F3
# (qui met à jour cette ligne automatiquement).
language: fr
`},
	{"mouse", `
# Support de la souris (cliquer pour donner le focus à un champ/une entrée
# de liste). Désactivé par défaut : toute interaction souris a un
# équivalent clavier complet (SPEC.md §3-4), et la laisser désactivée
# garde la sélection/collage natifs du terminal disponibles (F2 copie
# quand même le résultat, voir SPEC.md §3.3).
mouse: false
`},
	{"default_ca_dir", fmt.Sprintf(`
# Dossier pré-rempli pour le champ CA lors de la saisie d'une nouvelle
# connexion (§3.0), et utilisé par le sélecteur de certificat (Entrée sur
# le champ) pour y lister les fichiers disponibles à choisir.
default_ca_dir: %s
`, defaultCertDir)},
	{"default_client_cert_dir", fmt.Sprintf(`
# Dossier pré-rempli pour les champs de certificat client (mTLS) lors de la
# saisie d'une nouvelle connexion (§3.0), même usage que default_ca_dir
# ci-dessus. Identique par défaut : ajustez si vos certificats client sont
# ailleurs.
default_client_cert_dir: %s
`, defaultCertDir)},
	{"clusters", `
# Historique des clusters connus. L'ordre fait office d'historique
# d'utilisation : le plus récemment utilisé est automatiquement replacé en
# tête (§9.2). Rempli/mis à jour automatiquement après chaque connexion
# réussie — vide au premier lancement.
clusters: []
`},
}

// defaultConfigFileContent renders the full config.yaml written on first
// launch: the header followed by every setting at its default value.
func defaultConfigFileContent() string {
	var b strings.Builder
	b.WriteString(configFileHeader)
	for _, s := range configSettings {
		b.WriteString(s.block)
	}
	return b.String()
}

// writeDefaultConfigFile creates config.yaml at path, pre-filled from
// defaultConfigFileContent (see Load, called only when the file doesn't
// exist yet). Best-effort and silent: a failure here (e.g. a read-only
// filesystem) must not stop the app from starting — it still runs fine off
// the in-memory defaults, and later explicit saves (Ctrl+S, F3, a
// successful connection) surface their own errors as usual if the problem
// persists.
func writeDefaultConfigFile(path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(defaultConfigFileContent()), 0o600)
}

// settingKeyPresent reports whether key already appears in content as a
// YAML mapping key at the start of a line — commented-out counts too (e.g.
// "#default_ca_dir:"), since that's exactly how an unset optional setting
// is documented by this same package: once shown, a setting must not be
// re-appended just because it's still at its default.
func settingKeyPresent(content, key string) bool {
	re := regexp.MustCompile(`(?m)^[ \t]*#?[ \t]*` + regexp.QuoteMeta(key) + `[ \t]*:`)
	return re.MatchString(content)
}

// backfillMissingSettings appends, to the existing config.yaml at path
// (whose current content is passed in as content, already read by Load),
// any of configSettings not already present in it — so a config.yaml
// written by an older version of the program (before some setting existed)
// still ends up documenting every setting a current one supports, without
// touching what the user already has. A no-op, as it should be, once every
// setting has been backfilled once. Best-effort and silent, same rationale
// as writeDefaultConfigFile.
func backfillMissingSettings(path, content string) {
	var missing strings.Builder
	for _, s := range configSettings {
		if !settingKeyPresent(content, s.key) {
			missing.WriteString(s.block)
		}
	}
	if missing.Len() == 0 {
		return
	}

	updated := strings.TrimRight(content, "\n") + "\n" +
		"\n# Paramètres ajoutés automatiquement au démarrage (absents de ce fichier) :\n" +
		missing.String()
	_ = os.WriteFile(path, []byte(updated), 0o600)
}

// FindByURL returns a copy of the cluster at url, if it exists.
func (c *Config) FindByURL(url string) (Cluster, bool) {
	for _, cl := range c.Clusters {
		if cl.URL == url {
			return cl, true
		}
	}
	return Cluster{}, false
}

// Promote inserts or updates cluster (identified by its URL) then moves it
// to the front of the list — the order of Clusters acts as usage history
// (most recent first). Must only be called after a successful connection.
func (c *Config) Promote(cluster Cluster) {
	filtered := make([]Cluster, 0, len(c.Clusters)+1)
	filtered = append(filtered, cluster)
	for _, cl := range c.Clusters {
		if cl.URL != cluster.URL {
			filtered = append(filtered, cl)
		}
	}
	c.Clusters = filtered
}
