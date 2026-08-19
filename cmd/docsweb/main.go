// Command docsweb is the docsweb POC's CLI. Its only command, "build", runs
// a full build: collect targets, validate & classify @uses references,
// resolve @anchor:/@link: destinations, and render the static HTML site.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tfaller/docsweb/internal/build"
	"github.com/tfaller/docsweb/internal/site"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "docsweb:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("expected a command, e.g. \"docsweb build\"")
	}

	switch args[0] {
	case "build":
		return runBuild(args[1:])
	default:
		return fmt.Errorf("unknown command %q (only \"build\" is supported)", args[0])
	}
}

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	configPath := fs.String("config", ".docsweb.yaml", "path to the root .docsweb.yaml")
	rootScope := fs.String("scope", "", "name of the root scope (files directly under the config's directory)")
	outDir := fs.String("out", "dist", "output directory for the generated site")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := build.Run(build.Options{ConfigPath: *configPath, RootScope: *rootScope})
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	if err := site.Generate(result, *outDir); err != nil {
		return fmt.Errorf("build: %w", err)
	}

	fmt.Printf("docsweb: built %d target(s), %d outdated use(s), into %s\n",
		len(result.Targets), len(result.Issues), *outDir)
	return nil
}
