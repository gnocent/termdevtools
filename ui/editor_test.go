package ui

import "testing"

func TestCompletionPrefixAtEndOfLine(t *testing.T) {
	e := NewEditor()
	e.view.SetText("GET _cat/s", true) // curseur en fin de texte

	prefix, start, end, ok := e.CompletionPrefix()
	if !ok {
		t.Fatal("expected ok=true for a method+partial-path line")
	}
	if prefix != "_cat/s" {
		t.Errorf("expected prefix %q, got %q", "_cat/s", prefix)
	}
	if start != 4 || end != 10 {
		t.Errorf("expected range [4,10), got [%d,%d)", start, end)
	}
}

func TestCompletionPrefixEmpty(t *testing.T) {
	e := NewEditor()
	e.view.SetText("GET ", true)

	prefix, _, _, ok := e.CompletionPrefix()
	if !ok {
		t.Fatal("expected ok=true right after the method and a space")
	}
	if prefix != "" {
		t.Errorf("expected empty prefix, got %q", prefix)
	}
}

func TestCompletionPrefixNotAMethodLine(t *testing.T) {
	cases := []string{
		"# un commentaire",
		`{"query": {}}`,
		"  ",
		"GETX _cat/s", // pas une méthode reconnue
	}
	for _, text := range cases {
		e := NewEditor()
		e.view.SetText(text, true)
		if _, _, _, ok := e.CompletionPrefix(); ok {
			t.Errorf("expected ok=false for %q", text)
		}
	}
}

func TestCompletionPrefixSecondLine(t *testing.T) {
	e := NewEditor()
	e.view.SetText("# cheatsheet\nGET _cat/sh", true)

	prefix, start, end, ok := e.CompletionPrefix()
	if !ok {
		t.Fatal("expected ok=true on the second line")
	}
	if prefix != "_cat/sh" {
		t.Errorf("expected prefix %q, got %q", "_cat/sh", prefix)
	}
	// "# cheatsheet\n" fait 13 runes ; "GET " en fait 4 de plus.
	if start != 13+4 {
		t.Errorf("expected start %d, got %d", 13+4, start)
	}
	if end-start != len([]rune(prefix)) {
		t.Errorf("expected end-start == len(prefix), got end=%d start=%d", end, start)
	}
}

func TestApplyCompletion(t *testing.T) {
	e := NewEditor()
	e.view.SetText("GET _cat/s", true)

	_, start, end, ok := e.CompletionPrefix()
	if !ok {
		t.Fatal("expected ok=true")
	}
	e.ApplyCompletion(start, end, "_cat/shards")

	if got := e.Text(); got != "GET _cat/shards" {
		t.Errorf("expected %q, got %q", "GET _cat/shards", got)
	}
}
