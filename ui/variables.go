package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// variableRe matches a "${name}" reference — name restricted to a
// shell/identifier-like charset (letters, digits, underscore, not starting
// with a digit) so it can't accidentally swallow unrelated "${...}" text
// (e.g. a literal template placeholder pasted from elsewhere).
var variableRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// substituteVariables replaces every "${name}" reference in text with its
// value from vars. A reference whose name isn't in vars is left as-is in
// result and its name added to missing (deduplicated, in first-appearance
// order) — the caller decides whether that's fatal (see App.resolveRequest);
// substituteVariables itself never errors.
func substituteVariables(text string, vars map[string]string) (result string, missing []string) {
	seen := make(map[string]bool)
	result = variableRe.ReplaceAllStringFunc(text, func(match string) string {
		name := match[2 : len(match)-1] // strip the surrounding "${" and "}"
		if v, ok := vars[name]; ok {
			return v
		}
		if !seen[name] {
			seen[name] = true
			missing = append(missing, name)
		}
		return match
	})
	return result, missing
}

// parseVariables reads a variables file's content: one "name=value" pair
// per line (split on the first "=" only, so a value may itself contain
// "="), blank lines and lines starting with "#" ignored — same convention
// as the reference files (endpoints.txt, recipes; SPEC.md §9.1).
func parseVariables(data []byte) map[string]string {
	vars := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		name, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		vars[name] = strings.TrimSpace(value)
	}
	return vars
}

// defaultVariablesFileContent is the commented starter file created the
// first time a cluster is connected to and has no variables file yet (see
// LoadVariablesFile) — explains the feature in place, the same
// self-documenting approach as config.yaml (config.WriteDefaultConfigFile).
const defaultVariablesFileContent = `# Reusable variables for this cluster — one per line, "name=value".
# Refer to them in your requests (URL or JSON body) as ${name}: the value is
# substituted right before execution (Ctrl+E) or when the cURL command is
# generated (F9) — never written into the save of your requests itself,
# which keeps ${name} as is.
#
# A line starting with # is a comment (ignored).
# Reload this file without restarting the program with F7.
#
# The recipes of the catalog (F8) use these names: index, node, repository,
# snapshot, field, task_id.
#
# Example:
# index=my-index-000001
`

// LoadVariablesFile reads the variables file at path (see
// config.VariablesPathForURL). If it doesn't exist yet, a commented
// starter file is created (best-effort — a write failure here isn't fatal,
// it just means nothing gets pre-seeded) and an empty map is returned, no
// error either way: no variables configured is a perfectly normal state,
// not a problem to report.
func LoadVariablesFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if mkErr := os.MkdirAll(filepath.Dir(path), 0o700); mkErr == nil {
			_ = os.WriteFile(path, []byte(defaultVariablesFileContent), 0o600)
		}
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return parseVariables(data), nil
}
