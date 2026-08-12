// Package esclient exécute des requêtes HTTP vers un cluster Elasticsearch,
// avec gestion de l'authentification (aucune, Basic Auth, API Key, mTLS) et
// du TLS (CA personnalisé, vérification désactivable). Voir SPEC.md §5.
package esclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	AuthNone   = "none"
	AuthBasic  = "basic"
	AuthAPIKey = "api_key"
	AuthMTLS   = "mtls"
)

// Params rassemble tout ce qu'il faut pour établir une connexion, y compris
// les secrets. Une valeur Params ne doit jamais être persistée telle quelle.
type Params struct {
	URL      string
	AuthType string
	Timeout  time.Duration

	// Basic Auth
	Username string
	Password string

	// API Key (Authorization: ApiKey base64(id:secret))
	APIKeyID     string
	APIKeySecret string

	// TLS
	Verify     bool
	CAFile     string
	ClientCert string
	ClientKey  string
	// KeyPassphrase déchiffre ClientKey si la clé privée est chiffrée
	// (format PEM legacy, ex. clé générée par `openssl -des3`).
	KeyPassphrase string
}

// Client exécute des requêtes vers un cluster Elasticsearch déjà authentifié.
type Client struct {
	baseURL string
	params  Params
	http    *http.Client
}

// Result est la réponse d'une requête exécutée.
type Result struct {
	StatusCode int
	Duration   time.Duration
	Body       []byte
}

// New construit un Client à partir de Params, y compris la configuration TLS.
func New(p Params) (*Client, error) {
	if p.URL == "" {
		return nil, errors.New("URL du cluster manquante")
	}
	if p.Timeout <= 0 {
		p.Timeout = 120 * time.Second
	}

	tlsConfig := &tls.Config{InsecureSkipVerify: !p.Verify}

	if p.CAFile != "" {
		pool, err := loadCAPool(p.CAFile)
		if err != nil {
			return nil, fmt.Errorf("chargement du CA %s: %w", p.CAFile, err)
		}
		tlsConfig.RootCAs = pool
	}

	if p.AuthType == AuthMTLS {
		cert, err := loadClientCertificate(p.ClientCert, p.ClientKey, p.KeyPassphrase)
		if err != nil {
			return nil, fmt.Errorf("chargement du certificat client: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	transport := &http.Transport{TLSClientConfig: tlsConfig}

	return &Client{
		baseURL: strings.TrimRight(p.URL, "/"),
		params:  p,
		http:    &http.Client{Transport: transport, Timeout: p.Timeout},
	}, nil
}

// Execute envoie une requête method/path (ex. "GET", "_cat/indices?v") avec
// un corps JSON optionnel, et renvoie le statut, la durée et le corps reçu.
func (c *Client) Execute(ctx context.Context, method, path string, body []byte) (*Result, error) {
	url := c.baseURL + "/" + strings.TrimLeft(path, "/")

	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), url, reader)
	if err != nil {
		return nil, fmt.Errorf("construction de la requête: %w", err)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	// Pas d'Accept forcé à application/json : Elasticsearch en tient compte
	// pour la négociation de contenu, y compris sur _cat/* qui bascule alors
	// en JSON au lieu de son format tabulaire par défaut (SPEC.md §3.3) — le
	// format de réponse doit rester au choix de chaque endpoint.
	c.applyAuth(req)

	start := time.Now()
	resp, err := c.http.Do(req)
	duration := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("appel HTTP: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lecture de la réponse: %w", err)
	}

	return &Result{StatusCode: resp.StatusCode, Duration: duration, Body: respBody}, nil
}

func (c *Client) applyAuth(req *http.Request) {
	switch c.params.AuthType {
	case AuthBasic:
		req.SetBasicAuth(c.params.Username, c.params.Password)
	case AuthAPIKey:
		token := base64.StdEncoding.EncodeToString([]byte(c.params.APIKeyID + ":" + c.params.APIKeySecret))
		req.Header.Set("Authorization", "ApiKey "+token)
	case AuthMTLS, AuthNone:
		// Rien à ajouter : l'authentification mTLS se fait au niveau TLS.
	}
}

func loadCAPool(caFile string) (*x509.CertPool, error) {
	data, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("aucun certificat valide trouvé dans %s", caFile)
	}
	return pool, nil
}

func loadClientCertificate(certFile, keyFile, passphrase string) (tls.Certificate, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("lecture du certificat %s: %w", certFile, err)
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("lecture de la clé %s: %w", keyFile, err)
	}

	if passphrase != "" {
		keyPEM, err = decryptPEMKey(keyPEM, passphrase)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("déchiffrement de la clé privée: %w", err)
		}
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}

// decryptPEMKey déchiffre une clé privée PEM legacy (ex. "-----BEGIN RSA
// PRIVATE KEY-----" chiffrée par openssl avec DES/AES-CBC et en-tête
// "DEK-Info"). Ne couvre pas le format PKCS8 chiffré moderne.
//
// seul moyen simple de déchiffrer ce format legacy sans dépendance externe.
//
//nolint:staticcheck // x509.DecryptPEMBlock est deprecated mais reste le
func decryptPEMKey(keyPEM []byte, passphrase string) ([]byte, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("clé privée PEM invalide")
	}
	if !x509.IsEncryptedPEMBlock(block) {
		return keyPEM, nil
	}
	der, err := x509.DecryptPEMBlock(block, []byte(passphrase))
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: block.Type, Bytes: der}), nil
}
