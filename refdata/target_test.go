package refdata

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectTargetRealClusters runs detection over "GET /" bodies captured
// from real single-node clusters (tools/testclusters.sh): every supported
// line, plus OpenSearch in 7.10.2 compatibility mode.
func TestDetectTargetRealClusters(t *testing.T) {
	cases := []struct {
		file string
		want Target
	}{
		{"es-7.17.29.json", Target{Distribution: Elasticsearch, Version: Version{7, 17, 29}, HasVersion: true}},
		{"es-8.0.1.json", Target{Distribution: Elasticsearch, Version: Version{8, 0, 1}, HasVersion: true}},
		{"es-8.19.22.json", Target{Distribution: Elasticsearch, Version: Version{8, 19, 22}, HasVersion: true}},
		{"es-9.0.8.json", Target{Distribution: Elasticsearch, Version: Version{9, 0, 8}, HasVersion: true}},
		{"es-9.5.4.json", Target{Distribution: Elasticsearch, Version: Version{9, 5, 4}, HasVersion: true}},
		{"os-2.0.1.json", Target{Distribution: OpenSearch, Version: Version{2, 0, 1}, HasVersion: true}},
		{"os-2.19.6.json", Target{Distribution: OpenSearch, Version: Version{2, 19, 6}, HasVersion: true}},
		{"os-3.0.0.json", Target{Distribution: OpenSearch, Version: Version{3, 0, 0}, HasVersion: true}},
		{"os-3.9.0.json", Target{Distribution: OpenSearch, Version: Version{3, 9, 0}, HasVersion: true}},
		// The reported 7.10.2 is a deliberate lie: the version must be dropped.
		{"os-2.19.6-compat.json", Target{Distribution: OpenSearch}},
	}
	for _, c := range cases {
		body, err := os.ReadFile(filepath.Join("testdata", "root", c.file))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if got := DetectTarget(body); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.file, got, c.want)
		}
	}
}

func TestDetectTargetUnrecognized(t *testing.T) {
	cases := map[string]string{
		"not JSON (a proxy's HTML page)": "<html><body>Bad gateway</body></html>",
		"empty":                          "",
		"JSON without the expected keys": `{"status": "ok"}`,
		"another fork":                   `{"version": {"distribution": "easysearch", "number": "1.9.0"}, "tagline": "You Know, For Easy Search!"}`,
	}
	for name, body := range cases {
		if got := DetectTarget([]byte(body)); got != (Target{}) {
			t.Errorf("%s: got %+v, want the zero Target", name, got)
		}
	}
}

func TestDetectTargetServerless(t *testing.T) {
	body := `{"version": {"number": "8.11.0", "build_flavor": "serverless"}, "tagline": "You Know, for Search"}`
	want := Target{Distribution: Elasticsearch, Serverless: true}
	if got := DetectTarget([]byte(body)); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseVersion(t *testing.T) {
	valid := map[string]Version{
		"9":              {9, 0, 0},
		"8.19":           {8, 19, 0},
		"8.19.22":        {8, 19, 22},
		"9.6.0-SNAPSHOT": {9, 6, 0},
		" 2.4 ":          {2, 4, 0},
	}
	for in, want := range valid {
		got, ok := ParseVersion(in)
		if !ok || got != want {
			t.Errorf("ParseVersion(%q) = %+v, %v; want %+v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "abc", "8.x", "1.2.3.4", "-1"} {
		if _, ok := ParseVersion(in); ok {
			t.Errorf("ParseVersion(%q) should have failed", in)
		}
	}
}

func TestVersionCompareAndString(t *testing.T) {
	if (Version{8, 19, 0}).Compare(Version{9, 0, 0}) >= 0 {
		t.Error("8.19 should sort before 9.0")
	}
	if (Version{8, 2, 0}).Compare(Version{8, 19, 0}) >= 0 {
		t.Error("8.2 should sort before 8.19 (numeric, not lexical)")
	}
	if (Version{8, 19, 1}).Compare(Version{8, 19, 1}) != 0 {
		t.Error("equal versions should compare equal")
	}
	if got := (Version{8, 19, 0}).String(); got != "8.19" {
		t.Errorf("String() = %q, want 8.19", got)
	}
	if got := (Version{8, 19, 22}).String(); got != "8.19.22" {
		t.Errorf("String() = %q, want 8.19.22", got)
	}
}

func TestTargetLabels(t *testing.T) {
	cases := []struct {
		target      Target
		label, name string
	}{
		{Target{}, "", ""},
		{Target{Distribution: Elasticsearch, Version: Version{9, 5, 4}, HasVersion: true}, "ES 9.5.4", "Elasticsearch 9.5.4"},
		{Target{Distribution: OpenSearch, Version: Version{2, 19, 0}, HasVersion: true}, "OS 2.19.0", "OpenSearch 2.19.0"},
		{Target{Distribution: OpenSearch}, "OS", "OpenSearch"},
		{Target{Distribution: Elasticsearch, Serverless: true}, "ES Serverless", "Elasticsearch Serverless"},
	}
	for _, c := range cases {
		if got := c.target.Label(); got != c.label {
			t.Errorf("Label(%+v) = %q, want %q", c.target, got, c.label)
		}
		if got := c.target.Name(); got != c.name {
			t.Errorf("Name(%+v) = %q, want %q", c.target, got, c.name)
		}
	}
}

func TestWithOverride(t *testing.T) {
	detected := Target{Distribution: Elasticsearch, Version: Version{8, 19, 22}, HasVersion: true}

	got, err := detected.WithOverride("", "")
	if err != nil || got != detected {
		t.Errorf("no override: got %+v, %v; want the detected target unchanged", got, err)
	}

	got, err = detected.WithOverride("", "8.5")
	want := Target{Distribution: Elasticsearch, Version: Version{8, 5, 0}, HasVersion: true}
	if err != nil || got != want {
		t.Errorf("version only: got %+v, %v; want %+v", got, err, want)
	}

	// Same distribution as detected: the detected version stays.
	got, err = detected.WithOverride("Elasticsearch", "")
	if err != nil || got != detected {
		t.Errorf("same distribution: got %+v, %v; want %+v", got, err, detected)
	}

	// A different distribution invalidates the detected version.
	got, err = detected.WithOverride("opensearch", "")
	want = Target{Distribution: OpenSearch}
	if err != nil || got != want {
		t.Errorf("other distribution: got %+v, %v; want %+v", got, err, want)
	}

	got, err = Target{}.WithOverride("opensearch", "2.11")
	want = Target{Distribution: OpenSearch, Version: Version{2, 11, 0}, HasVersion: true}
	if err != nil || got != want {
		t.Errorf("full override: got %+v, %v; want %+v", got, err, want)
	}

	if _, err := detected.WithOverride("solr", ""); err == nil {
		t.Error("an unknown distribution should be rejected")
	}
	if _, err := detected.WithOverride("", "latest"); err == nil {
		t.Error("an unparseable version should be rejected")
	}
}
