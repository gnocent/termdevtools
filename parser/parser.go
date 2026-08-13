// Package parser splits the editor content (left panel) into individual
// requests — a "METHOD endpoint" line followed by an optional JSON body —
// and locates the request under the cursor. See SPEC.md §3.2.
package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrNoRequest indicates that no request could be found in the buffer.
var ErrNoRequest = errors.New("no request found")

var requestLineRe = regexp.MustCompile(`^(?i)(GET|POST|PUT|DELETE)\s+(\S+)\s*$`)

// Request is a request extracted from the editor.
type Request struct {
	Method string
	Path   string
	Body   []byte // nil if the request has no body

	// StartLine/EndLine are the (0-indexed) line indices covered by this
	// request in the original buffer, including the method line.
	StartLine int
	EndLine   int
}

// ParseAll splits the whole buffer into requests, in order of appearance.
// Empty lines or lines starting with '#' are ignored outside of a JSON body;
// lines not recognized as a method line are simply skipped (lenient, so as
// not to choke on free-form text).
func ParseAll(text string) []Request {
	lines := strings.Split(text, "\n")
	var requests []Request

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}

		method, path, ok := parseRequestLine(trimmed)
		if !ok {
			i++
			continue
		}

		start := i
		i++
		body, next := collectBody(lines, i)
		i = next

		end := i - 1
		if end < start {
			end = start
		}

		requests = append(requests, Request{
			Method:    method,
			Path:      path,
			Body:      body,
			StartLine: start,
			EndLine:   end,
		})
	}

	return requests
}

// collectBody consumes, starting at start, a request's optional JSON body
// (brace balancing, ignoring braces found inside strings) and returns the
// body along with the index of the next unconsumed line.
func collectBody(lines []string, start int) (body []byte, next int) {
	i := start
	started := false
	depth := 0
	inString := false
	escape := false
	var bodyLines []string

	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])

		if !started {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				i++
				continue
			}
			if !strings.HasPrefix(trimmed, "{") {
				break // no body for this request
			}
			started = true
		} else if strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}

		bodyLines = append(bodyLines, lines[i])
		for _, r := range lines[i] {
			if escape {
				escape = false
				continue
			}
			switch r {
			case '\\':
				if inString {
					escape = true
				}
			case '"':
				inString = !inString
			case '{':
				if !inString {
					depth++
				}
			case '}':
				if !inString {
					depth--
				}
			}
		}
		i++
		if started && depth <= 0 {
			break
		}
	}

	if len(bodyLines) == 0 {
		return nil, i
	}
	return []byte(strings.Join(bodyLines, "\n")), i
}

func parseRequestLine(line string) (method, path string, ok bool) {
	m := requestLineRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return strings.ToUpper(m[1]), m[2], true
}

// RequestAtLine returns the request cursorLine (0-indexed line index)
// belongs to. If the cursor doesn't fall inside any request (blank/comment
// line between two requests), the closest request preceding the cursor is
// used; failing that, the first one in the buffer.
func RequestAtLine(text string, cursorLine int) (*Request, error) {
	requests := ParseAll(text)
	if len(requests) == 0 {
		return nil, ErrNoRequest
	}

	var candidate *Request
	for idx := range requests {
		r := &requests[idx]
		if cursorLine >= r.StartLine && cursorLine <= r.EndLine {
			return r, nil
		}
		if r.StartLine <= cursorLine {
			candidate = r
		}
	}
	if candidate != nil {
		return candidate, nil
	}
	return &requests[0], nil
}

// ValidateBody checks that body is syntactically valid JSON (or empty).
func ValidateBody(body []byte) error {
	if len(body) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(body, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
