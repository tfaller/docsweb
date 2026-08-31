package vcs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	fdiff "github.com/go-git/go-git/v6/plumbing/format/diff"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// Step is one first-parent transition in a history walk: Commit is a commit
// reached by the walk, Parent is its first parent (nil if Commit is the
// repository's root commit, which has no parent - see WalkFirstParent).
type Step struct {
	Commit *Commit
	Parent *Commit
}

// WalkFirstParent calls fn once per commit reached by following first
// parents backward from start (inclusive) to the repository's root commit.
// fn returns false to stop the walk early (e.g. once a caller has all the
// information it needs), or a non-nil error to abort it. Side-branch
// commits (anything only reachable via a merge commit's non-first parent)
// are never visited - this is a mainline-only walk, not a full ancestry
// traversal.
func WalkFirstParent(start *Commit, fn func(Step) (bool, error)) error {
	c := start
	for {
		parent, err := c.Parent(0)
		if err != nil {
			if !errors.Is(err, object.ErrParentNotFound) {
				return fmt.Errorf("vcs: walking history from %s: %w", c.Hash, err)
			}
			parent = nil
		}

		cont, err := fn(Step{Commit: c, Parent: parent})
		if err != nil || !cont || parent == nil {
			return err
		}
		c = parent
	}
}

// Diff is the set of file-level changes between the two commits of a Step.
type Diff struct {
	patch *object.Patch
}

// DiffFile is one file that differs between a Step's Parent and Commit.
type DiffFile struct {
	// Path is the file's path as of Commit (repository-tree-relative,
	// slash-separated), or as of Parent if the file was removed by Commit -
	// there is no "as of Commit" path to report in that case.
	Path string
	// Removed is true if the file existed in Parent but not in Commit.
	Removed bool
}

// DiffStep computes the file-level changes of step: every file that differs
// between step.Parent and step.Commit. step.Parent may be nil (the
// repository's root commit, per WalkFirstParent), in which case every file
// in step.Commit is reported as a (non-removed) change, since there is no
// prior state to compare against.
func DiffStep(step Step) (*Diff, error) {
	var fromTree *object.Tree
	if step.Parent != nil {
		t, err := step.Parent.Tree()
		if err != nil {
			return nil, fmt.Errorf("vcs: reading tree of %s: %w", step.Parent.Hash, err)
		}
		fromTree = t
	}
	toTree, err := step.Commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("vcs: reading tree of %s: %w", step.Commit.Hash, err)
	}

	patch, err := fromTree.PatchContext(context.Background(), toTree)
	if err != nil {
		return nil, fmt.Errorf("vcs: diffing %s: %w", step.Commit.Hash, err)
	}
	return &Diff{patch: patch}, nil
}

// Files returns every file that changed in d, in no particular order.
func (d *Diff) Files() []DiffFile {
	fps := d.patch.FilePatches()
	files := make([]DiffFile, 0, len(fps))
	for _, fp := range fps {
		from, to := fp.Files()
		switch {
		case to != nil:
			files = append(files, DiffFile{Path: to.Path()})
		case from != nil:
			files = append(files, DiffFile{Path: from.Path(), Removed: true})
		}
	}
	return files
}

// AddedLineContains reports whether path (as reported by Files, and not
// Removed) gained a line, relative to the diff's older side, whose content
// contains substr. Like Repository.BlameAuthor, this matches by substring
// rather than requiring a caller to reproduce a line verbatim - a docsweb
// tag line ("@define name vX.Y.Z") can be matched purely from the tag token
// itself, or from already-in-memory structured data, without re-reading the
// file.
func (d *Diff) AddedLineContains(path, substr string) bool {
	for _, fp := range d.patch.FilePatches() {
		_, to := fp.Files()
		if to == nil || to.Path() != path {
			continue
		}
		for _, chunk := range fp.Chunks() {
			if chunk.Type() != fdiff.Add {
				continue
			}
			for _, line := range strings.Split(chunk.Content(), "\n") {
				if strings.Contains(line, substr) {
					return true
				}
			}
		}
		return false
	}
	return false
}
