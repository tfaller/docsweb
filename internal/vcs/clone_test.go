package vcs

import (
	"io/fs"
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
// its default branch, usable as a local "remote" for OpenScope (go-git
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

func TestOpenScopeClonesDefaultBranch(t *testing.T) {
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	remoteDir := t.TempDir()
	initRemote(t, remoteDir, "v1", alice)

	cacheDir := t.TempDir()
	treeFS, _, err := OpenScope(cacheDir, remoteDir, "")
	require.NoError(t, err)

	content, err := fs.ReadFile(treeFS, "file.txt")
	require.NoError(t, err)
	assert.Equal(t, "v1", string(content))
}

// A bare mirror never checks a worktree out to disk - only git's own
// object/ref data lives under the cache directory.
func TestOpenScopeNeverChecksOutAWorktree(t *testing.T) {
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	remoteDir := t.TempDir()
	initRemote(t, remoteDir, "v1", alice)

	cacheDir := t.TempDir()
	_, _, err := OpenScope(cacheDir, remoteDir, "")
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(cacheDir, cloneDirName(remoteDir)))
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotEqual(t, "file.txt", e.Name(), "bare mirror must not contain a checked-out worktree file")
	}
}

func TestOpenScopeResolvesBranch(t *testing.T) {
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
	treeFS, _, err := OpenScope(cacheDir, remoteDir, "feature")
	require.NoError(t, err)

	content, err := fs.ReadFile(treeFS, "file.txt")
	require.NoError(t, err)
	assert.Equal(t, "v2", string(content))
}

func TestOpenScopeResolvesTag(t *testing.T) {
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
	treeFS, _, err := OpenScope(cacheDir, remoteDir, "v1.0.0")
	require.NoError(t, err)

	content, err := fs.ReadFile(treeFS, "file.txt")
	require.NoError(t, err)
	assert.Equal(t, "v1", string(content))
}

func TestOpenScopeReusesExistingMirrorAndPicksUpNewCommits(t *testing.T) {
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	remoteDir := t.TempDir()
	remote := initRemote(t, remoteDir, "v1", alice)

	cacheDir := t.TempDir()
	_, _, err := OpenScope(cacheDir, remoteDir, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(remoteDir, "file.txt"), []byte("v2"), 0o644))
	wt, err := remote.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("file.txt")
	require.NoError(t, err)
	_, err = wt.Commit("update", &git.CommitOptions{Author: &alice})
	require.NoError(t, err)

	treeFS, _, err := OpenScope(cacheDir, remoteDir, "")
	require.NoError(t, err)

	content, err := fs.ReadFile(treeFS, "file.txt")
	require.NoError(t, err)
	assert.Equal(t, "v2", string(content))
}

func TestOpenScopeUnknownRefIsError(t *testing.T) {
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	remoteDir := t.TempDir()
	initRemote(t, remoteDir, "v1", alice)

	cacheDir := t.TempDir()
	_, _, err := OpenScope(cacheDir, remoteDir, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving ref")
}

func TestOpenScopeInvalidRepoURLIsError(t *testing.T) {
	cacheDir := t.TempDir()
	_, _, err := OpenScope(cacheDir, filepath.Join(t.TempDir(), "does-not-exist"), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cloning")
}

// The Repository OpenScope returns is pinned to the resolved commit - blame
// works against it exactly as it does for a Repository discovered on disk
// via Open, even though there is no worktree to discover a root from.
func TestOpenScopeRepositoryBlameAuthor(t *testing.T) {
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	remoteDir := t.TempDir()
	repo, err := git.PlainInit(remoteDir, false)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(remoteDir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(remoteDir, "sub", "doc.go"), []byte("package doc\n// @define foo v1.0.0"), 0o644))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	_, err = wt.Commit("initial", &git.CommitOptions{Author: &alice})
	require.NoError(t, err)

	cacheDir := t.TempDir()
	_, scopeRepo, err := OpenScope(cacheDir, remoteDir, "")
	require.NoError(t, err)

	author, ok, err := scopeRepo.BlameAuthor("sub/doc.go", 2, "@define foo v1.0.0")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Author{Name: "Alice", Email: "alice@example.com"}, author)
}
