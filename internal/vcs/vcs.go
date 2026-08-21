// Package vcs looks up, via git blame, who last touched a specific source
// line. docsweb uses this to attribute a target's current version to the
// commit author who made the @define bump - see README.md's "The
// documentation system knows about Version Control" paragraph.
package vcs

// @docsweb
// @define vcs v0.1.0
// @name VCS
// @summary
// Git-blame lookups: who last touched a given source line, used to
// attribute a target's current version to the commit author who bumped it.
// @audience dev
// @changelog
// Initial documentation.
// @doc
// # VCS
//
// `vcs` depends on no other docsweb package; it wraps
// `github.com/go-git/go-git/v6` blame against a repository's current HEAD
// commit.
//
// `Open` discovers the git repository containing a directory (walking
// upward for a `.git` entry, like the git CLI) and returns a `Repository`
// ready for blame lookups against HEAD. It returns `ErrNoRepository` -
// never a hard failure a caller has to propagate - when the directory
// isn't inside a git working tree or the repository has no commits yet:
// git attribution is optional, best-effort metadata, not something a
// docsweb build should fail over.
//
// `Repository.BlameAuthor` blames one line of one file. It's given a
// 1-based line number (checked first, as a fast path) and a substring to
// find - not the line's exact text, so a caller never has to re-read the
// file just to reconstruct it; the substring can be built purely from
// already-in-memory structured data (e.g. a target's name and version).
// Falling back to a full scan for that substring, rather than trusting the
// line number outright, makes this robust against the common case where a
// caller parsed a file's current working-tree contents (which may have
// uncommitted edits elsewhere) rather than the exact blob HEAD has
// committed - a pure line-number lookup would silently blame the wrong
// line whenever unrelated edits shifted line numbers. Returns `ok == false`
// (no error) when the file isn't tracked at HEAD, or no line matches at
// all - again, best-effort rather than a hard failure.
// @docsweb

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
)

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
	commit *object.Commit
	blame  map[string]*git.BlameResult
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
	return &Repository{root: root, commit: commit, blame: map[string]*git.BlameResult{}}, nil
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

// BlameAuthor returns who introduced the line at absPath (an absolute path
// inside the repository) containing the substring contains, per git blame
// against HEAD. line is a 1-based hint into the file's current line
// numbering, checked first as a fast path; if that line doesn't contain the
// substring (e.g. unrelated edits elsewhere in the working tree shifted line
// numbers since the last commit), every blamed line is scanned in order and
// the first match wins. Matching by substring rather than requiring the
// caller to reproduce a line verbatim means BlameAuthor never needs a
// caller to re-read the file - the substring can be built purely from
// already-in-memory structured data (e.g. a target's name and version).
// Returns ok == false, without error, if the file isn't tracked at HEAD or
// no line contains it at all - this is best-effort metadata, not something
// a caller should treat as fatal.
func (r *Repository) BlameAuthor(absPath string, line int, contains string) (author Author, ok bool, err error) {
	rel, err := filepath.Rel(r.root, absPath)
	if err != nil {
		return Author{}, false, fmt.Errorf("vcs: %s is not under repository root %s: %w", absPath, r.root, err)
	}
	rel = filepath.ToSlash(rel)

	result, cached := r.blame[rel]
	if !cached {
		result, err = git.Blame(r.commit, rel)
		if err != nil {
			// Not tracked at HEAD (new/untracked file, or moved/renamed) -
			// not an error, just no answer.
			return Author{}, false, nil
		}
		r.blame[rel] = result
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

func authorFromLine(l *git.Line) Author {
	return Author{Name: l.AuthorName, Email: l.Author}
}
