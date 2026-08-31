package build

// @docsweb
// @define build v0.9.0
// @name Build
// @summary
// Orchestrates a full docsweb build: run every check, then render every
// target's Markdown to HTML and attribute its current version to a git
// blame author.
// @uses check@v0.4.0
// @uses mdlink@v0.1.0
// @uses model@v0.3.0
// @uses vcs@v0.4.0
// @audience dev
// @changelog
// Git-blame attribution now sources its `vcs.Repository` per target
// differently depending on whether that target's own scope is local or
// remote: a local scope still opens (lazily, cached) a `vcs.Repository` via
// `vcs.Open` against that scope's on-disk root, exactly as before; a remote
// (`git:`) scope's `vcs.Repository` now comes pre-resolved from
// [check](@link:check@v0.4.0)'s new `Result.RemoteScopes`, pinned to the
// commit its file tree was read from - since [vcs.OpenScope](@link:vcs@v0.4.0)
// no longer checks a worktree out to disk at all, there's nothing left for
// `vcs.Open` to discover there. Attribution itself is unaffected; a remote
// scope's targets are still correctly blamed from that scope's own
// repository history, not the root's. Non-breaking for callers of `Run`.
// @doc
// # Build
//
// `Run` is the whole pipeline in one call:
//
// 1. [check.RunForBuild](@link:check@v0.1.0) does everything needed to
//    confirm every target is in shape to render correctly: load
//    `.docsweb.yaml`, open/fetch and walk the root scope plus every
//    declared referenced scope (local or remote), validate `@audience`
//    names, validate `@uses` references (classifying each by
//    [DiffKind](@link:model@v0.1.0#diffkind) into an [outdated use](@anchor:outdated)),
//    collect every target's anchors, and validate every `@link`
//    reference resolves - all without rendering a single piece of
//    Markdown to HTML.
// 2. Git-blame attribution (best-effort, see `blameAuthor`) looks up who
//    last touched each target's `@define` line, against that target's own
//    scope's repository - a remote scope's own resolved commit, when it is
//    one (see `Result.RemoteScopes`).
// 3. Render every target's Markdown pieces to HTML via
//    [mdlink](@link:mdlink@v0.1.0#resolver), backed by a `Resolver` over
//    the checked registry and its anchor sets.
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
	"path"
	"path/filepath"
	"strings"

	"github.com/tfaller/docsweb/internal/check"
	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/mdlink"
	"github.com/tfaller/docsweb/internal/model"
	"github.com/tfaller/docsweb/internal/vcs"
)

// Options configures a full docsweb build.
type Options struct {
	// ConfigPath is the path to the root .docsweb.yaml. Its directory is
	// the root scope's file tree; scopes it declares are read relative to
	// that directory, per README.md's "Scopes" section. The root scope's
	// own name comes from that config's own self-declared `name:` field
	// (Config.Name), not from an option here.
	ConfigPath string
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
	// the reverse of Target.Uses. See check.ComputeUsedBy.
	UsedBy []check.UsedByRef
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
	Issues  []check.UsageIssue
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

// RelLink computes a relative link from the page at fromURL to the page at
// toURL, where both are root-relative URLs in TargetURL's scheme
// (slash-separated, no "." or ".." segments). The result is simply "../"
// repeated once per directory level fromURL is nested under, followed by
// toURL.
func RelLink(fromURL, toURL string) string {
	depth := strings.Count(fromURL, "/")
	return strings.Repeat("../", depth) + toURL
}

// Run executes a full build: run every check (see internal/check), attribute
// each target's current version to a git blame author, and render every
// target's Markdown to HTML.
func Run(opts Options) (*Result, error) {
	chk, err := check.RunForBuild(check.Options{ConfigPath: opts.ConfigPath})
	if err != nil {
		return nil, err
	}

	// git-blame attribution is best-effort: a scope root outside of a git
	// checkout (or one that hits some other VCS error) simply produces
	// targets with no Author, rather than failing. A local scope root is
	// opened lazily and cached, since its defining files may live in a
	// different git repository (a different local scope's own checkout)
	// than the root's; a remote scope's Repository comes pre-resolved from
	// chk.RemoteScopes - it has no on-disk root for vcs.Open to discover in
	// the first place, since [vcs.OpenScope](@link:vcs@v0.4.0) never checks
	// a worktree out to disk.
	repos := map[string]*vcs.Repository{}
	localRepo := func(dir string) *vcs.Repository {
		if r, cached := repos[dir]; cached {
			return r
		}
		r, _ := vcs.Open(dir)
		repos[dir] = r
		return r
	}

	rendered := make([]RenderedTarget, 0, len(chk.Registry.Targets()))
	for _, t := range chk.Registry.Targets() {
		rt := RenderedTarget{Target: t, UsedBy: chk.UsedBy[t.Key()]}

		var repo *vcs.Repository
		var sourcePath string
		if len(t.SourceFiles) > 0 {
			if rs, ok := chk.RemoteScopes[t.ConfigScope]; ok {
				repo = rs.Repo
				sourcePath = path.Join(rs.Path, t.SourceFiles[0])
			} else if scopeRoot, ok := chk.ScopeRoots[t.ConfigScope]; ok {
				repo = localRepo(scopeRoot)
				sourcePath = filepath.Join(scopeRoot, t.SourceFiles[0])
			}
		}
		if repo != nil {
			rt.Author = blameAuthor(repo, sourcePath, t)
		}

		// Resolved @link URLs must be relative to this target's own page, so
		// a fresh resolver is built per target with its page URL as the
		// relative-link origin (see registryResolver.ResolveTarget).
		resolver := &registryResolver{reg: chk.Registry, anchors: chk.Anchors, fromURL: TargetURL(t.Ref())}

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

	return &Result{Targets: rendered, Issues: chk.Issues}, nil
}

// blameAuthor looks up who last touched t's @define line - the line naming
// its current version - via git blame against repo's pinned commit (HEAD,
// for a local scope's repo opened via vcs.Open). sourcePath locates t's
// defining file within repo - see vcs.Repository.BlameAuthor's doc for what
// it means for a local vs. a remote scope's repo. The line is found by its
// content ("@define <name> <version>", exactly what @define's grammar
// requires), built from t.Name/t.Version already in memory, rather than by
// re-reading the defining file just to reproduce its exact text. Returns ""
// (never an error) if that isn't knowable: no DefineLine recorded, or
// repo.BlameAuthor itself can't place the line (untracked file, no matching
// line in the committed blob).
func blameAuthor(repo *vcs.Repository, sourcePath string, t *model.Target) string {
	if t.DefineLine <= 0 {
		return ""
	}
	contains := "@define " + t.Name + " " + t.Version.String()
	author, ok, err := repo.BlameAuthor(sourcePath, t.DefineLine, contains)
	if err != nil || !ok {
		return ""
	}
	return author.String()
}

// registryResolver implements mdlink.Resolver against a collected target
// registry and its precomputed anchor sets. It resolves @link references
// into links relative to fromURL - the page of the target whose Markdown is
// currently being rendered - via RelLink, since the resolved HTML is later
// written verbatim to that page.
type registryResolver struct {
	reg     *collect.Registry
	anchors map[string]map[string]bool
	fromURL string
}

func (r *registryResolver) ResolveTarget(ref model.TargetRef) (string, bool) {
	if _, ok := r.reg.Get(ref.Key()); !ok {
		return "", false
	}
	return RelLink(r.fromURL, TargetURL(ref)), true
}

func (r *registryResolver) HasAnchor(ref model.TargetRef, anchor string) bool {
	return r.anchors[ref.Key()][anchor]
}
