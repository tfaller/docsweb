// Package history walks a local git repository's first-parent commit log to
// discover past versions of the targets a docsweb build collects from its
// working tree today - so a static site can link back to what a target's
// documentation looked like at an older version, not just its current one.
package history

// @docsweb
// @define history v0.2.0
// @name History
// @summary
// Walks a repository's first-parent commit log backward from its pinned
// commit, reconstructing every past version of every target defined in the
// root scope or a local (path-based) referenced scope.
// @uses collect@v0.5.0
// @uses config@v0.3.0
// @uses ignore@v0.1.0
// @uses model@v0.3.0
// @uses vcs@v0.6.0
// @audience dev
// @changelog
// No behavior change to `history` itself - `@uses` reference bumped to
// [vcs](@link:vcs@v0.6.0)'s current version following its
// `Repository.FileContents`/`BlameAuthor`/`BlameAuthorAt` now always taking
// a repository-tree-relative path (the OS-absolute-path branch is gone).
// @doc
// # History
//
// `Walk` is a "smart" walk: most commits touch neither a `@define` line nor
// a `.docsweb.yaml`, so the bulk of history costs only a tree diff, never a
// full re-collection of a scope. Two signals make a commit worth acting on,
// found via [vcs.DiffStep](@link:vcs@v0.5.0)'s per-file added-line/removed
// detection:
//
//   - A file gained a line containing `@define`. Only the added line
//     matters - a `@define` line removed in the same commit (the common
//     case, since a version bump edits one line) is never itself
//     informative: the version it named was already captured when the walk
//     later reaches the commit that first introduced it. The file (as
//     committed at that commit) is re-parsed via
//     [annotation](@link:annotation@v0.1.0) and converted via
//     [collect.ToTarget](@link:collect@v0.5.0), exactly like re-parsing a
//     single old revision for a diff - malformed historic content is
//     skipped rather than failing the walk, since a past commit can't be
//     fixed after the fact.
//   - The root `.docsweb.yaml` itself changed (added, modified, or
//     removed). Its parsed content is cached and reused for every commit
//     until this happens - see [config](@link:config@v0.3.0)'s "Scopes"/
//     "Ignore" semantics for what it governs: which directory belongs to
//     which local scope, and which files are ignored. A referenced local
//     scope's own `.docsweb.yaml` is not separately tracked - like a live
//     build, only its self-declared `name:` is ever consulted, never its
//     structure (see PLAN.md assumption 16), and that name is assumed
//     constant through history.
//
// Only the root scope's own directory and local (`path:`-based) referenced
// scopes are covered - a remote (`git:`) scope is a separate repository with
// its own separate history, pinned at one `ref` exactly as a live build
// already does; walking it in lockstep with the root's is a different,
// larger problem left for later. `Walk` returns immediately (no error) if
// repo has no on-disk root (i.e. it was opened via
// [vcs.OpenScope](@link:vcs@v0.5.0), not [vcs.Open](@link:vcs@v0.5.0)) - a
// remote scope's own repository is exactly this case.
//
// The walk stops (without error) once an ancestor commit's root config can
// no longer be safely used: the file didn't exist yet (the commit that
// first introduced docsweb to this repository), or it exists but
// [config.Parse](@link:config@v0.3.0) rejects it (e.g. an older commit
// predates a since-added requirement, like `name:` becoming mandatory) -
// both mean there's no way to know what scope structure/ignore rules
// governed that commit, so older history is left undiscovered rather than
// guessed at. Versions are returned in walk order (newest-introduced
// first), keyed by [model.Target.Key](@link:model@v0.3.0), including the
// entry for the target's current version - a caller that only wants
// genuinely *past* versions filters that one out itself by comparing
// against the live registry.
// @docsweb

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/tfaller/docsweb/internal/annotation"
	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/config"
	"github.com/tfaller/docsweb/internal/ignore"
	"github.com/tfaller/docsweb/internal/model"
	"github.com/tfaller/docsweb/internal/vcs"
)

// cacheDirName mirrors internal/check's own constant: a remote scope's bare
// mirror is never itself part of the root repository's real history, but is
// excluded defensively in case it was ever accidentally committed.
const cacheDirName = "docsweb-cache"

// Version is one version of a target discovered by Walk, reconstructed from
// the exact commit that introduced it.
type Version struct {
	// Target is the target as it existed at Commit - its own Summary/Doc/
	// Changelog/Uses/Audiences, not merely the version number.
	Target *model.Target
	// Commit is the commit whose diff first added Target's @define line.
	Commit *vcs.Commit
	// Path is Target's defining file's path as committed in Commit,
	// repository-tree-relative and slash-separated - kept separately from
	// Target.SourceFiles (which is scope-relative) since it is what
	// Repository.BlameAuthorAt needs to attribute this version to its
	// author.
	Path string
}

// Walk walks repo's first-parent commit history backward from its pinned
// commit, discovering every past version of every target defined in the
// root scope (whose config is at rootConfigRel, a repository-tree-relative,
// slash-separated path, already parsed as rootCfg - its current, live
// content) or a local referenced scope it declares. See the package doc for
// exactly what makes a commit worth acting on, and what is deliberately out
// of scope (remote scopes, nested scope discovery).
func Walk(repo *vcs.Repository, rootConfigRel string, rootCfg *config.Config) (map[string][]Version, error) {
	if repo.Root() == "" {
		return nil, nil
	}
	start := repo.PinnedCommit()
	if start == nil {
		return nil, nil
	}

	rootScopeRel := path.Dir(rootConfigRel)
	if rootScopeRel == "." {
		rootScopeRel = ""
	}

	cfg := rootCfg
	matcher := ignore.Compile(cfg.Ignore)
	scopes := localScopes(cfg, rootScopeRel)

	result := map[string][]Version{}
	seen := map[string]map[string]bool{}

	walkErr := vcs.WalkFirstParent(start, func(step vcs.Step) (bool, error) {
		diff, err := vcs.DiffStep(step)
		if err != nil {
			return false, fmt.Errorf("history: %w", err)
		}

		configChanged := false
		var candidates []string

		for _, f := range diff.Files() {
			if f.Path == rootConfigRel {
				configChanged = true
				continue
			}
			if f.Removed {
				continue
			}
			if diff.AddedLineContains(f.Path, "@define") {
				candidates = append(candidates, f.Path)
			}
		}

		for _, p := range candidates {
			owner, scopeRelFile, ok := resolveOwner(scopes, p)
			if !ok {
				continue
			}
			if isIgnored(matcher, rootScopeRel, p) {
				continue
			}

			content, ok, err := repo.FileContents(step.Commit, p)
			if err != nil {
				return false, fmt.Errorf("history: reading %s at %s: %w", p, step.Commit.Hash, err)
			}
			if !ok {
				continue
			}

			docs, err := annotation.ParseSource(content)
			if err != nil {
				continue
			}
			for _, doc := range docs {
				t, err := collect.ToTarget(owner, scopeRelFile, doc)
				if err != nil {
					continue
				}
				key := t.Key()
				v := t.Version.String()
				if seen[key][v] {
					continue
				}
				if seen[key] == nil {
					seen[key] = map[string]bool{}
				}
				seen[key][v] = true
				result[key] = append(result[key], Version{Target: t, Commit: step.Commit, Path: p})
			}
		}

		if !configChanged {
			return true, nil
		}
		if step.Parent == nil {
			return false, nil
		}
		content, ok, err := repo.FileContents(step.Parent, rootConfigRel)
		if err != nil {
			return false, fmt.Errorf("history: reading %s at %s: %w", rootConfigRel, step.Parent.Hash, err)
		}
		if !ok {
			return false, nil
		}
		newCfg, err := config.Parse([]byte(content))
		if err != nil {
			return false, nil
		}
		cfg = newCfg
		matcher = ignore.Compile(cfg.Ignore)
		scopes = localScopes(cfg, rootScopeRel)
		return true, nil
	})

	return result, walkErr
}

// scopeEntry is one local scope known to a config generation: its
// self-declared name and its directory, relative to the git repository root
// (slash-separated), sorted longest-directory-first so ownership resolution
// can do a simple linear longest-prefix match.
type scopeEntry struct {
	name   string
	dirRel string
}

func localScopes(cfg *config.Config, rootScopeRel string) []scopeEntry {
	scopes := []scopeEntry{{name: cfg.Name, dirRel: rootScopeRel}}
	for name, sc := range cfg.Scopes {
		if sc.Remote() {
			continue
		}
		scopes = append(scopes, scopeEntry{name: name, dirRel: path.Join(rootScopeRel, sc.Path)})
	}
	sort.Slice(scopes, func(i, j int) bool { return len(scopes[i].dirRel) > len(scopes[j].dirRel) })
	return scopes
}

// resolveOwner finds which scope in scopes (pre-sorted longest-directory-
// first by localScopes) owns repo-root-relative path p, returning that
// scope's name and p's path relative to the scope's own directory. p under
// the shared docsweb-cache mirror directory never resolves - see cacheDirName.
func resolveOwner(scopes []scopeEntry, p string) (name, scopeRelFile string, ok bool) {
	for _, s := range scopes {
		var rel string
		switch {
		case s.dirRel == "":
			rel = p
		case p == s.dirRel:
			rel = ""
		case strings.HasPrefix(p, s.dirRel+"/"):
			rel = strings.TrimPrefix(p, s.dirRel+"/")
		default:
			continue
		}
		// docsweb-cache always lives directly under the root scope's own
		// directory (see internal/check's cacheDirName) - excluded
		// defensively here too, in case it was ever accidentally committed.
		if rel == cacheDirName || strings.HasPrefix(rel, cacheDirName+"/") {
			return "", "", false
		}
		return s.name, rel, true
	}
	return "", "", false
}

// isIgnored reports whether p (repository-root-relative, slash-separated) is
// excluded by matcher - Ignore patterns are declared relative to the root
// config's own directory (rootScopeRel), regardless of which scope actually
// owns p, matching internal/check/internal/collect's live-build semantics.
func isIgnored(matcher *ignore.Matcher, rootScopeRel, p string) bool {
	if matcher == nil {
		return false
	}
	rel := p
	if rootScopeRel != "" {
		rel = strings.TrimPrefix(p, rootScopeRel+"/")
	}
	return matcher.Match(rel, false)
}
