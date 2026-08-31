package vcs

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalkFirstParentVisitsEveryCommitBackToRoot(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc", "// @define foo v1.0.0"}, alice)

	gitRepo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := gitRepo.Worktree()
	require.NoError(t, err)

	for i := 2; i <= 3; i++ {
		writeLines(t, wtPath(dir), []string{"package doc", "// @define foo v" + strconv.Itoa(i) + ".0.0"})
		_, err = wt.Add("doc.go")
		require.NoError(t, err)
		_, err = wt.Commit("bump", &git.CommitOptions{Author: &alice})
		require.NoError(t, err)
	}

	repo, err := Open(dir)
	require.NoError(t, err)

	var hashes []string
	var parents []bool
	err = WalkFirstParent(repo.PinnedCommit(), func(step Step) (bool, error) {
		hashes = append(hashes, step.Commit.Hash.String())
		parents = append(parents, step.Parent != nil)
		return true, nil
	})
	require.NoError(t, err)

	require.Len(t, hashes, 3)
	assert.Equal(t, []bool{true, true, false}, parents, "only the root commit should report Parent == nil")
}

func TestWalkFirstParentStopsEarly(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc"}, alice)

	gitRepo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := gitRepo.Worktree()
	require.NoError(t, err)
	writeLines(t, wtPath(dir), []string{"package doc", "// second"})
	_, err = wt.Add("doc.go")
	require.NoError(t, err)
	_, err = wt.Commit("second", &git.CommitOptions{Author: &alice})
	require.NoError(t, err)

	repo, err := Open(dir)
	require.NoError(t, err)

	visited := 0
	err = WalkFirstParent(repo.PinnedCommit(), func(step Step) (bool, error) {
		visited++
		return false, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, visited)
}

func TestDiffStepRootCommitReportsEveryFileAsAdded(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc", "// @define foo v1.0.0"}, alice)

	repo, err := Open(dir)
	require.NoError(t, err)

	diff, err := DiffStep(Step{Commit: repo.PinnedCommit(), Parent: nil})
	require.NoError(t, err)

	files := diff.Files()
	require.Len(t, files, 1)
	assert.Equal(t, "doc.go", files[0].Path)
	assert.False(t, files[0].Removed)
	assert.True(t, diff.AddedLineContains("doc.go", "@define"))
}

func TestDiffStepDetectsAddedDefineLineNotRemovedOne(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc", "// @define foo v1.0.0"}, alice)

	gitRepo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := gitRepo.Worktree()
	require.NoError(t, err)

	oldHead, err := gitRepo.Head()
	require.NoError(t, err)
	oldCommit, err := gitRepo.CommitObject(oldHead.Hash())
	require.NoError(t, err)

	writeLines(t, wtPath(dir), []string{"package doc", "// @define foo v2.0.0"})
	_, err = wt.Add("doc.go")
	require.NoError(t, err)
	_, err = wt.Commit("bump", &git.CommitOptions{Author: &alice})
	require.NoError(t, err)

	repo, err := Open(dir)
	require.NoError(t, err)

	diff, err := DiffStep(Step{Commit: repo.PinnedCommit(), Parent: oldCommit})
	require.NoError(t, err)

	// The old @define line was removed and a new one added in the same
	// commit - only the added line should register, per the rule that a
	// removed @define line is never itself informative (its version was
	// already captured when it was first introduced).
	assert.True(t, diff.AddedLineContains("doc.go", "@define foo v2.0.0"))
	assert.False(t, diff.AddedLineContains("doc.go", "@define foo v1.0.0"))
}

func TestDiffStepReportsRemovedFile(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc"}, alice)

	gitRepo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := gitRepo.Worktree()
	require.NoError(t, err)

	oldHead, err := gitRepo.Head()
	require.NoError(t, err)
	oldCommit, err := gitRepo.CommitObject(oldHead.Hash())
	require.NoError(t, err)

	_, err = wt.Remove("doc.go")
	require.NoError(t, err)
	_, err = wt.Commit("remove", &git.CommitOptions{Author: &alice})
	require.NoError(t, err)

	repo, err := Open(dir)
	require.NoError(t, err)

	diff, err := DiffStep(Step{Commit: repo.PinnedCommit(), Parent: oldCommit})
	require.NoError(t, err)

	files := diff.Files()
	require.Len(t, files, 1)
	assert.Equal(t, "doc.go", files[0].Path)
	assert.True(t, files[0].Removed)
}

func TestBlameAuthorAtBlamesAnArbitraryCommitNotJustThePinnedOne(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc", "// @define foo v1.0.0"}, alice)

	gitRepo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := gitRepo.Worktree()
	require.NoError(t, err)

	oldHead, err := gitRepo.Head()
	require.NoError(t, err)
	oldCommit, err := gitRepo.CommitObject(oldHead.Hash())
	require.NoError(t, err)

	writeLines(t, wtPath(dir), []string{"package doc", "// @define foo v2.0.0"})
	_, err = wt.Add("doc.go")
	require.NoError(t, err)
	bob := object.Signature{Name: "Bob", Email: "bob@example.com", When: fixedTime().Add(time.Hour)}
	_, err = wt.Commit("bump", &git.CommitOptions{Author: &bob})
	require.NoError(t, err)

	repo, err := Open(dir)
	require.NoError(t, err)

	// Blaming against HEAD finds Bob's bump; blaming the same path against
	// the old commit directly must still find Alice's original line, even
	// though Repository stays pinned to HEAD.
	head, ok, err := repo.BlameAuthorAt(repo.PinnedCommit(), wtPath(dir), 2, "@define foo v2.0.0")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "Bob", head.Name)

	old, ok, err := repo.BlameAuthorAt(oldCommit, wtPath(dir), 2, "@define foo v1.0.0")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "Alice", old.Name)
}

func TestRootAndPinnedCommit(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc"}, alice)

	repo, err := Open(dir)
	require.NoError(t, err)

	abs, err := filepath.Abs(dir)
	require.NoError(t, err)
	assert.Equal(t, abs, repo.Root())
	require.NotNil(t, repo.PinnedCommit())
	assert.Equal(t, repo.commit.Hash, repo.PinnedCommit().Hash)
}

func wtPath(dir string) string {
	return filepath.Join(dir, "doc.go")
}
