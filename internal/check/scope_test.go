package check

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initGitScope creates a fresh git repository at dir containing a
// .docsweb.yaml self-declaring scopeName, plus a Go file whose @define name
// is targetName, and commits both - usable as a remote (git:) scope for
// checkScopes to clone.
func initGitScope(t *testing.T, dir, scopeName, targetName string) *git.Repository {
	t.Helper()

	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".docsweb.yaml"), []byte("name: "+scopeName+"\n"), 0o644))
	src := "package remote\n\n/*\n    @docsweb\n    @define " + targetName + " v1.0.0\n    @doc\n    Remote docs.\n    @docsweb\n*/\n\nfunc F() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(src), 0o644))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	author := object.Signature{Name: "Alice", Email: "alice@example.com", When: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	_, err = wt.Commit("initial", &git.CommitOptions{Author: &author})
	require.NoError(t, err)

	return repo
}

func writeRootConfig(t *testing.T, dir, yaml string) string {
	t.Helper()
	path := filepath.Join(dir, ".docsweb.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o644))
	return path
}

func TestCheckScopesClonesRemoteScope(t *testing.T) {
	remoteDir := t.TempDir()
	initGitScope(t, remoteDir, "upstream", "widget")

	rootDir := t.TempDir()
	cfgPath := writeRootConfig(t, rootDir, "name: root\nscope:\n    upstream:\n        git: "+remoteDir+"\n        path: .\n")

	result, err := Run(Options{ConfigPath: cfgPath})
	require.NoError(t, err)

	require.Len(t, result.Registry.Targets(), 1)
	target, ok := result.Registry.Get("upstream.widget")
	require.True(t, ok)
	assert.Equal(t, "upstream", target.Scope)

	// The clone lives under the root scope's own directory (docsweb-cache)
	// but must not be picked up as part of the root scope's own walk.
	assert.DirExists(t, filepath.Join(rootDir, "docsweb-cache"))
}

func TestCheckScopesRemoteScopeNameMismatch(t *testing.T) {
	remoteDir := t.TempDir()
	initGitScope(t, remoteDir, "actualname", "widget")

	rootDir := t.TempDir()
	cfgPath := writeRootConfig(t, rootDir, "name: root\nscope:\n    expectedname:\n        git: "+remoteDir+"\n        path: .\n")

	_, err := Run(Options{ConfigPath: cfgPath})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `declares name "actualname"`)
	assert.Contains(t, err.Error(), `expected "expectedname"`)
}

func TestCheckScopesRemoteScopeUnresolvableRefIsError(t *testing.T) {
	remoteDir := t.TempDir()
	initGitScope(t, remoteDir, "upstream", "widget")

	rootDir := t.TempDir()
	cfgPath := writeRootConfig(t, rootDir, "name: root\nscope:\n    upstream:\n        git: "+remoteDir+"\n        path: .\n        ref: does-not-exist\n")

	_, err := Run(Options{ConfigPath: cfgPath})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving ref")
}

func TestCheckScopesRemoteScopeUnresolvableURLIsError(t *testing.T) {
	rootDir := t.TempDir()
	cfgPath := writeRootConfig(t, rootDir, "name: root\nscope:\n    upstream:\n        git: "+filepath.Join(t.TempDir(), "does-not-exist")+"\n        path: .\n")

	_, err := Run(Options{ConfigPath: cfgPath})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `scope "upstream"`)
}

func TestCheckScopesRemoteScopePicksUpNewCommitsAcrossRuns(t *testing.T) {
	remoteDir := t.TempDir()
	initGitScope(t, remoteDir, "upstream", "widget")

	rootDir := t.TempDir()
	cfgPath := writeRootConfig(t, rootDir, "name: root\nscope:\n    upstream:\n        git: "+remoteDir+"\n        path: .\n")

	result, err := Run(Options{ConfigPath: cfgPath})
	require.NoError(t, err)
	require.Len(t, result.Registry.Targets(), 1)

	// A second target lands in a fresh commit on the same remote; a second
	// Run against the same root config must pick it up via a fetch against
	// the already-cached clone, not silently keep serving the first commit.
	repo, err := git.PlainOpen(remoteDir)
	require.NoError(t, err)
	src := "package remote\n\n/*\n    @docsweb\n    @define gadget v1.0.0\n    @doc\n    More remote docs.\n    @docsweb\n*/\n\nfunc G() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(remoteDir, "b.go"), []byte(src), 0o644))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	author := object.Signature{Name: "Alice", Email: "alice@example.com", When: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}
	_, err = wt.Commit("add gadget", &git.CommitOptions{Author: &author})
	require.NoError(t, err)

	result, err = Run(Options{ConfigPath: cfgPath})
	require.NoError(t, err)
	require.Len(t, result.Registry.Targets(), 2)
	_, ok := result.Registry.Get("upstream.gadget")
	assert.True(t, ok)
}
