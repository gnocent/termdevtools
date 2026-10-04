package testclusters

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"termdevtools/refdata"
)

// TestDefaultsMatchScript keeps the default list in step with the containers
// tools/testclusters.sh actually starts.
func TestDefaultsMatchScript(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "tools", "testclusters.sh"))
	if err != nil {
		t.Fatal(err)
	}
	// Lines of the form:  "es-7.17.29|docker.elastic.co/...:7.17.29|19217"
	matches := regexp.MustCompile(`"((?:es|os)-[0-9.]+)\|[^|"]+\|([0-9]+)"`).FindAllStringSubmatch(string(script), -1)
	if len(matches) != len(defaults) {
		t.Fatalf("the script lists %d clusters, the Go defaults %d", len(matches), len(defaults))
	}
	for i, m := range matches {
		want := Cluster{Name: m[1], URL: "http://localhost:" + m[2]}
		if defaults[i] != want {
			t.Errorf("entry %d: script says %+v, Go defaults say %+v", i, want, defaults[i])
		}
	}
}

func TestParse(t *testing.T) {
	got, err := Parse(" es-8.19.22=http://a:9200/ , os-2.19.6=http://b:9200 ")
	if err != nil {
		t.Fatal(err)
	}
	want := []Cluster{{"es-8.19.22", "http://a:9200"}, {"os-2.19.6", "http://b:9200"}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %+v, want %+v", got, want)
	}

	if got, err := Parse(""); err != nil || len(got) != len(defaults) {
		t.Errorf("empty spec should give the defaults, got %+v, %v", got, err)
	}
	for _, bad := range []string{"es-8.19.22", "=http://a", "solr-9=http://a", "es-x=http://a"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) should have failed", bad)
		}
	}
}

func TestClusterTarget(t *testing.T) {
	got, err := Cluster{Name: "os-2.19.6"}.Target()
	want := refdata.Target{Distribution: refdata.OpenSearch, Version: refdata.Version{Major: 2, Minor: 19, Patch: 6}, HasVersion: true}
	if err != nil || got != want {
		t.Errorf("got %+v, %v; want %+v", got, err, want)
	}
}
