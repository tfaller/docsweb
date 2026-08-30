package vcs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initRemote creates a repository at dir with a single committed file on
// its default branch, usable as a local "remote" for CloneOrFetch (go-git
// clones from a plain local path the same way it would from any other git
// URL).
func initRemote(t *testing.T, dir, content string, author object.Signature) *git.Repository {
	t.Helper()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte(content), 0o644))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("file.txt")
	require.NoError(t, err)
	_, err = wt.Commit("initial", &git.CommitOptions{Author: &author})
	require.NoError(t, err)

	return repo
}

func TestCloneOrFetchClonesDefaultBranch(t *testing.T) {
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	remoteDir := t.TempDir()
	initRemote(t, remoteDir, "v1", alice)

	cacheDir := t.TempDir()
	cloneDir, err := CloneOrFetch(cacheDir, remoteDir, "")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(cloneDir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(content))
}

func TestCloneOrFetchResolvesBranch(t *testing.T) {
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	remoteDir := t.TempDir()
	remote := initRemote(t, remoteDir, "v1", alice)

	wt, err := remote.Worktree()
	require.NoError(t, err)
	require.NoError(t, wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("feature"), Create: true}))
	require.NoError(t, os.WriteFile(filepath.Join(remoteDir, "file.txt"), []byte("v2"), 0o644))
	_, err = wt.Add("file.txt")
	require.NoError(t, err)
	_, err = wt.Commit("feature work", &git.CommitOptions{Author: &alice})
	require.NoError(t, err)

	cacheDir := t.TempDir()
	cloneDir, err := CloneOrFetch(cacheDir, remoteDir, "feature")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(cloneDir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v2", string(content))
}

func TestCloneOrFetchResolvesTag(t *testing.T) {
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	remoteDir := t.TempDir()
	remote := initRemote(t, remoteDir, "v1", alice)

	head, err := remote.Head()
	require.NoError(t, err)
	_, err = remote.CreateTag("v1.0.0", head.Hash(), nil)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(remoteDir, "file.txt"), []byte("v2"), 0o644))
	wt, err := remote.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("file.txt")
	require.NoError(t, err)
	_, err = wt.Commit("later work", &git.CommitOptions{Author: &alice})
	require.NoError(t, err)

	cacheDir := t.TempDir()
	cloneDir, err := CloneOrFetch(cacheDir, remoteDir, "v1.0.0")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(cloneDir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(content))
}

func TestCloneOrFetchReusesExistingCloneAndPicksUpNewCommits(t *testing.T) {
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	remoteDir := t.TempDir()
	remote := initRemote(t, remoteDir, "v1", alice)

	cacheDir := t.TempDir()
	_, err := CloneOrFetch(cacheDir, remoteDir, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(remoteDir, "file.txt"), []byte("v2"), 0o644))
	wt, err := remote.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("file.txt")
	require.NoError(t, err)
	_, err = wt.Commit("update", &git.CommitOptions{Author: &alice})
	require.NoError(t, err)

	cloneDir, err := CloneOrFetch(cacheDir, remoteDir, "")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(cloneDir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v2", string(content))
}

func TestCloneOrFetchUnknownRefIsError(t *testing.T) {
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	remoteDir := t.TempDir()
	initRemote(t, remoteDir, "v1", alice)

	cacheDir := t.TempDir()
	_, err := CloneOrFetch(cacheDir, remoteDir, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving ref")
}

func TestCloneOrFetchInvalidRepoURLIsError(t *testing.T) {
	cacheDir := t.TempDir()
	_, err := CloneOrFetch(cacheDir, filepath.Join(t.TempDir(), "does-not-exist"), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cloning")
}
