// Command docsweb is the docsweb POC's CLI. "build" runs a full build:
// collect targets, validate & classify @uses references, resolve
// @anchor:/@link: destinations, and render the static HTML site. "check"
// runs the same validation without rendering anything, for local
// development and CI pipelines.
package main

// @docsweb
// @define docsweb v0.8.0
// @name docsweb
// @summary
// Write technical documentation where it belongs: besides the code.
// docsweb reads @docsweb annotation blocks out of source-code comments
// and builds a cross-linked static HTML site from them.
// @uses build@v0.13.0
// @uses check@v0.7.0
// @uses site@v0.7.0
// @audience dev, user
// @changelog
// No behavior change to this CLI itself - `@uses` references bumped to
// [build](@link:build@v0.13.0)/[check](@link:check@v0.7.0)/
// [site](@link:site@v0.7.0)'s current versions following
// `vcs.Repository`'s path-handling unification rippling through the whole
// dependency chain.
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
// [config](@link:config@v0.3.0)); its directory is the root scope's file
// tree, and that config's own required, self-declared `name:` names the
// root scope itself - there is no unscoped default. `--out` is the output
// directory for the generated site (default: `dist`).
//
// ## Checking without building
//
// ```
// docsweb check [--config .docsweb.yaml] [--base <rev>]
// ```
//
// `check` runs every validation [build](@link:build@v0.9.0) does - the
// same [checks](@link:check@v0.4.0) - plus one it doesn't: that a target
// whose documentation changed since a comparison base also bumped its
// version and changelog. Nothing is ever rendered to HTML or written to
// disk, and there is no `--out` flag. Use it as a fast local/CI gate to
// confirm a change hasn't broken anything - and hasn't silently skipped
// updating its own docs - before running a real build. `--base` overrides
// which revision that last check diffs against; left unset, it
// auto-detects a GitLab/GitHub merge/pull-request pipeline's target branch,
// falling back to `HEAD`.
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
	"github.com/tfaller/docsweb/internal/check"
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
		return fmt.Errorf(`expected a command, e.g. "docsweb build" or "docsweb check"`)
	}

	switch args[0] {
	case "build":
		return runBuild(args[1:])
	case "check":
		return runCheck(args[1:])
	default:
		return fmt.Errorf(`unknown command %q (only "build" and "check" are supported)`, args[0])
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

// runCheck runs the same validation runBuild does, without ever rendering a
// target's Markdown to HTML or writing anything to disk - see
// internal/check. Intended for local development and CI pipelines that just
// want to confirm a change hasn't broken anything, without paying for (or
// needing to point at an output directory for) a full build.
func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	configPath := fs.String("config", ".docsweb.yaml", "path to the root .docsweb.yaml")
	base := fs.String("base", "", "revision to diff documentation against for the version/changelog-bump check (default: auto-detected merge base in a GitLab/GitHub merge/pull-request pipeline, else HEAD)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := check.Run(check.Options{ConfigPath: *configPath, Base: *base})
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}

	fmt.Printf("docsweb: checked %d target(s), %d outdated use(s), OK\n",
		len(result.Registry.Targets()), len(result.Issues))
	return nil
}
