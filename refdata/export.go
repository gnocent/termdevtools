package refdata

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// userTemplateFile is the recipe file created in the user's directory, see
// CreateUserTemplate. Embedded next to the recipes, not among them.
const userTemplateFile = "my-recipes.txt"

// CreateUserTemplate writes, the first time only — as long as userDir has no
// recipes directory — a recipe file holding nothing but the explanation of
// its own format, so that where and how to write one's own recipes can be
// found without the documentation. A user who deletes the file keeps the
// directory, and isn't given the file again.
func CreateUserTemplate(userDir string) error {
	if userDir == "" {
		return nil
	}
	dir := filepath.Join(userDir, recipesDir)
	if _, err := os.Lstat(dir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, userTemplateFile), mustReadEmbedded(userTemplateFile), 0o600)
}

// ExportDefaults writes the data built into the binary under dir, as the
// plain files it was compiled from: to read what is built in, or as examples
// for one's own files (see Sources). Never overwrites: if any of the files
// already exists, nothing is written at all. Returns the files written —
// none on failure: what was written before the failure is removed again, so
// that a second attempt isn't refused because of the first one's leftovers.
//
// dir must not be a directory reference files are read from (Sources.Reads,
// for the caller to check): every built-in recipe would come back as the
// user's own, a frozen copy hiding the corrections of later versions.
func ExportDefaults(dir string) (written []string, err error) {
	type export struct {
		path string
		data []byte
	}
	var exports []export
	err = fs.WalkDir(embedded, "data", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := embedded.ReadFile(name)
		if err != nil {
			return err
		}
		relative := filepath.FromSlash(strings.TrimPrefix(name, "data/"))
		exports = append(exports, export{filepath.Join(dir, relative), data})
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, e := range exports {
		if _, err := os.Lstat(e.path); err == nil {
			return nil, fmt.Errorf("%s already exists", e.path)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}

	defer func() {
		if err != nil {
			for _, path := range written {
				_ = os.Remove(path)
			}
			written = nil
		}
	}()
	for _, e := range exports {
		if err := os.MkdirAll(filepath.Dir(e.path), 0o755); err != nil {
			return written, err
		}
		// O_EXCL: still no overwrite if the file appeared since the check.
		f, err := os.OpenFile(e.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return written, err
		}
		// Ours from here on, even if writing it fails.
		written = append(written, e.path)
		_, err = f.Write(e.data)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return written, err
		}
	}
	return written, nil
}
