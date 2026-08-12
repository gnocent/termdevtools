// Package config lit et écrit config.yaml, dans le dossier de configuration
// de l'utilisateur courant (l'historique de connexions est personnel, pas
// lié à l'installation du binaire). Aucun secret (mot de passe, API key
// secret, passphrase de clé privée) n'y est jamais stocké — voir SPEC.md §5
// et §9.2.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

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
	appDirName            = "termdevtools"
)

// unsafeFilenameChars couvre tout ce qu'une URL peut contenir et qu'un nom
// de fichier ne supporte pas forcément (":", "/", espaces...) — remplacé
// par "_" dans QueriesPathForURL.
var unsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// TLS regroupe les options TLS d'un cluster.
type TLS struct {
	Verify     bool   `yaml:"verify"`
	CAFile     string `yaml:"ca_file,omitempty"`
	ClientCert string `yaml:"client_cert,omitempty"`
	ClientKey  string `yaml:"client_key,omitempty"`
}

// Cluster décrit une connexion connue, sans aucun secret. L'URL sert
// d'identifiant (pas de nom séparé : l'URL est déjà l'information la plus
// explicite pour reconnaître un cluster dans l'historique).
type Cluster struct {
	URL      string `yaml:"url"`
	AuthType string `yaml:"auth_type"`
	Username string `yaml:"username,omitempty"`
	APIKeyID string `yaml:"api_key_id,omitempty"`
	TLS      TLS    `yaml:"tls"`
}

// Config est le contenu complet de config.yaml.
type Config struct {
	DefaultTimeoutSeconds int       `yaml:"default_timeout_seconds"`
	DefaultCADir          string    `yaml:"default_ca_dir,omitempty"`
	DefaultClientCertDir  string    `yaml:"default_client_cert_dir,omitempty"`
	Clusters              []Cluster `yaml:"clusters"`

	path string `yaml:"-"`
}

// ExecutableDir renvoie le dossier contenant le binaire termdevtools.
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

// ConfigDir renvoie le dossier de configuration de l'utilisateur courant
// (~/.config/termdevtools, ou $XDG_CONFIG_HOME/termdevtools s'il est défini).
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

// Path renvoie le chemin attendu de config.yaml, dans ConfigDir().
func Path() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// QueriesPathForURL renvoie le chemin de la sauvegarde personnelle du
// contenu de l'éditeur pour le cluster d'URL url : un fichier par cluster,
// par utilisateur (Ctrl+S et sauvegarde automatique en sortie de programme,
// cf. SPEC.md §3.2 et §9.1), à côté de config.yaml. Les caractères non
// compatibles avec un nom de fichier sont remplacés par "_" ; deux URLs
// distinctes qui se ressembleraient au point de produire le même nom après
// cette normalisation partageraient (rare) le même fichier — limite admise
// pour garder des noms de fichiers simples et lisibles.
func QueriesPathForURL(url string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	safe := unsafeFilenameChars.ReplaceAllString(url, "_")
	return filepath.Join(dir, queriesFilePrefix+safe+queriesFileSuffix), nil
}

// Load lit config.yaml. Si le fichier n'existe pas encore (premier lancement),
// une configuration vide avec les valeurs par défaut est renvoyée sans erreur.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, fmt.Errorf("résolution du chemin de config.yaml: %w", err)
	}

	cfg := &Config{DefaultTimeoutSeconds: defaultTimeoutSeconds, path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lecture de %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing de %s: %w", path, err)
	}
	cfg.path = path
	if cfg.DefaultTimeoutSeconds <= 0 {
		cfg.DefaultTimeoutSeconds = defaultTimeoutSeconds
	}
	return cfg, nil
}

// Save écrit la configuration dans config.yaml (permissions restreintes :
// le fichier ne contient pas de secret mais autant limiter sa visibilité).
// Le dossier de configuration est créé s'il n'existe pas encore (premier
// lancement de cet utilisateur).
func (c *Config) Save() error {
	if c.path == "" {
		path, err := Path()
		if err != nil {
			return err
		}
		c.path = path
	}

	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("création de %s: %w", filepath.Dir(c.path), err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("sérialisation de config.yaml: %w", err)
	}
	if err := os.WriteFile(c.path, data, 0o600); err != nil {
		return fmt.Errorf("écriture de %s: %w", c.path, err)
	}
	return nil
}

// FindByURL renvoie une copie du cluster d'URL url, s'il existe.
func (c *Config) FindByURL(url string) (Cluster, bool) {
	for _, cl := range c.Clusters {
		if cl.URL == url {
			return cl, true
		}
	}
	return Cluster{}, false
}

// Promote insère ou met à jour cluster (identifié par son URL) puis le
// place en tête de la liste — l'ordre de Clusters fait office d'historique
// d'utilisation (le plus récent en premier). À appeler uniquement après une
// connexion réussie.
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
