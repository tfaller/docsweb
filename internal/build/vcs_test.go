package build

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initTempRepo writes files (relative path -> content) under a fresh temp
// directory, commits them all to a brand-new git repository with the given
// author, and returns the directory.
func initTempRepo(t *testing.T, files map[string]string, author object.Signature) string {
	t.Helper()

	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)

	for rel, content := range files {
		path := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	wt, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, wt.AddGlob("."))
	_, err = wt.Commit("initial", &gogit.CommitOptions{Author: &author})
	require.NoError(t, err)

	return dir
}

func TestRunAttributesVersionToItsGitBlameAuthor(t *testing.T) {
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: time.Now()}
	dir := initTempRepo(t, map[string]string{
		".docsweb.yaml": "name: standalone\n",
		"main.go": "package integration\n\n" +
			"/*\n" +
			"    @docsweb\n" +
			"    @define app v1.0.0\n" +
			"    @docsweb\n" +
			"*/\n",
	}, alice)

	result, err := Run(Options{ConfigPath: filepath.Join(dir, ".docsweb.yaml")})
	require.NoError(t, err)
	require.Len(t, result.Targets, 1)
	require.Equal(t, "Alice <alice@example.com>", result.Targets[0].Author)
	// The version's introducing commit is the same one history.Walk finds
	// via its added-@define-line detection - its short hash and committer
	// timestamp (defaulted to the author's, since none was set explicitly).
	assert.Len(t, result.Targets[0].CommitHash, 7)
	assert.WithinDuration(t, alice.When, result.Targets[0].CommitTime, time.Second)
	require.Len(t, result.Targets[0].Versions, 1)
	assert.Equal(t, result.Targets[0].CommitHash, result.Targets[0].Versions[0].CommitHash)
	assert.WithinDuration(t, alice.When, result.Targets[0].Versions[0].CommitTime, time.Second)
}

func TestRunLeavesAuthorEmptyOutsideGitRepository(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".docsweb.yaml"), []byte("name: standalone\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(
		"package standalone\n\n"+
			"/*\n"+
			"    @docsweb\n"+
			"    @define app v1.0.0\n"+
			"    @docsweb\n"+
			"*/\n"), 0o644))

	result, err := Run(Options{ConfigPath: filepath.Join(dir, ".docsweb.yaml")})
	require.NoError(t, err)
	require.Len(t, result.Targets, 1)
	require.Empty(t, result.Targets[0].Author)
	assert.Empty(t, result.Targets[0].CommitHash)
	assert.True(t, result.Targets[0].CommitTime.IsZero())
}
