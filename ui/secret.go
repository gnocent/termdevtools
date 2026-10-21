package ui

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gnocent/termdevtools/config"
	"github.com/gnocent/termdevtools/i18n"
)

// secretFileMaxSize bounds what is read of a secret_file: a password, a key
// or a token takes a few hundred bytes; a file larger than this isn't one.
const secretFileMaxSize = 64 << 10

// clusterSecret reads the secret cl's entry in config.yaml points to —
// secret_env or secret_file, SPEC.md §5 — for the connection form to fill in
// as if it had been typed. It returns the secret, empty if there is none to
// read or it can't be; what the form's message says about it — where it was
// read from, or why it wasn't — empty if the entry points to nothing; and
// whether that message is a warning. Never blocking: without a secret, it is
// typed as usual.
func clusterSecret(cl config.Cluster, msgs *i18n.Strings) (secret, message string, warn bool) {
	switch {
	case !asksForSecret(cl):
		return "", "", false
	case cl.SecretEnv != "" && cl.SecretFile != "":
		return "", msgs.ErrSecretBothKeys, true
	case cl.SecretEnv != "":
		if secret = os.Getenv(cl.SecretEnv); secret == "" {
			return "", fmt.Sprintf(msgs.ErrSecretEnvFmt, cl.SecretEnv), true
		}
		return secret, fmt.Sprintf(msgs.InfoSecretEnvFmt, cl.SecretEnv), false
	case cl.SecretFile != "":
		return readSecretFile(cl.SecretFile, msgs)
	}
	return "", "", false
}

// asksForSecret reports whether the connection form asks cl's
// authentication for a secret: one at most, whatever the type.
func asksForSecret(cl config.Cluster) bool {
	switch cl.AuthType {
	case config.AuthBasic, config.AuthAPIKey, config.AuthBearer:
		return true
	case config.AuthMTLS:
		return urlLooksHTTPS(cl.URL) // the client key's passphrase: https only
	}
	return false
}

// errNotRegularFile refuses a secret_file that is a pipe, a terminal, a
// device or a directory: opened or read from the interface's own goroutine,
// a pipe with no writer or a terminal would freeze it.
var errNotRegularFile = errors.New("not a regular file")

// utf8BOM is what some Windows editors put at the start of a UTF-8 file.
const utf8BOM = "\xef\xbb\xbf"

// readSecretFile is clusterSecret for a secret_file, name as written in
// config.yaml: read whole, without the line ends an editor puts at the end
// (spaces are kept: they may be part of a password), nor a leading UTF-8
// byte order mark.
func readSecretFile(name string, msgs *i18n.Strings) (secret, message string, warn bool) {
	path := expandHome(name)
	if !filepath.IsAbs(path) {
		// Relative to what the program was started from: not what was meant.
		return "", fmt.Sprintf(msgs.ErrSecretFileRelativeFmt, name), true
	}
	info, err := os.Stat(path)
	if err == nil && !info.Mode().IsRegular() {
		err = errNotRegularFile
	}
	if err != nil {
		return "", fmt.Sprintf(msgs.ErrSecretFileFmt, name, withoutPath(err)), true
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Sprintf(msgs.ErrSecretFileFmt, name, withoutPath(err)), true
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, secretFileMaxSize+1))
	if err != nil {
		return "", fmt.Sprintf(msgs.ErrSecretFileFmt, name, withoutPath(err)), true
	}
	if strings.IndexByte(string(data), 0) >= 0 {
		// UTF-16, what Windows PowerShell 5.1's ">" writes, or not text at
		// all: sent as it is, it would only fail — or lock the account.
		return "", fmt.Sprintf(msgs.ErrSecretFileEncodingFmt, name), true
	}
	secret = strings.TrimRight(strings.TrimPrefix(string(data), utf8BOM), "\r\n")
	if secret == "" || len(data) > secretFileMaxSize {
		return "", fmt.Sprintf(msgs.ErrSecretFileSizeFmt, name), true
	}
	if info, err := file.Stat(); err == nil && othersCanRead(runtime.GOOS, info.Mode()) {
		return secret, fmt.Sprintf(msgs.WarnSecretFileReadableFmt, name), true
	}
	return secret, fmt.Sprintf(msgs.InfoSecretFileFmt, name), false
}

// expandHome replaces a leading "~/" with the user's home directory. Not
// "~user/": left as it is, it is refused as a relative path.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, "~"+string(filepath.Separator)) {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// withoutPath drops the path an error from os repeats: the message names the
// file as written in config.yaml already.
func withoutPath(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}

// othersCanRead reports whether a file of this mode can be read by users
// other than its owner — its group, or anyone — on goos. Never on Windows,
// whose access rights these bits don't describe. A function of goos, rather
// than of runtime.GOOS, to be tested for every system whichever one the tests
// run on.
func othersCanRead(goos string, mode fs.FileMode) bool {
	return goos != "windows" && mode.Perm()&0o044 != 0
}
