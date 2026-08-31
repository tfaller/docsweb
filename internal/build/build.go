package build

// @docsweb
// @define build v0.10.0
// @name Build
// @summary
// Orchestrates a full docsweb build: run every check, discover every
// target's past versions, then render every version's Markdown to HTML and
// attribute it to a git blame author.
// @uses check@v0.5.0
// @uses history@v0.1.0
// @uses mdlink@v0.2.0
// @uses model@v0.3.0
// @uses vcs@v0.5.0
// @audience dev
// @changelog
// **Past target versions are now discovered and rendered.** After
// [check.RunForBuild](@link:check@v0.1.0), `Run` walks the root scope's own
// git history via [history.Walk](@link:history@v0.1.0) and builds, for
// every target, a `Versions` list (current plus every past version found)
// and a `History` list of fully rendered past-version snapshots - one
// additional page per entry, at `HistoricTargetURL`. Best-effort, like
// blame attribution: outside of a git repository, or for a target in a
// remote scope (out of scope for `history.Walk`, see its own doc),
// `Versions` simply holds only the current entry and `History` is empty -
// never a build failure. A historic snapshot renders leniently (see
// `mdlink.RenderDocLenient`): a broken `@link`/`@uses`/anchor in old content
// degrades to plain text rather than failing the build, since a past commit
// can't be fixed after the fact.
//
// **`@uses`/`@link` now resolve to the exact version they name, when one
// was discovered, instead of always the target's current version.** Both
// the structured `Uses` list (new `RenderedTarget.Uses`/`HistoricVersion.
// Uses`, resolved by `resolveUses`) and prose `@link:`s (via the new
// `versionResolver`, replacing `registryResolver`) look a reference's exact
// version up across every known version of its target - current and
// historic - before falling back to the current version, the same
// resolution `registryResolver` always did. This matters most for a
// historic page: its own `@uses`/`@link`s were written against whatever
// versions existed *at that point in history*, and now correctly link to
// those specific past pages rather than always jumping to what's current
// today.
//
// Breaking for direct callers of `RenderedTarget`/`Result` construction
// (e.g. `internal/site`'s tests): `RenderedTarget` gained `Uses`, `Versions`,
// and `History`; a hand-built one with no history simply leaves them empty,
// same as a real build outside of a git repository would.
//
// `@uses check@v0.4.0` also bumped to `check`'s current `v0.5.0` (itself
// just a refreshed `mdlink`/`vcs` reference, no behavior change there).
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
// 2. Git-blame attribution (best-effort, see `blameAuthorAt`) looks up who
//    last touched each target's `@define` line, against that target's own
//    scope's repository - a remote scope's own resolved commit, when it is
//    one (see `Result.RemoteScopes`).
// 3. [history.Walk](@link:history@v0.1.0) discovers every past version of
//    every target defined in the root scope's own git history (best-effort;
//    see the changelog above), building a `versionsByKey` index - every
//    known version of every target, current first - shared by every
//    resolver and every page rendered below.
// 4. Render every version's Markdown pieces to HTML via
//    [mdlink](@link:mdlink@v0.2.0#resolver) (strictly for the current
//    version, leniently for a historic one via `renderHistoricVersion`),
//    backed by a `versionResolver` over `versionsByKey`.
//
// `TargetURL` is the current-version URL scheme every downstream consumer
// (currently just the static site generator) uses to turn a `TargetRef`
// into a page path: dot-joined scope segments become path segments, so
// scope `"a.b"`, name `"c"` becomes `a/b/c.html`, and the root scope's `"c"`
// becomes just `c.html`. A non-current version discovered via
// `history.Walk` instead gets a page at `HistoricTargetURL`, nested under
// its target's own directory (e.g. `a/b/c/v1.0.0.html`).
// @docsweb

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tfaller/docsweb/internal/check"
	"github.com/tfaller/docsweb/internal/history"
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
	// Uses resolves every one of Target.Uses to a specific page: the exact
	// version referenced, if internal/history discovered it, else the
	// target's current version - see versionResolver.lookup.
	Uses []UseLink
	// Versions lists every version of this target known to the build - its
	// current one plus every one discovered by internal/history - for a
	// version-switcher link list. Present (and identical) on this target's
	// current page and every one of its History pages. Only ever a single
	// entry (the current version) when no VCS history could be discovered.
	Versions []VersionLink
	// History holds every non-current version's own rendered snapshot -
	// internal/site writes one additional page per entry, at its
	// VersionLink's URL. Rendered leniently (see mdlink.RenderDocLenient):
	// a broken @link/@uses in old content degrades to plain text rather
	// than failing the build, since a past commit can't be fixed after the
	// fact.
	History []HistoricVersion
}

// VersionLink is one entry in a target's version-switcher list.
type VersionLink struct {
	Version model.Version
	URL     string
	Current bool
}

// UseLink resolves one Target.Uses reference to the page it should link to.
type UseLink struct {
	// Label is "scope.name@vX.Y.Z", exactly as the @uses reference reads.
	Label string
	// URL is relative to the page this reference was resolved for, or ""
	// if Found is false.
	URL   string
	Found bool
}

// HistoricVersion is one past version of a target, reconstructed by
// internal/history from the commit that introduced it.
type HistoricVersion struct {
	Target        *model.Target
	SummaryHTML   string
	DocHTML       string
	ChangelogHTML []ChangelogHTML
	// Author is who introduced this version, per git blame against the
	// commit internal/history found it at - see RenderedTarget.Author for
	// what "" means.
	Author string
	// Uses is this version's own Target.Uses, resolved the same way as
	// RenderedTarget.Uses.
	Uses []UseLink
}

// Result is everything internal/site needs to render the static output.
type Result struct {
	Targets []RenderedTarget
	Issues  []check.UsageIssue
}

// TargetURL returns the site-relative URL (from the output root) of ref's
// current-version page: dot-joined scope segments become path segments,
// e.g. scope "a.b", name "c" -> "a/b/c.html"; the root scope ("") ->
// "c.html". See HistoricTargetURL for a non-current version's page.
func TargetURL(ref model.TargetRef) string {
	if ref.Scope == "" {
		return ref.Name + ".html"
	}
	return strings.ReplaceAll(ref.Scope, ".", "/") + "/" + ref.Name + ".html"
}

// HistoricTargetURL returns the site-relative URL of a past version of
// ref's target, nested under its current page's own directory, e.g. current
// page "a/b/c.html" gets past-version pages at "a/b/c/vX.Y.Z.html".
func HistoricTargetURL(ref model.TargetRef) string {
	return strings.TrimSuffix(TargetURL(model.TargetRef{Scope: ref.Scope, Name: ref.Name}), ".html") +
		"/" + ref.Version.String() + ".html"
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
// each target's current version to a git blame author, discover any past
// versions via internal/history, and render every version's Markdown to
// HTML.
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

	// versionsByKey seeds every target's version list with its current
	// version (always index 0), then appends every non-current version
	// internal/history discovers. Discovery is best-effort, same spirit as
	// blame attribution: outside of a git repository (or any other VCS
	// error), every target simply has no history, never a build failure.
	// Only the root scope's own repository is walked - a remote scope has
	// its own separate history and is out of scope for internal/history
	// (see its package doc), so its targets only ever get a single-entry
	// (current-only) version list.
	versionsByKey := map[string][]versionEntry{}
	for _, t := range chk.Registry.Targets() {
		versionsByKey[t.Key()] = []versionEntry{{target: t, anchors: chk.Anchors[t.Key()], url: TargetURL(t.Ref())}}
	}

	var rootRepo *vcs.Repository
	if rootDir, ok := chk.ScopeRoots[chk.Config.Name]; ok {
		if repo := localRepo(rootDir); repo != nil {
			rootRepo = repo
			rootConfigPath := filepath.Join(chk.RootDir, filepath.Base(opts.ConfigPath))
			if found, err := history.Walk(repo, rootConfigPath, chk.Config); err == nil {
				addHistoricVersions(versionsByKey, found)
			}
		}
	}

	rendered := make([]RenderedTarget, 0, len(chk.Registry.Targets()))
	for _, t := range chk.Registry.Targets() {
		versions := versionsByKey[t.Key()]
		targetURL := TargetURL(t.Ref())
		rt := RenderedTarget{
			Target:   t,
			UsedBy:   chk.UsedBy[t.Key()],
			Uses:     resolveUses(t.Uses, versionsByKey, targetURL),
			Versions: versionLinks(versions),
		}

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
			rt.Author = blameAuthorAt(repo, repo.PinnedCommit(), sourcePath, t)
		}

		// Resolved @link/@uses URLs must be relative to this target's own
		// page, so a fresh resolver is built per target (and per historic
		// version, below) with its own page URL as the relative-link
		// origin. Every resolver shares the same versionsByKey, so a
		// reference to an exact past version (e.g. an outdated @uses, or an
		// old @link) lands on that version's own page when one was
		// discovered, not always the current one - see versionResolver.
		resolver := &versionResolver{versionsByKey: versionsByKey, fromURL: targetURL}

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

		for _, v := range versions[1:] {
			rt.History = append(rt.History, renderHistoricVersion(rootRepo, v, versionsByKey))
		}

		rendered = append(rendered, rt)
	}

	return &Result{Targets: rendered, Issues: chk.Issues}, nil
}

// versionEntry is one version of a target - current or historic - carrying
// everything a versionResolver needs to answer @link/@uses lookups, plus
// (for a historic entry) what renderHistoricVersion needs to attribute and
// render it.
type versionEntry struct {
	target  *model.Target
	anchors map[string]bool
	url     string
	// commit and path are set only for a historic entry (nil/"" for the
	// current one, whose Author is instead attributed against the live
	// repository's pinned commit in Run): the commit internal/history found
	// this version at, and its defining file's path as of that commit
	// (repository-tree-relative, slash-separated - see history.Version.Path).
	commit *vcs.Commit
	path   string
}

// addHistoricVersions appends every version history.Walk found (other than
// a target's current one, already versionsByKey's index 0) to its target's
// entry, newest version first.
func addHistoricVersions(versionsByKey map[string][]versionEntry, found map[string][]history.Version) {
	for key, hvs := range found {
		cur, ok := versionsByKey[key]
		if !ok {
			continue
		}
		currentVersion := cur[0].target.Version

		var historic []versionEntry
		for _, hv := range hvs {
			if hv.Target.Version.Equal(currentVersion) {
				continue
			}
			historic = append(historic, versionEntry{
				target:  hv.Target,
				anchors: collectAnchors(hv.Target),
				url:     HistoricTargetURL(hv.Target.Ref()),
				commit:  hv.Commit,
				path:    hv.Path,
			})
		}
		sort.Slice(historic, func(i, j int) bool {
			return historic[i].target.Version.Compare(historic[j].target.Version) > 0
		})
		versionsByKey[key] = append(cur, historic...)
	}
}

// collectAnchors gathers every @anchor: t declares across its Markdown
// pieces, the same way check.checkAnchors does for the current registry -
// but tolerantly, since a historic target's content was never validated the
// way the current one was: a piece with an invalid/duplicate anchor name
// simply contributes no anchors from that piece, rather than failing.
func collectAnchors(t *model.Target) map[string]bool {
	set := map[string]bool{}
	for _, p := range t.Pieces() {
		names, err := mdlink.CollectAnchors(p)
		if err != nil {
			continue
		}
		for _, n := range names {
			set[n] = true
		}
	}
	return set
}

// versionLinks renders entries (current first, then historic newest-first,
// per addHistoricVersions) into the version-switcher list every page of
// this target shows.
func versionLinks(entries []versionEntry) []VersionLink {
	links := make([]VersionLink, len(entries))
	for i, e := range entries {
		links[i] = VersionLink{Version: e.target.Version, URL: e.url, Current: i == 0}
	}
	return links
}

// renderHistoricVersion attributes and renders one past version's Markdown,
// leniently (see mdlink.RenderDocLenient): a broken @link/@uses/anchor in
// old content degrades to plain text instead of failing the build, since a
// past commit can't be fixed after the fact. rootRepo is nil when no git
// repository backs the root scope, in which case v.commit is also always
// nil (history.Walk never runs) and Author is simply left empty.
func renderHistoricVersion(rootRepo *vcs.Repository, v versionEntry, versionsByKey map[string][]versionEntry) HistoricVersion {
	hv := HistoricVersion{Target: v.target, Uses: resolveUses(v.target.Uses, versionsByKey, v.url)}
	if rootRepo != nil && v.commit != nil {
		absPath := filepath.Join(rootRepo.Root(), filepath.FromSlash(v.path))
		hv.Author = blameAuthorAt(rootRepo, v.commit, absPath, v.target)
	}

	resolver := &versionResolver{versionsByKey: versionsByKey, fromURL: v.url}
	hv.SummaryHTML, _ = mdlink.RenderDocLenient(v.target.Summary, v.target.Scope, resolver)
	hv.DocHTML, _ = mdlink.RenderDocLenient(v.target.Doc, v.target.Scope, resolver)
	for _, c := range v.target.Changelog {
		html, _ := mdlink.RenderDocLenient(c.Body, v.target.Scope, resolver)
		hv.ChangelogHTML = append(hv.ChangelogHTML, ChangelogHTML{Audiences: c.Audiences, HTML: html})
	}
	return hv
}

// blameAuthorAt looks up who last touched t's @define line - the line
// naming its version - via git blame against commit. sourcePath locates t's
// defining file within repo - see vcs.Repository.BlameAuthorAt's doc for
// what it means for a local vs. a remote scope's repo. The line is found by
// its content ("@define <name> <version>", exactly what @define's grammar
// requires), built from t.Name/t.Version already in memory, rather than by
// re-reading the defining file just to reproduce its exact text. Returns ""
// (never an error) if that isn't knowable: no DefineLine recorded, no
// commit to blame against, or repo.BlameAuthorAt itself can't place the line
// (untracked file, no matching line in the committed blob).
func blameAuthorAt(repo *vcs.Repository, commit *vcs.Commit, sourcePath string, t *model.Target) string {
	if t.DefineLine <= 0 || commit == nil {
		return ""
	}
	contains := "@define " + t.Name + " " + t.Version.String()
	author, ok, err := repo.BlameAuthorAt(commit, sourcePath, t.DefineLine, contains)
	if err != nil || !ok {
		return ""
	}
	return author.String()
}

// versionResolver implements mdlink.Resolver against every known version of
// every target (current and historic, see versionsByKey), so a reference to
// a specific version (e.g. `@uses foo@v1.0.0`, or `@link:foo@v1.0.0#x`)
// resolves to that exact version's own page - and its own anchor set - when
// internal/history discovered it, falling back to the target's current
// version (versionsByKey's index 0) when it didn't. It resolves links
// relative to fromURL - the page of the version currently being rendered -
// via RelLink, since the resolved HTML is later written verbatim to that
// page.
type versionResolver struct {
	versionsByKey map[string][]versionEntry
	fromURL       string
}

func (r *versionResolver) ResolveTarget(ref model.TargetRef) (string, bool) {
	v, ok := lookupVersion(r.versionsByKey, ref)
	if !ok {
		return "", false
	}
	return RelLink(r.fromURL, v.url), true
}

func (r *versionResolver) HasAnchor(ref model.TargetRef, anchor string) bool {
	v, ok := lookupVersion(r.versionsByKey, ref)
	if !ok {
		return false
	}
	return v.anchors[anchor]
}

// lookupVersion returns the versionEntry ref should resolve to: an exact
// version match if one is known, otherwise the target's current version
// (index 0), so a reference to a version internal/history never captured
// still resolves somewhere useful rather than becoming a hard failure -
// important for rendering historic content (see mdlink.RenderDocLenient),
// whose own references were never validated against today's target set the
// way the current registry's are.
func lookupVersion(versionsByKey map[string][]versionEntry, ref model.TargetRef) (versionEntry, bool) {
	versions, ok := versionsByKey[ref.Key()]
	if !ok || len(versions) == 0 {
		return versionEntry{}, false
	}
	for _, v := range versions {
		if v.target.Version.Equal(ref.Version) {
			return v, true
		}
	}
	return versions[0], true
}

// resolveUses resolves every one of uses to the page it should link to (see
// lookupVersion), relative to fromURL - the page whose structured "Uses"
// list this is for.
func resolveUses(uses []model.TargetRef, versionsByKey map[string][]versionEntry, fromURL string) []UseLink {
	if len(uses) == 0 {
		return nil
	}
	links := make([]UseLink, 0, len(uses))
	for _, use := range uses {
		label := fmt.Sprintf("%s@%s", use.Key(), use.Version)
		v, ok := lookupVersion(versionsByKey, use)
		if !ok {
			links = append(links, UseLink{Label: label})
			continue
		}
		links = append(links, UseLink{Label: label, URL: RelLink(fromURL, v.url), Found: true})
	}
	return links
}
