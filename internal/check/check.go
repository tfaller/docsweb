// Package check runs every validation a docsweb pipeline needs before its
// data is in shape to render correctly: config/scope collection, @audience
// mapping, @uses resolution, @anchor uniqueness, and @link resolution. It
// never renders a single piece of Markdown to HTML or writes anything to
// disk - see internal/build for the additional rendering step layered on
// top of a successful Result.
package check

// @docsweb
// @define check v0.10.0
// @name Check
// @summary
// Runs every validation a docsweb pipeline needs - scope collection
// (opening any remote scope's resolved commit first, authenticated via
// internal/auth when needed), audience mapping, use resolution, anchor
// uniqueness, link resolution, and (docsweb-check-only) that documented
// changes bump a target's version and changelog - without rendering any
// Markdown to HTML.
// @uses annotation@v0.2.0
// @uses auth@v0.1.0
// @uses collect@v0.6.0
// @uses config@v0.3.2
// @uses ignore@v0.1.0
// @uses mdlink@v0.2.0
// @uses vcs@v0.7.0
// @uses model@v0.3.0
// @audience dev
// @changelog
// Fixed **checkScopes**: a local referenced scope's tree was filtered by
// the *root* config's `ignore:` rules (re-anchored to the scope's own
// subdirectory) instead of the referenced scope's own `.docsweb.yaml`
// `ignore:` list, which was never applied at all - the reverse of a
// remote (`git:`) scope, whose own `ignore:` list was also never applied
// (only the root's, which doesn't even describe that repository). Every
// referenced scope - local or remote - is now walked with only its own
// `ignore:` rules; the root config's list is applied only to the root
// scope's own tree, matching README.md's "Scopes" section, which already
// documented this as the intended contract.
// @doc
// # Check
//
// `check` is docsweb's validation pipeline, factored out of
// [build](@link:build@v0.9.0) so it can run on its own - via `docsweb
// check` - without ever rendering a target's Markdown to HTML or writing
// a static site to disk. `build` layers rendering and git-blame
// attribution on top of the same `Result` this package produces, so the
// two commands never validate a config two different ways.
//
// ## Checks
//
// Validation is a fixed, ordered list of small, independently testable
// [Check](@anchor:checktype)s, each tagged with the [Phase](@anchor:phase)
// it applies to:
//
// - **scopes** - loads the root `.docsweb.yaml`, opens/fetches any
//   declared `git:` scope's resolved commit via
//   [vcs.OpenScope](@link:vcs@v0.7.0) (a bare mirror under `docsweb-cache`,
//   no worktree checkout, authenticated via [auth](@link:auth@v0.1.0) when
//   a registered provider recognizes the scope's URL), verifies every
//   declared referenced scope's own config against the parent's `scope:`
//   key (see [config](@link:config@v0.3.2)'s "Scopes" section), and walks
//   every scope's file tree via [collect](@link:collect@v0.6.0). Each
//   scope - root or referenced, local or remote - is walked with only its
//   own `.docsweb.yaml`'s `ignore:` rules: the root config's list is never
//   applied to a referenced scope's tree, and a referenced scope's own
//   list is never applied outside of it.
// - **audiences** - validates every target's (and changelog entry's)
//   `@audience` names against the config's declared `audience:` map.
// - **uses** - validates that every `@uses` lands on an existing target
//   and classifies outdated ones.
// - **anchors** - collects every target's `@anchor:` declarations and
//   rejects duplicates within a target.
// - **links** - validates that every `@link` reference (and its optional
//   `#anchor`) resolves to something real, by running
//   [mdlink](@link:mdlink@v0.1.0)'s `Preprocess` over every Markdown piece
//   and discarding the result - the one step that would otherwise require
//   actually rendering a page.
// - **versionbump** (`CheckOnly`) - via [vcs](@link:vcs@v0.4.0), diffs every
//   target's documentation against a comparison base commit (auto-detected
//   CI merge/pull-request base, an explicit `Options.Base`, or `HEAD`),
//   ignoring incidental whitespace, and requires that a documented target
//   whose content changed since that base also bumped its `@define`
//   version and updated its `@changelog` to genuinely new text - not the
//   previous entry with something appended or prepended to it.
//
// Each check's `Phase` says whether it applies to `docsweb
// check` (`Run`), `docsweb build` (`RunForBuild`), or both - most checks
// are correctness requirements that apply either way, but the split lets
// a future check be scoped to only the command where it makes sense:
// **versionbump** is `CheckOnly` because it's a process/review gate, not a
// prerequisite for a build to render correctly, and because it depends on
// git history that a build run from a plain source tree (no `.git`, e.g. an
// extracted archive) may not have.
// @docsweb

import (
	"fmt"

	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/config"
	"github.com/tfaller/docsweb/internal/vcs"
)

// Options configures a validation pass against a root .docsweb.yaml.
type Options struct {
	// ConfigPath is the path to the root .docsweb.yaml. Its directory is
	// the root scope's file tree; scopes it declares are read relative to
	// that directory, per README.md's "Scopes" section. The root scope's
	// own name comes from that config's own self-declared `name:` field.
	ConfigPath string
	// Base optionally overrides the revision checkVersionBump diffs every
	// target's documentation against (see baseRevision) - a commit SHA,
	// branch name, tag, or anything else "git.Repository.ResolveRevision"
	// accepts. Empty auto-detects: the merge base against a GitLab
	// merge-request/GitHub pull-request's target branch when running in
	// that CI pipeline, else the current HEAD.
	Base string
}

// RemoteScope is a resolved remote (git:) scope's pinned repository and its
// own path within that repository's tree - both needed by internal/build's
// git-blame attribution, since a remote scope has no on-disk checkout for
// vcs.Open to discover the way a local scope's ScopeRoots entry does.
type RemoteScope struct {
	Repo *vcs.Repository
	Path string
}

// Result is everything a caller needs once every check has passed: the
// fully collected target registry, plus the data later pipeline steps
// (rendering, site generation) build on top of - without any Markdown
// ever having been rendered to HTML.
type Result struct {
	Config     *config.Config
	RootDir    string
	ScopeRoots map[string]string
	// RemoteScopes carries every remote (git:) scope's own RemoteScope,
	// keyed by scope name - the counterpart to ScopeRoots for a scope with
	// no on-disk root at all.
	RemoteScopes map[string]RemoteScope
	Registry     *collect.Registry
	Anchors      map[string]map[string]bool
	UsedBy       map[string][]UsedByRef
	Issues       []UsageIssue
}

// context carries state between checks as the pipeline runs. Order matters:
// a check may depend on fields an earlier check populated (e.g. checkUses
// looks targets up in ctx.registry, which only checkScopes populates).
type context struct {
	opts Options

	cfg          *config.Config
	rootDir      string
	scopeRoots   map[string]string
	remoteScopes map[string]RemoteScope
	registry     *collect.Registry
	anchors      map[string]map[string]bool
	issues       []UsageIssue
}

// Phase says which command(s) a Check applies to.
type Phase int

const (
	// Both runs during "docsweb check" and "docsweb build" - the default
	// for anything that must hold before a build is allowed to render.
	Both Phase = iota
	// BuildOnly runs only as part of "docsweb build".
	BuildOnly
	// CheckOnly runs only as part of "docsweb check".
	CheckOnly
)

// Check is one named validation step run against the pipeline's shared
// context, either erroring or populating context fields (or both) for
// later checks/the final Result to use.
type Check struct {
	Name  string
	Phase Phase
	Run   func(*context) error
}

// checks is the ordered, fixed list of every check the pipeline runs.
// Additional checks - including ones scoped to only "check" or only
// "build" via Phase - are added here.
var checks = []Check{
	{Name: "scopes", Phase: Both, Run: checkScopes},
	{Name: "audiences", Phase: Both, Run: checkAudiences},
	{Name: "uses", Phase: Both, Run: checkUses},
	{Name: "anchors", Phase: Both, Run: checkAnchors},
	{Name: "links", Phase: Both, Run: checkLinks},
	{Name: "versionbump", Phase: CheckOnly, Run: checkVersionBump},
}

// Run runs every check applicable to "docsweb check": everything tagged
// Both or CheckOnly.
func Run(opts Options) (*Result, error) {
	return run(opts, CheckOnly)
}

// RunForBuild runs every check applicable to "docsweb build": everything
// tagged Both or BuildOnly. Exported for internal/build, which layers
// rendering and site generation on top of this same validated Result.
func RunForBuild(opts Options) (*Result, error) {
	return run(opts, BuildOnly)
}

func run(opts Options, phase Phase) (*Result, error) {
	ctx := &context{opts: opts}

	for _, c := range checks {
		if c.Phase != Both && c.Phase != phase {
			continue
		}
		if err := c.Run(ctx); err != nil {
			return nil, fmt.Errorf("check %s: %w", c.Name, err)
		}
	}

	return &Result{
		Config:       ctx.cfg,
		RootDir:      ctx.rootDir,
		ScopeRoots:   ctx.scopeRoots,
		RemoteScopes: ctx.remoteScopes,
		Registry:     ctx.registry,
		Anchors:      ctx.anchors,
		UsedBy:       ComputeUsedBy(ctx.registry),
		Issues:       ctx.issues,
	}, nil
}
