// Command genrefdata regenerates the reference data embedded in the binary
// (refdata/data). Run from the repository root:
//
//	go run ./tools/genrefdata endpoints     from the official API specifications
//	go run ./tools/genrefdata catcolumns    from real clusters (tools/testclusters.sh up)
//	go run ./tools/genrefdata explain _ilm  what the specifications say about matching paths
//
// What it writes is then checked against real clusters by the integration
// tests (go test -tags integration ./refdata/).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "genrefdata:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: genrefdata endpoints|catcolumns|explain [flags] [path fragments]")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	out := fs.String("out", filepath.Join("refdata", "data"), "directory of the embedded data files")
	specs := fs.String("specs", filepath.Join(os.TempDir(), "termdevtools-specs"), "directory caching the downloaded API specifications (endpoints)")
	targets := fs.String("targets", "", "clusters to query, as name=url,... (catcolumns; default: those of tools/testclusters.sh)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	switch args[0] {
	case "endpoints":
		return generateEndpoints(*specs, filepath.Join(*out, "endpoints.txt"))
	case "explain":
		return explainEndpoints(*specs, fs.Args())
	case "catcolumns":
		return generateCatColumns(*targets, filepath.Join(*out, "cat_columns.txt"))
	default:
		return fmt.Errorf("unknown command %q (expected endpoints, catcolumns or explain)", args[0])
	}
}
