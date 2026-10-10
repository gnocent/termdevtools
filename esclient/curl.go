package esclient

import "strings"

// CurlCommand renders a shell command line reproducing the given
// method/path/body request as closely as Execute would send it — same
// auth and TLS handling — for the "copy as cURL" feature (F9, SPEC.md §7
// backlog #3): a working command to replicate a call outside the tool (a
// script, a colleague, a bug report).
//
// The actual secret (password, API key secret, private key passphrase) is
// deliberately replaced with a "<...>" placeholder rather than embedded in
// clear text: the generated command is meant to be copied to the clipboard,
// and from there potentially pasted anywhere — unlike config.yaml (never
// written to disk in the first place, SPEC.md §5/§9.2), a clipboard copy
// has no such guardrail. Everything else — username, API key ID,
// certificate/key paths — is included as-is: the same information already
// persisted, unencrypted, in config.yaml.
func (c *Client) CurlCommand(method, path string, body []byte) string {
	url := c.baseURL + "/" + strings.TrimLeft(path, "/")

	var b strings.Builder
	b.WriteString("curl -X " + strings.ToUpper(method))

	switch c.params.AuthType {
	case AuthBasic:
		b.WriteString(" -u " + shellQuote(c.params.Username+":<password>"))
	case AuthAPIKey:
		b.WriteString(" -H " + shellQuote("Authorization: ApiKey <base64("+c.params.APIKeyID+":<api_key_secret>)>"))
	case AuthMTLS:
		if c.params.ClientCert != "" {
			b.WriteString(" --cert " + shellQuote(c.params.ClientCert))
		}
		if c.params.ClientKey != "" {
			b.WriteString(" --key " + shellQuote(c.params.ClientKey))
		}
		if c.params.KeyPassphrase != "" {
			b.WriteString(" --pass " + shellQuote("<key_passphrase>"))
		}
	}

	if c.params.CAFile != "" {
		b.WriteString(" --cacert " + shellQuote(c.params.CAFile))
	}
	if !c.params.Verify {
		b.WriteString(" -k")
	}

	// A proxy the cluster's own setting imposes or rules out is spelled out.
	// One the environment designates is not: curl reads the same variables,
	// and their value may hold the proxy's credentials.
	switch proxy := strings.TrimSpace(c.params.Proxy); {
	case proxy == "":
	case strings.EqualFold(proxy, ProxyNone):
		b.WriteString(" --noproxy " + shellQuote("*"))
	default:
		b.WriteString(" --proxy " + shellQuote(proxy))
	}

	if len(body) > 0 {
		b.WriteString(" -H " + shellQuote("Content-Type: application/json"))
		b.WriteString(" -d " + shellQuote(string(body)))
	}

	b.WriteString(" " + shellQuote(url))
	return b.String()
}

// shellQuote wraps s in single quotes for safe inclusion in a POSIX shell
// command line (the target of a copy-paste, not necessarily this program's
// own platform — curl commands are conventionally quoted this way
// regardless), escaping any embedded single quote via the standard
// close-quote/escaped-quote/reopen-quote trick: 'it'"'"'s' → it's.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
