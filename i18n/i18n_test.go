package i18n

import (
	"reflect"
	"regexp"
	"testing"
)

// formatVerbRe finds the verbs of a format string ("%s", "%d", "%-10v"...),
// "%%" excluded.
var formatVerbRe = regexp.MustCompile(`%[-+# 0]*[0-9]*(?:\.[0-9]+)?[a-zA-Z]`)

// TestCatalogsAreComplete checks what the compiler can't: that no message is
// left empty in either language — a field added to Strings and given a value
// in one catalog only — and that a message takes the same arguments, in the
// same order, in both: its call site formats them identically.
func TestCatalogsAreComplete(t *testing.T) {
	french, english := reflect.ValueOf(fr), reflect.ValueOf(en)
	fields := french.Type()

	for i := 0; i < fields.NumField(); i++ {
		name := fields.Field(i).Name
		if fields.Field(i).Type.Kind() != reflect.String {
			t.Errorf("%s: unexpected non-string field", name)
			continue
		}
		frText, enText := french.Field(i).String(), english.Field(i).String()
		if frText == "" {
			t.Errorf("%s: no French text", name)
		}
		if enText == "" {
			t.Errorf("%s: no English text", name)
		}
		frVerbs, enVerbs := formatVerbRe.FindAllString(frText, -1), formatVerbRe.FindAllString(enText, -1)
		if !reflect.DeepEqual(frVerbs, enVerbs) {
			t.Errorf("%s: format verbs differ between French %v and English %v", name, frVerbs, enVerbs)
		}
	}
}

// TestForFallsBackToEnglish checks the interface's default language: English
// for anything that isn't French, a setting left out or mistyped included.
func TestForFallsBackToEnglish(t *testing.T) {
	if For("fr") != &fr || For("FR") != &fr || For(" fr ") != &fr {
		t.Error("expected the French catalog for \"fr\", whatever its case or padding")
	}
	for _, lang := range []string{"", "en", "EN", "de"} {
		if For(lang) != &en {
			t.Errorf("expected the English catalog for %q", lang)
		}
	}
}
