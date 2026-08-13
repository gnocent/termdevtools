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

// Load reads config.yaml. If the file doesn't exist yet (first launch), an
// empty configuration with default values is returned with no error.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, fmt.Errorf("resolving config.yaml path: %w", err)
	}

	cfg := &Config{DefaultTimeoutSeconds: defaultTimeoutSeconds, path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
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
