package esclient

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The encrypted keys under testdata/pkcs8 were written by OpenSSL itself —
// the point of these tests being that what OpenSSL encrypts, this package
// decrypts. To write them again (openssl must be in the PATH):
//
//	go test ./esclient/ -run TestEncryptedPKCS8 -args -update-pkcs8
var updatePKCS8 = flag.Bool("update-pkcs8", false, "regenerate testdata/pkcs8 with openssl")

// pkcs8Passphrase protects every key under testdata/pkcs8. Test keys, made
// for this repository and used nowhere else.
const pkcs8Passphrase = "termdevtools test passphrase"

// pkcs8Fixtures lists the encrypted keys: the options given to "openssl
// pkcs8 -topk8" to produce each, and — for the forms this package doesn't
// read — what the error must say.
var pkcs8Fixtures = []struct {
	name    string
	ec      bool // an ECDSA P-256 key rather than RSA 2048
	options []string
	wantErr string
}{
	// What "openssl pkcs8 -topk8" and "openssl genpkey -aes256" write by
	// default since OpenSSL 1.1.0: PBKDF2 with HMAC-SHA256.
	{name: "rsa-aes256-sha256", options: []string{"-v2", "aes-256-cbc"}},
	{name: "ec-aes128-sha1", ec: true, options: []string{"-v2", "aes-128-cbc", "-v2prf", "hmacWithSHA1"}},
	{name: "rsa-aes192-sha512", options: []string{"-v2", "aes-192-cbc", "-v2prf", "hmacWithSHA512"}},
	{name: "ec-des3-sha256", ec: true, options: []string{"-v2", "des3"}},
	{name: "rsa-scrypt", options: []string{"-scrypt"}, wantErr: "scrypt"},
	{name: "ec-pkcs12-pbe", ec: true, options: []string{"-v1", "PBE-SHA1-3DES"}, wantErr: "only PBES2"},
}

func pkcs8Paths(name string) (certFile, keyFile string) {
	base := filepath.Join("testdata", "pkcs8", name)
	return base + ".crt", base + ".key"
}

// writePKCS8Fixture generates a key and its self-signed client certificate,
// and has openssl write the key in encrypted PKCS#8 form.
func writePKCS8Fixture(t *testing.T, name string, ec bool, options []string) {
	t.Helper()
	var key crypto.Signer
	var err error
	if ec {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	} else {
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	}
	if err != nil {
		t.Fatalf("generating the key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "termdevtools test client " + name},
		NotBefore:             time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2126, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true, // self-signed: its own authority on the server side
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatalf("creating the certificate: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("encoding the key: %v", err)
	}

	certFile, keyFile := pkcs8Paths(name)
	if err := os.MkdirAll(filepath.Dir(certFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	plain := filepath.Join(t.TempDir(), "plain.key")
	if err := os.WriteFile(plain, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	args := append([]string{"pkcs8", "-topk8", "-in", plain, "-out", keyFile, "-passout", "pass:" + pkcs8Passphrase}, options...)
	if out, err := exec.Command("openssl", args...).CombinedOutput(); err != nil {
		t.Fatalf("openssl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// TestEncryptedPKCS8Keys checks every key written by OpenSSL: the supported
// forms are decrypted — to the very key the certificate was issued for,
// which tls.X509KeyPair verifies — and the others refused with an error
// that names what is unsupported.
func TestEncryptedPKCS8Keys(t *testing.T) {
	for _, fixture := range pkcs8Fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			if *updatePKCS8 {
				writePKCS8Fixture(t, fixture.name, fixture.ec, fixture.options)
			}
			certFile, keyFile := pkcs8Paths(fixture.name)
			keyPEM, err := os.ReadFile(keyFile)
			if err != nil {
				t.Fatalf("%v — regenerate the fixtures with -args -update-pkcs8", err)
			}
			if block, _ := pem.Decode(keyPEM); block == nil || block.Type != "ENCRYPTED PRIVATE KEY" {
				t.Fatalf("test setup: %s is not an encrypted PKCS#8 key", keyFile)
			}

			cert, err := loadClientCertificate(certFile, keyFile, pkcs8Passphrase)
			if fixture.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), fixture.wantErr) {
					t.Fatalf("expected an error mentioning %q, got %v", fixture.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected the key to be decrypted, got %v", err)
			}
			if _, isSigner := cert.PrivateKey.(crypto.Signer); !isSigner {
				t.Errorf("unexpected private key type %T", cert.PrivateKey)
			}

			const wrong = "not the passphrase"
			_, err = loadClientCertificate(certFile, keyFile, wrong)
			if err == nil || !strings.Contains(err.Error(), "wrong passphrase") {
				t.Errorf("wrong passphrase: expected it to be reported as such, got %v", err)
			}
			if err != nil && strings.Contains(err.Error(), wrong) {
				t.Errorf("a passphrase must never appear in an error: %v", err)
			}
			_, err = loadClientCertificate(certFile, keyFile, "")
			if err == nil || !strings.Contains(err.Error(), "passphrase is required") {
				t.Errorf("no passphrase: expected it to be asked for, got %v", err)
			}
		})
	}
}

// TestMutualTLSWithEncryptedPKCS8Key is the whole path: a request to a
// server that demands a client certificate, authenticated with a key
// OpenSSL encrypted.
func TestMutualTLSWithEncryptedPKCS8Key(t *testing.T) {
	certFile, keyFile := pkcs8Paths("rsa-aes256-sha256")
	clientCertPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("%v — regenerate the fixtures with -args -update-pkcs8", err)
	}
	trusted := x509.NewCertPool()
	if !trusted.AppendCertsFromPEM(clientCertPEM) {
		t.Fatal("test setup: the client certificate could not be read")
	}

	var presented string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) > 0 {
			presented = r.TLS.PeerCertificates[0].Subject.CommonName
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	srv.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trusted}
	srv.StartTLS()
	defer srv.Close()

	caFile := filepath.Join(t.TempDir(), "server-ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	client, err := New(Params{
		URL: srv.URL, AuthType: AuthMTLS, Verify: true, CAFile: caFile,
		ClientCert: certFile, ClientKey: keyFile, KeyPassphrase: pkcs8Passphrase,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result, err := client.Execute(context.Background(), "GET", "/", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(presented, "termdevtools test client") {
		t.Errorf("expected the server to accept the client certificate, got HTTP %d, certificate %q", result.StatusCode, presented)
	}
}

// TestDecryptPKCS8RejectsGarbage checks that what isn't an encrypted key at
// all is refused cleanly, whatever it holds.
func TestDecryptPKCS8RejectsGarbage(t *testing.T) {
	for _, der := range [][]byte{nil, []byte("opaque"), {0x30, 0x00}, {0x30, 0x03, 0x02, 0x01, 0x00}} {
		if _, err := decryptPKCS8(der, pkcs8Passphrase); err == nil {
			t.Errorf("expected %x to be refused", der)
		}
	}
}

func TestStripPKCS7Padding(t *testing.T) {
	for _, c := range []struct {
		in   []byte
		want string
		ok   bool
	}{
		{[]byte("abc\x05\x05\x05\x05\x05"), "abc", true},
		{[]byte("abcdefg\x01"), "abcdefg", true},
		{[]byte("\x08\x08\x08\x08\x08\x08\x08\x08"), "", true},
		{[]byte("abcdefgh"), "", false},                         // no padding: the last byte is far too large
		{[]byte("abc\x05\x05\x04\x05\x05"), "", false},          // inconsistent
		{[]byte("abcdefg\x00"), "", false},                      // zero is not a padding length
		{[]byte("\x09\x09\x09\x09\x09\x09\x09\x09"), "", false}, // longer than a block
		{nil, "", false},
	} {
		got, ok := stripPKCS7Padding(c.in, 8)
		if ok != c.ok || string(got) != c.want {
			t.Errorf("stripPKCS7Padding(%q): got %q, %v — want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
