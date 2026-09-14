// Command docsweb is the docsweb POC's CLI. "build" runs a full build:
// collect targets, validate & classify @uses references, resolve
// @anchor:/@link: destinations, and render the static site (HTML pages
// plus their JSON data API counterpart). "check" runs the same validation
// without rendering anything, for local development and CI pipelines.
package main

// @docsweb
// @define docsweb v0.20.0
// @name docsweb
// @summary
// Write technical documentation where it belongs: besides the code.
// docsweb reads @docsweb annotation blocks out of source-code comments
// and builds a cross-linked static site from them.
// @uses build@v0.18.0
// @uses check@v0.11.0
// @uses pagefind@v0.2.0
// @uses site@v0.18.0
// @audience dev, user
// @changelog
// `docsweb build` now also builds a search index for the generated site's
// new "Search" tab (see [site](@link:site@v0.18.0)'s own changelog):
// after rendering the site, it `Ensure`s a pinned, checksum-verified
// [pagefind](@link:pagefind@v0.2.0) binary (downloaded into
// `docsweb-cache` next to the root config on first use) and runs it
// against the output directory. A new `-search` flag (default `true`)
// skips this step when passed `-search=false`, e.g. to build offline or
// without network access. `@uses` reference bumped to
// [site](@link:site@v0.18.0)'s current version accordingly, and a new one
// added for [pagefind](@link:pagefind@v0.2.0); [build](@link:build@v0.18.0)'s
// and [check](@link:check@v0.11.0)'s are unchanged.
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
// docsweb build [--config .docsweb.yaml] [--out dist] [--search]
// ```
//
// `--config` points at the root `.docsweb.yaml` (see
// [config](@link:config@v0.3.0)); its directory is the root scope's file
// tree, and that config's own required, self-declared `name:` names the
// root scope itself - there is no unscoped default. `--out` is the output
// directory for the generated site (default: `dist`), which holds both the
// HTML pages and their [JSON data API](@link:site@v0.16.0) counterpart.
//
// `--search` (default `true`) builds a full-text search index for the
// generated site's "Search" tab: once every page is written, a pinned
// [pagefind](@link:pagefind@v0.2.0) binary is downloaded (checksum-verified,
// cached under `docsweb-cache` next to `--config` across builds) and run
// against `--out`. Pass `--search=false` to skip this - e.g. building
// offline, or somewhere the pagefind release binary can't be fetched -
// which still produces every other page; the "Search" tab just has nothing
// to search until a later build with the index step enabled runs against
// the same output directory.
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
	"path/filepath"

	"github.com/tfaller/docsweb/internal/build"
	"github.com/tfaller/docsweb/internal/check"
	"github.com/tfaller/docsweb/internal/pagefind"
	"github.com/tfaller/docsweb/internal/site"
)

// cacheDirName mirrors internal/check's own identical constant
// (internal/check/scope.go): a directory named "docsweb-cache", next to the
// root config file, that every docsweb subsystem needing a persistent local
// cache reuses - git scope mirrors there today, the downloaded pagefind
// binary now too, each under its own subdirectory so neither collides with
// the other.
const cacheDirName = "docsweb-cache"

// pagefindVersion and pagefindSHA256 pin the exact pagefind release
// `docsweb build` downloads (via pagefind.Ensure) to index a generated
// site's search tab, and the expected sha256 of every platform's release
// asset docsweb supports - see internal/pagefind's own doc comment for why
// every platform must be pinned here, not just the one this binary happens
// to run on.
const pagefindVersion = "1.5.2"

var pagefindSHA256 = map[string]string{
	"linux/amd64":   "afb824a9e7f64905a934900481cea5be679c03975e527329e0e5e6cc70f5feda",
	"linux/arm64":   "f50ec608bcbf431cebd84e0efa3a5b041ee63df2ad81f138023e0cfd2f509424",
	"darwin/amd64":  "26f51b4ba921897142338fb13b836420696e251fe197b1782ea7981de311156d",
	"darwin/arm64":  "7286f394a349bd37677d44a65a20078a02b1747da0b0814d83403bf86be17abe",
	"windows/amd64": "fab125d5e8e2d3481ffe7d36dec6e101f54a6581cd51cf5e2e09220d4bc78e9c",
	"windows/arm64": "c4d869293a9cd5c14c2dff7e7f568b27a5f4acb1124a2097b638cffc0d82b840",
}

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
	search := fs.Bool("search", true, "build a pagefind search index for the generated site's \"Search\" tab (downloads the pagefind binary into docsweb-cache on first use; pass -search=false to skip, e.g. offline)")
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

	if *search {
		cacheDir := filepath.Join(filepath.Dir(*configPath), cacheDirName)
		bin, err := pagefind.Ensure(cacheDir, pagefind.Options{Version: pagefindVersion, SHA256: pagefindSHA256})
		if err != nil {
			return fmt.Errorf("build: search index: %w", err)
		}
		if err := pagefind.Index(bin, *outDir); err != nil {
			return fmt.Errorf("build: search index: %w", err)
		}
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
