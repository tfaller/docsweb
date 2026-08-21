package build

// @docsweb
// @define build v0.3.0
// @name Build
// @summary
// Orchestrates a full docsweb build: collect every scope's targets,
// validate and classify @uses references, resolve @anchor:/@link:
// destinations, and render every target's Markdown to HTML.
// @uses collect@v0.2.0
// @uses config@v0.1.0
// @uses ignore@v0.1.0
// @uses mdlink@v0.1.0
// @uses model@v0.2.0
// @uses vcs@v0.1.0
// @audience dev
// @changelog
// Every rendered target now carries an `Author` - who last bumped its
// version, per git blame against HEAD's committed content, via the new
// [vcs](@link:vcs@v0.1.0) package. Best-effort: "" if the build isn't
// running inside a git repository, or the version-bumping line can't be
// placed in the committed blob. Non-breaking addition.
// @doc
// # Build
//
// `Run` is the whole pipeline in one call:
//
// 1. Load `.docsweb.yaml` and compile its `ignore:` list via
//    [ignore](@link:ignore@v0.1.0).
// 2. Walk the root scope plus every declared local scope with
//    [collect](@link:collect@v0.1.0), excluding each other declared
//    scope's own subtree from the root walk so nothing is scanned twice.
//    A remote (`git:`) scope is a hard error - out of scope for this POC.
// 3. Remap every non-root-scope target's `@audience` names through
//    [config](@link:config@v0.1.0)'s
//    [sub-scope audience mapping](@link:config@v0.1.0#audiencemap).
// 4. `ResolveUses` validates that every `@uses` lands on an existing
//    target, and classifies each one by
//    [DiffKind](@link:model@v0.1.0#diffkind) into an
//    [outdated use](@anchor:outdated): `DiffMajor` is reported as
//    breaking, `DiffMinor` as informational, `DiffPatch`/`DiffNone` are
//    dropped entirely.
// 5. Collect every target's declared anchors up front (anchor names must
//    be unique across a target's `@summary`+`@doc`+`@changelog` pieces
//    combined), so a `@link ...#anchor` can be resolved regardless of
//    which target - referencing or referenced - was scanned first.
// 6. Render every target's Markdown pieces to HTML via
//    [mdlink](@link:mdlink@v0.1.0#resolver), backed by a `Resolver` over
//    the collected registry and its anchor sets.
// 7. `ComputeUsedBy` inverts every target's `@uses` list into a "Used by"
//    index keyed by the referenced target - `@uses` already implies its
//    own reverse edge, so no separate annotation is needed to declare a
//    dependant.
//
// `TargetURL` is the single canonical URL scheme every downstream
// consumer (currently just the static site generator) uses to turn a
// `TargetRef` into a page path: dot-joined scope segments become path
// segments, so scope `"a.b"`, name `"c"` becomes `a/b/c.html`, and the
// root scope's `"c"` becomes just `c.html`. There is exactly one page per
// target - its current version - since the POC has no version history to
// page through.
// @docsweb

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/config"
	"github.com/tfaller/docsweb/internal/ignore"
	"github.com/tfaller/docsweb/internal/mdlink"
	"github.com/tfaller/docsweb/internal/model"
	"github.com/tfaller/docsweb/internal/vcs"
)

// Options configures a full docsweb build.
type Options struct {
	// ConfigPath is the path to the root .docsweb.yaml. Its directory is
	// the root scope's file tree; scopes it declares are read relative to
	// that directory, per README.md's "Scopes" section.
	ConfigPath string
	// RootScope names the root scope itself (the scope files directly
	// under the config's directory belong to, outside any declared
	// sub-scope). "" is a valid, and the default, root scope name.
	RootScope string
}

// ChangelogHTML is one target's @changelog entry, rendered.
type ChangelogHTML struct {
	Audiences []model.Audience
	HTML      string
}

// RenderedTarget pairs a parsed target with its Markdown pieces rendered to
// HTML (with @anchor:/@link: already resolved).
type RenderedTarget struct {
	Target        *model.Target
	SummaryHTML   string
	DocHTML       string
	ChangelogHTML []ChangelogHTML
	// UsedBy lists every other target whose @uses references this one -
	// the reverse of Target.Uses. See ComputeUsedBy.
	UsedBy []UsedByRef
	// Author is who last bumped this target's version, per git blame
	// against HEAD's committed content (see internal/vcs): "Name <email>",
	// or "" if unknown - the build isn't running inside a git repository,
	// the defining file isn't tracked yet, or its @define line couldn't be
	// matched in the committed blob. Best-effort informational metadata,
	// never a hard build requirement.
	Author string
}

// Result is everything internal/site needs to render the static output.
type Result struct {
	Targets []RenderedTarget
	Issues  []UsageIssue
}

// TargetURL returns the site-relative URL (from the output root) of ref's
// page: dot-joined scope segments become path segments, e.g. scope "a.b",
// name "c" -> "a/b/c.html"; the root scope ("") -> "c.html". There is
// exactly one page per target (its current version), since the POC has no
// version history to page through.
func TargetURL(ref model.TargetRef) string {
	if ref.Scope == "" {
		return ref.Name + ".html"
	}
	return strings.ReplaceAll(ref.Scope, ".", "/") + "/" + ref.Name + ".html"
}

// Run executes a full build: load config, collect every scope's targets,
// validate & classify @uses references, resolve @anchor:/@link:
// destinations, and render every target's Markdown to HTML.
func Run(opts Options) (*Result, error) {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	rootDir, err := filepath.Abs(filepath.Dir(opts.ConfigPath))
	if err != nil {
		return nil, err
	}

	reg := collect.NewRegistry()
	matcher := ignore.Compile(cfg.Ignore)

	// scopeRoots maps every scope name (including the root scope) to its
	// absolute directory, reused below to locate each target's defining
	// file for git-blame attribution.
	scopeRoots := map[string]string{opts.RootScope: rootDir}
	excludes := make([]string, 0, len(cfg.Scopes))
	for name, sc := range cfg.Scopes {
		if sc.Remote() {
			return nil, fmt.Errorf("scope %q: remote scopes are not supported by the POC (see README.md \"After POC\")", name)
		}
		scopeRoot := filepath.Join(rootDir, sc.Path)
		scopeRoots[name] = scopeRoot
		excludes = append(excludes, scopeRoot)
	}
	if err := reg.AddScope(collect.Options{Scope: opts.RootScope, Root: rootDir, Exclude: excludes, Ignore: matcher, IgnoreBase: rootDir}); err != nil {
		return nil, err
	}
	for name := range cfg.Scopes {
		if err := reg.AddScope(collect.Options{Scope: name, Root: scopeRoots[name], Ignore: matcher, IgnoreBase: rootDir}); err != nil {
			return nil, err
		}
	}

	if err := remapScopeAudiences(cfg, reg, opts.RootScope); err != nil {
		return nil, err
	}

	issues, err := ResolveUses(reg)
	if err != nil {
		return nil, err
	}

	anchors, err := collectAllAnchors(reg)
	if err != nil {
		return nil, err
	}
	resolver := &registryResolver{reg: reg, anchors: anchors}
	usedBy := ComputeUsedBy(reg)

	// git-blame attribution is best-effort: a build outside of a git
	// checkout (or one that hits some other VCS error) simply produces
	// targets with no Author, rather than failing.
	repo, _ := vcs.Open(rootDir)

	rendered := make([]RenderedTarget, 0, len(reg.Targets()))
	for _, t := range reg.Targets() {
		rt := RenderedTarget{Target: t, UsedBy: usedBy[t.Key()]}
		if repo != nil {
			rt.Author = blameAuthor(repo, scopeRoots[t.Scope], t)
		}

		rt.SummaryHTML, err = mdlink.RenderDoc(t.Summary, t.Scope, resolver)
		if err != nil {
			return nil, fmt.Errorf("%s: @summary: %w", t.Key(), err)
		}
		rt.DocHTML, err = mdlink.RenderDoc(t.Doc, t.Scope, resolver)
		if err != nil {
			return nil, fmt.Errorf("%s: @doc: %w", t.Key(), err)
		}
		for _, c := range t.Changelog {
			html, err := mdlink.RenderDoc(c.Body, t.Scope, resolver)
			if err != nil {
				return nil, fmt.Errorf("%s: @changelog: %w", t.Key(), err)
			}
			rt.ChangelogHTML = append(rt.ChangelogHTML, ChangelogHTML{Audiences: c.Audiences, HTML: html})
		}

		rendered = append(rendered, rt)
	}

	return &Result{Targets: rendered, Issues: issues}, nil
}

// blameAuthor looks up who last touched t's @define line - the line naming
// its current version - via git blame against HEAD. The line is found by
// its content ("@define <name> <version>", exactly what @define's grammar
// requires), built from t.Name/t.Version already in memory, rather than by
// re-reading the defining file just to reproduce its exact text. Returns ""
// (never an error) if that isn't knowable: no SourceFiles/DefineLine
// recorded, or repo.BlameAuthor itself can't place the line (untracked
// file, no matching line in the committed blob).
func blameAuthor(repo *vcs.Repository, scopeRoot string, t *model.Target) string {
	if t.DefineLine <= 0 || len(t.SourceFiles) == 0 {
		return ""
	}
	absPath := filepath.Join(scopeRoot, t.SourceFiles[0])
	contains := "@define " + t.Name + " " + t.Version.String()
	author, ok, err := repo.BlameAuthor(absPath, t.DefineLine, contains)
	if err != nil || !ok {
		return ""
	}
	return author.String()
}

// collectAllAnchors gathers every target's declared anchors up front, across
// all of its Markdown pieces, so @link:...#anchor fragments can be validated
// and resolved regardless of definition order across targets/files.
func collectAllAnchors(reg *collect.Registry) (map[string]map[string]bool, error) {
	out := make(map[string]map[string]bool)
	for _, t := range reg.Targets() {
		set := map[string]bool{}
		pieces := append([]string{t.Summary, t.Doc}, changelogBodies(t)...)
		for _, p := range pieces {
			names, err := mdlink.CollectAnchors(p)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", t.Key(), err)
			}
			for _, n := range names {
				if set[n] {
					return nil, fmt.Errorf("%s: duplicate anchor %q", t.Key(), n)
				}
				set[n] = true
			}
		}
		out[t.Key()] = set
	}
	return out, nil
}

func changelogBodies(t *model.Target) []string {
	out := make([]string, len(t.Changelog))
	for i, c := range t.Changelog {
		out[i] = c.Body
	}
	return out
}

// registryResolver implements mdlink.Resolver against a collected target
// registry and its precomputed anchor sets.
type registryResolver struct {
	reg     *collect.Registry
	anchors map[string]map[string]bool
}

func (r *registryResolver) ResolveTarget(ref model.TargetRef) (string, bool) {
	if _, ok := r.reg.Get(ref.Key()); !ok {
		return "", false
	}
	return TargetURL(ref), true
}

func (r *registryResolver) HasAnchor(ref model.TargetRef, anchor string) bool {
	return r.anchors[ref.Key()][anchor]
}
