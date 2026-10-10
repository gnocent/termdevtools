// Package testclusters names the real clusters the reference data is
// generated from and checked against: the single-node containers started by
// tools/testclusters.sh. Not used by the application itself.
package testclusters

import (
	"fmt"
	"os"
	"strings"

	"termdevtools/refdata"
)

// EnvVar overrides the default list: "name=url,name=url...", each name being
// "es-<version>" or "os-<version>".
const EnvVar = "TDT_IT_TARGETS"

// Cluster is one cluster to query.
type Cluster struct {
	// Name is "es-8.19.22" or "os-2.19.6": the distribution and version the
	// cluster is expected to report.
	Name string
	URL  string
}

// Target is what the cluster's name says it is.
func (c Cluster) Target() (refdata.Target, error) {
	prefix, version, found := strings.Cut(c.Name, "-")
	v, ok := refdata.ParseVersion(version)
	if !found || !ok {
		return refdata.Target{}, fmt.Errorf("cluster name %q: expected es-<version> or os-<version>", c.Name)
	}
	switch prefix {
	case "es":
		return refdata.Target{Distribution: refdata.Elasticsearch, Version: v, HasVersion: true}, nil
	case "os":
		return refdata.Target{Distribution: refdata.OpenSearch, Version: v, HasVersion: true}, nil
	}
	return refdata.Target{}, fmt.Errorf("cluster name %q: expected es-<version> or os-<version>", c.Name)
}

// defaults mirrors the list in tools/testclusters.sh (a test keeps the two
// in step): for each distribution, the first and last minor of every major
// the reference data covers, plus one in the middle of the longest lines.
var defaults = []Cluster{
	{"es-7.17.29", "http://localhost:19217"},
	{"es-8.0.1", "http://localhost:19280"},
	{"es-8.11.4", "http://localhost:19281"},
	{"es-8.19.22", "http://localhost:19289"},
	{"es-9.0.8", "http://localhost:19290"},
	{"es-9.5.4", "http://localhost:19295"},
	{"os-2.0.1", "http://localhost:19320"},
	{"os-2.11.1", "http://localhost:19321"},
	{"os-2.19.6", "http://localhost:19329"},
	{"os-3.0.0", "http://localhost:19330"},
	{"os-3.9.0", "http://localhost:19339"},
}

// Secured is the one cluster started with security on, for what the others
// can't check: authentication. Its password protects nothing — the container
// holds no data and is only reachable from this machine. Mirrors
// tools/testclusters.sh like the list above, and is not part of it: the
// reference data tests have no credentials to give.
var Secured = SecuredCluster{
	Name:     "secured-es-9.5.4",
	URL:      "http://localhost:19395",
	Username: "elastic",
	Password: "tdt-throwaway",
}

// SecuredCluster is a cluster to query with a user name and password.
type SecuredCluster struct {
	Name     string
	URL      string
	Username string
	Password string
}

// FromEnv returns the clusters named by EnvVar, or the default list if it
// isn't set.
func FromEnv() ([]Cluster, error) {
	return Parse(os.Getenv(EnvVar))
}

// Parse reads a "name=url,name=url" list; empty means the default list.
func Parse(spec string) ([]Cluster, error) {
	if strings.TrimSpace(spec) == "" {
		return append([]Cluster(nil), defaults...), nil
	}
	var clusters []Cluster
	for _, item := range strings.Split(spec, ",") {
		name, url, found := strings.Cut(strings.TrimSpace(item), "=")
		if !found || name == "" || url == "" {
			return nil, fmt.Errorf("%q: expected name=url", item)
		}
		c := Cluster{Name: name, URL: strings.TrimRight(url, "/")}
		if _, err := c.Target(); err != nil {
			return nil, err
		}
		clusters = append(clusters, c)
	}
	return clusters, nil
}
