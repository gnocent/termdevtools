package parser

import "testing"

const sample = `# cheatsheet
GET _cat/indices?v

POST _search
{
  "query": { "match_all": {} }
}

# une requête sans corps
DELETE my-index
`

func TestParseAll(t *testing.T) {
	reqs := ParseAll(sample)
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requests, got %d: %+v", len(reqs), reqs)
	}

	if reqs[0].Method != "GET" || reqs[0].Path != "_cat/indices?v" || reqs[0].Body != nil {
		t.Errorf("unexpected first request: %+v", reqs[0])
	}

	if reqs[1].Method != "POST" || reqs[1].Path != "_search" {
		t.Errorf("unexpected second request: %+v", reqs[1])
	}
	if err := ValidateBody(reqs[1].Body); err != nil {
		t.Errorf("expected valid JSON body, got error: %v (body=%q)", err, reqs[1].Body)
	}

	if reqs[2].Method != "DELETE" || reqs[2].Path != "my-index" || reqs[2].Body != nil {
		t.Errorf("unexpected third request: %+v", reqs[2])
	}
}

func TestRequestAtLine(t *testing.T) {
	// Ligne du corps JSON de la deuxième requête (index 5 = `"query": ...`)
	r, err := RequestAtLine(sample, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Method != "POST" {
		t.Errorf("expected POST request at line 5, got %+v", r)
	}

	// Ligne vide entre deux requêtes (index 2) -> doit retomber sur la requête précédente (GET)
	r, err = RequestAtLine(sample, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Method != "GET" {
		t.Errorf("expected fallback to GET request at line 2, got %+v", r)
	}
}

func TestValidateBodyInvalid(t *testing.T) {
	if err := ValidateBody([]byte(`{"a":}`)); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
