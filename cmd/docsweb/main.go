// Command docsweb is the docsweb POC's CLI. Its only command, "build", runs
// a full build: collect targets, validate & classify @uses references,
// resolve @anchor:/@link: destinations, and render the static HTML site.
package main

// @docsweb
// @define docsweb v0.2.0
// @name docsweb
// @summary
// Write technical documentation where it belongs: besides the code.
// docsweb reads @docsweb annotation blocks out of source-code comments
// and builds a cross-linked static HTML site from them.
// @uses build@v0.5.0
// @uses site@v0.3.0
// @audience dev, user
// @changelog
// The `--scope` flag is removed: the root scope's name now comes from the
// root `.docsweb.yaml`'s own required, self-declared `name:` (see
// [build](@link:build@v0.4.0)) - this repo's own config now declares
// `name: docsweb`, so this very site now lives under a `docsweb/` prefix.
// `--scope` is no longer a recognized flag.
// @doc
// # docsweb
//
// Documentation lives next to the code it describes. Annotate a comment
// with a [`@docsweb` block](@link:annotation@v0.1.0#grammar) - `@define`
// a target and version, optionally `@name`/`@summary`/`@audience` it,
// `@uses` other targets to track when they change underneath you, and
// write the actual documentation as Markdown under `@doc`. Everything
// else - cross-linking, outdated-use detection, static site generation -
// is `docsweb build`'s job.
//
// ## Running a build
//
// ```
// docsweb build [--config .docsweb.yaml] [--out dist]
// ```
//
// `--config` points at the root `.docsweb.yaml` (see
// [config](@link:config@v0.2.0)); its directory is the root scope's file
// tree, and that config's own required, self-declared `name:` names the
// root scope itself - there is no unscoped default. `--out` is the output
// directory for the generated site (default: `dist`).
//
// ## This project, dogfed
//
// This very site is docsweb documenting itself: every package under
// `internal/` and this CLI command is a target, `@uses` mirrors the real
// Go import graph between them, and the annotation grammar's own worked
// example lives in [annotation](@link:annotation@v0.1.0). Start at
// [model](@link:model@v0.1.0) for the core types, then
// [collect](@link:collect@v0.1.0) and [config](@link:config@v0.1.0) for
// how a scope is discovered, [mdlink](@link:mdlink@v0.1.0) for how
// `@anchor:`/`@link:` are resolved, and [site](@link:site@v0.1.0) for how
// the result becomes the HTML you're reading now.
// @docsweb

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
	outDir := fs.String("out", "dist", "output directory for the generated site")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := build.Run(build.Options{ConfigPath: *configPath})
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
