// Package parser découpe le contenu de l'éditeur (panneau gauche) en
// requêtes individuelles — une ligne "MÉTHODE endpoint" suivie d'un corps
// JSON optionnel — et retrouve la requête sous le curseur. Voir SPEC.md §3.2.
package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrNoRequest indique qu'aucune requête n'a pu être trouvée dans le buffer.
var ErrNoRequest = errors.New("aucune requête trouvée")

var requestLineRe = regexp.MustCompile(`^(?i)(GET|POST|PUT|DELETE)\s+(\S+)\s*$`)

// Request est une requête extraite de l'éditeur.
type Request struct {
	Method string
	Path   string
	Body   []byte // nil si la requête n'a pas de corps

	// StartLine/EndLine sont les index de ligne (0-indexés) couverts par
	// cette requête dans le buffer d'origine, ligne de méthode incluse.
	StartLine int
	EndLine   int
}

// ParseAll découpe l'intégralité du buffer en requêtes, dans l'ordre
// d'apparition. Les lignes vides ou commençant par '#' sont ignorées en
// dehors d'un corps JSON ; les lignes non reconnues comme méthode sont
// simplement sautées (tolérant, pour ne pas bloquer sur un texte libre).
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

// collectBody consomme, à partir de start, le corps JSON éventuel d'une
// requête (équilibrage des accolades, en ignorant celles présentes dans des
// chaînes) et renvoie le corps ainsi que l'index de la ligne suivante non
// consommée.
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
				break // pas de corps pour cette requête
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

// RequestAtLine renvoie la requête à laquelle appartient cursorLine (index
// de ligne 0-indexé). Si le curseur ne tombe dans aucune requête (ligne
// vide/commentaire entre deux requêtes), la requête la plus proche
// précédant le curseur est utilisée ; à défaut, la première du buffer.
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

// ValidateBody vérifie que body est un JSON syntaxiquement valide (ou vide).
func ValidateBody(body []byte) error {
	if len(body) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(body, &v); err != nil {
		return fmt.Errorf("JSON invalide : %w", err)
	}
	return nil
}
