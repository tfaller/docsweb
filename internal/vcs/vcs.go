// Package vcs looks up, via git blame, who last touched a specific source
// line. docsweb uses this to attribute a target's current version to the
// commit author who made the @define bump - see README.md's "The
// documentation system knows about Version Control" paragraph.
package vcs

// @docsweb
// @define vcs v0.5.0
// @name VCS
// @summary
// Git lookups: blame (who last touched a given source line, at any commit -
// not just the one a Repository is pinned to), opening a remote repository's
// resolved commit into a local cache with no worktree checkout, walking a
// repository's first-parent commit history, and, given a revision, a
// repository's merge base and a file's committed contents - used to
// attribute a version bump to its author, materialize a remote scope's file
// tree, discover past target versions, and diff documentation against an
// older revision.
// @audience dev
// @changelog
// New history-walking primitives, added for
// [history](@link:history@v0.1.0)'s use: `WalkFirstParent(start, fn)` calls
// fn once per commit reached by following first parents backward from
// start, `DiffStep(step)` computes the file-level changes between one such
// step's two commits (`Parent` is nil for the repository's root commit,
// treated as "every file added"), and `Diff.Files`/`Diff.AddedLineContains`
// answer exactly the two questions a caller needs to decide whether a
// commit is worth acting on - which files changed, and whether a path
// gained a line containing some substring (matching `BlameAuthor`'s own
// substring-not-exact-line philosophy). `Repository.BlameAuthor` is now
// `Repository.BlameAuthorAt(commit, ...)` under the hood (a thin
// `BlameAuthor` wrapper pinning `commit` to `Repository`'s own one remains,
// non-breaking), so a caller can blame the same path against many different
// commits in one `Repository`'s lifetime, not only the one it's pinned to -
// needed to attribute a historic target version to whoever introduced it.
// `Repository.Root`/`Repository.PinnedCommit` expose what were previously
// private fields, for the same reason: a caller walking history needs the
// working-tree root to translate a commit diff's repository-relative paths
// back into the absolute OS paths every other lookup here expects, and a
// starting point for the walk itself.
//
// **`CloneOrFetch` is replaced by `OpenScope(cacheDir, repoURL, ref)`,
// which never checks a worktree out to disk.** It still ensures a local,
// up-to-date mirror of repoURL exists under cacheDir (cloning on first use,
// fetching `origin` into that same mirror on every later call) and resolves
// ref the same way (a branch, tag, or commit SHA, in that order, or the
// repository's default branch when ref is empty) - but the mirror is now
// *bare*, and instead of a working-tree root, OpenScope returns a read-only
// `fs.FS` over the resolved commit's file tree (via
// `github.com/tfaller/go-git-fs`'s `OpenFromCommit`, reading straight out
// of git's object store) alongside a `Repository` already pinned to that
// same commit, ready for `BlameAuthor`/`FileContents` lookups with no
// on-disk root to discover - a bare mirror holds only git's history/object
// data, which is both smaller and avoids ever having to reconcile a stale
// working tree. Used by [check](@link:check@v0.4.0)'s scope collection to
// materialize a `git:` referenced scope's file tree (see README.md's
// "Scopes" section) before walking it. Breaking: `CloneOrFetch` is gone.
// @doc
// # VCS
//
// `vcs` depends on no other docsweb package; it wraps
// `github.com/go-git/go-git/v6` and, for `OpenScope`,
// `github.com/tfaller/go-git-fs`.
//
// `OpenScope` is how a remote (`git:`) scope's file tree is read, with no
// worktree ever checked out to disk: it mirrors repoURL *bare* into a
// deterministic, hashed subdirectory of cacheDir on first use, or fetches
// `origin` into that same mirror on every later call, resolves `ref` to a
// commit, and returns both an `fs.FS` over that commit's tree (via
// `gitfs.OpenFromCommit`) and a `Repository` pinned to it. `ref` resolution
// tries, in order: a remote-tracking branch (`origin/<ref>`), a tag, then
// any revision `ResolveRevision` itself understands (a commit SHA, `HEAD`,
// ...). An empty `ref` tracks the repository's default branch across repeat
// calls by resolving through that same mirror's one local branch ref
// (created once, by the initial clone, and never touched again) rather
// than through `HEAD` itself, since a fetch only ever moves the matching
// remote-tracking ref.
//
// `Open` discovers the git repository containing a directory (walking
// upward for a `.git` entry, like the git CLI) and returns a `Repository`
// ready for lookups against HEAD. It returns `ErrNoRepository` - never a
// hard failure a caller has to propagate - when the directory isn't inside
// a git working tree or the repository has no commits yet: git attribution
// is optional, best-effort metadata, not something a docsweb build should
// fail over.
//
// `Repository.BlameAuthor` blames one line of one file, against whichever
// commit the `Repository` is pinned to (HEAD, for one opened via `Open`).
// It's given a 1-based line number (checked first, as a fast path) and a
// substring to find - not the line's exact text, so a caller never has to
// re-read the file just to reconstruct it; the substring can be built
// purely from already-in-memory structured data (e.g. a target's name and
// version). Falling back to a full scan for that substring, rather than
// trusting the line number outright, makes this robust against the common
// case where a caller parsed a file's current working-tree contents (which
// may have uncommitted edits elsewhere) rather than the exact committed
// blob - a pure line-number lookup would silently blame the wrong line
// whenever unrelated edits shifted line numbers. Returns `ok == false` (no
// error) when the file isn't tracked at that commit, or no line matches at
// all - again, best-effort rather than a hard failure. The path it's given
// is either an absolute OS path (for a `Repository` opened via `Open`,
// resolved relative to the discovered working-tree root) or already a
// repository-tree-relative path (for one opened via `OpenScope`, which has
// no working tree to resolve against).
//
// `Repository.Commit` resolves any revision string to a `Commit`.
// `Repository.MergeBase` resolves two revisions and returns their common
// ancestor, per `git merge-base` - used to diff a CI merge/pull-request
// branch against what it's actually being merged into, not against that
// target branch's moving tip. `Repository.FileContents` reads a file's
// contents as committed in a given `Commit`, `ok == false` (no error) if
// that file didn't exist yet in that commit - a caller diffing
// documentation against an older revision should treat a brand-new file as
// "nothing to compare against", not a failure.
//
// `WalkFirstParent(start, fn)` walks a commit's history backward, one first
// parent at a time, calling `fn` with each `Step` (a commit and its parent -
// nil for the root commit) until `fn` says to stop or the root commit is
// reached. `DiffStep(step)` turns a `Step` into a `Diff`: `Files` lists every
// file that changed, `AddedLineContains(path, substr)` reports whether that
// file gained a line containing substr - the same substring-matching
// philosophy as `BlameAuthor`, so a caller can check for a tag like
// `@define` without needing a full parse. `Repository.BlameAuthorAt(commit,
// ...)` blames against an arbitrary commit rather than only the one
// `Repository` is pinned to (`BlameAuthor` remains as a thin wrapper for
// that pinned-commit case). Together, these are what
// [history](@link:history@v0.1.0) uses to discover a target's past
// versions without needing a full `collect.AddScope` walk of every commit's
// tree.
// @docsweb

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// Commit identifies one commit - an alias for go-git's object.Commit so
// callers never need to import go-git themselves just to hold a value
// returned by Commit/MergeBase.
type Commit = object.Commit

// ErrNoRepository is returned by Open when dir is not inside a git working
// tree (or that tree has no commits yet). Callers should treat this as "no
// VCS metadata available", not a hard failure - docsweb builds outside of a
// git checkout just fine.
var ErrNoRepository = errors.New("vcs: not a git repository")

// Author identifies who last touched a blamed line, per its introducing
// commit's signature.
type Author struct {
	Name  string
	Email string
}

// String renders Author as "Name <email>", or just one of the two if the
// other is empty, or "" if both are.
func (a Author) String() string {
	switch {
	case a.Name == "":
		return a.Email
	case a.Email == "":
		return a.Name
	default:
		return a.Name + " <" + a.Email + ">"
	}
}

// Repository is a discovered git repository, ready for blame lookups
// against its current HEAD commit.
type Repository struct {
	root   string
	repo   *git.Repository
	commit *object.Commit
	blame  map[blameKey]*git.BlameResult
}

// blameKey caches a BlameAuthorAt result per (commit, path) pair, since
// historic-version lookups blame the same path against many different
// commits within one Repository, not just the single commit it's pinned to.
type blameKey struct {
	commit plumbing.Hash
	path   string
}

// Root returns the absolute path to the working tree Repository was opened
// against via Open, or "" for one opened via OpenScope, which has no
// worktree on disk at all.
func (r *Repository) Root() string {
	return r.root
}

// PinnedCommit returns the commit Repository is pinned to - HEAD, for one
// opened via Open, or whatever commit OpenScope resolved ref to.
func (r *Repository) PinnedCommit() *Commit {
	return r.commit
}

// Open discovers the git repository containing dir (walking upward for a
// .git entry, like the git CLI) and prepares it for blame lookups against
// HEAD. Returns ErrNoRepository if dir is not inside a working tree, or if
// the repository has no commits yet.
func Open(dir string) (*Repository, error) {
	root, ok := findRepoRoot(dir)
	if !ok {
		return nil, ErrNoRepository
	}
	repo, err := git.PlainOpen(root)
	if err != nil {
		return nil, fmt.Errorf("vcs: opening %s: %w", root, err)
	}
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrNoRepository, root, err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("vcs: reading HEAD commit: %w", err)
	}
	return &Repository{root: root, repo: repo, commit: commit, blame: map[blameKey]*git.BlameResult{}}, nil
}

// Commit resolves rev - a full/abbreviated SHA, branch/tag name, "HEAD",
// "origin/main", "HEAD~1", etc. (anything git.Repository.ResolveRevision
// accepts) - to its commit object.
func (r *Repository) Commit(rev string) (*Commit, error) {
	hash, err := r.repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return nil, fmt.Errorf("vcs: resolving revision %q: %w", rev, err)
	}
	c, err := r.repo.CommitObject(*hash)
	if err != nil {
		return nil, fmt.Errorf("vcs: resolving revision %q: %w", rev, err)
	}
	return c, nil
}

// MergeBase returns the merge base of revA and revB - the most recent
// commit both are descended from, per "git merge-base" - so a CI
// merge/pull-request pipeline can diff against what a branch is actually
// being merged into, rather than against that target branch's ever-moving
// tip.
func (r *Repository) MergeBase(revA, revB string) (*Commit, error) {
	a, err := r.Commit(revA)
	if err != nil {
		return nil, err
	}
	b, err := r.Commit(revB)
	if err != nil {
		return nil, err
	}
	bases, err := a.MergeBase(b)
	if err != nil {
		return nil, fmt.Errorf("vcs: merge base of %q and %q: %w", revA, revB, err)
	}
	if len(bases) == 0 {
		return nil, fmt.Errorf("vcs: %q and %q share no common history", revA, revB)
	}
	return bases[0], nil
}

// FileContents returns the contents of the file at path (see relPath's doc
// for what path means for a Repository opened via Open vs OpenScope) as
// committed in c. ok is false, without an error, if that file did not exist
// in c yet (e.g. it was added afterwards) - a caller diffing documentation
// against a base revision should treat a brand-new file as "nothing to
// compare against", not a hard failure.
func (r *Repository) FileContents(c *Commit, path string) (content string, ok bool, err error) {
	rel, err := r.relPath(path)
	if err != nil {
		return "", false, err
	}
	f, err := c.File(rel)
	if err != nil {
		if err == object.ErrFileNotFound {
			return "", false, nil
		}
		return "", false, fmt.Errorf("vcs: reading %s at %s: %w", rel, c.Hash, err)
	}
	content, err = f.Contents()
	if err != nil {
		return "", false, fmt.Errorf("vcs: reading %s at %s: %w", rel, c.Hash, err)
	}
	return content, true, nil
}

// findRepoRoot walks upward from dir looking for a .git entry (a directory
// for a normal checkout, a file for a linked worktree/submodule), returning
// the first containing directory found.
func findRepoRoot(dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			return abs, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", false
		}
		abs = parent
	}
}

// BlameAuthor returns who introduced the line at path (see relPath's doc
// for what path means for a Repository opened via Open vs OpenScope)
// containing the substring contains, per git blame against the
// Repository's pinned commit (HEAD, for one opened via Open). line is a
// 1-based hint into the file's current line numbering, checked first as a
// fast path; if that line doesn't contain the substring (e.g. unrelated
// edits elsewhere in the working tree shifted line numbers since the last
// commit), every blamed line is scanned in order and the first match wins.
// Matching by substring rather than requiring the caller to reproduce a
// line verbatim means BlameAuthor never needs a caller to re-read the file
// - the substring can be built purely from already-in-memory structured
// data (e.g. a target's name and version). Returns ok == false, without
// error, if the file isn't tracked at that commit or no line contains it at
// all - this is best-effort metadata, not something a caller should treat
// as fatal.
func (r *Repository) BlameAuthor(path string, line int, contains string) (author Author, ok bool, err error) {
	return r.BlameAuthorAt(r.commit, path, line, contains)
}

// BlameAuthorAt is like BlameAuthor, but blames against an explicit commit
// rather than the Repository's pinned one - used to attribute a historic
// target version (reconstructed from some ancestor commit) to whoever
// introduced it, the same way BlameAuthor attributes the current version
// against HEAD.
func (r *Repository) BlameAuthorAt(commit *Commit, path string, line int, contains string) (author Author, ok bool, err error) {
	rel, err := r.relPath(path)
	if err != nil {
		return Author{}, false, err
	}

	key := blameKey{commit: commit.Hash, path: rel}
	result, cached := r.blame[key]
	if !cached {
		result, err = git.Blame(commit, rel)
		if err != nil {
			// Not tracked at that commit (new/untracked file, or
			// moved/renamed) - not an error, just no answer.
			return Author{}, false, nil
		}
		r.blame[key] = result
	}

	if contains == "" {
		return Author{}, false, nil
	}
	if idx := line - 1; idx >= 0 && idx < len(result.Lines) && strings.Contains(result.Lines[idx].Text, contains) {
		return authorFromLine(result.Lines[idx]), true, nil
	}
	for _, l := range result.Lines {
		if strings.Contains(l.Text, contains) {
			return authorFromLine(l), true, nil
		}
	}
	return Author{}, false, nil
}

// relPath converts path into the slash-separated, repository-tree-relative
// form every go-git lookup (blame, tree entries) expects. For a Repository
// discovered via Open (a real on-disk working tree), path is an absolute OS
// path inside that tree, resolved relative to the discovered root. For a
// Repository opened directly against a resolved commit via OpenScope (e.g.
// a remote scope's bare mirror, which has no working tree on disk at all,
// so r.root is unset), there is no root to resolve against, and path is
// taken as already being that relative form.
func (r *Repository) relPath(path string) (string, error) {
	if r.root == "" {
		return path, nil
	}
	rel, err := filepath.Rel(r.root, path)
	if err != nil {
		return "", fmt.Errorf("vcs: %s is not under repository root %s: %w", path, r.root, err)
	}
	return filepath.ToSlash(rel), nil
}

func authorFromLine(l *git.Line) Author {
	return Author{Name: l.AuthorName, Email: l.Author}
}
