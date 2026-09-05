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

func TestCheckScopesRemoteScopeAppliesOwnIgnoreRules(t *testing.T) {
	remoteDir := t.TempDir()
	initGitScope(t, remoteDir, "upstream", "widget")

	// A second file redefining "widget" would normally be a hard duplicate
	// error - unless the remote scope's own ignore rules (declared in its
	// own .docsweb.yaml, not the root's) exclude it.
	require.NoError(t, os.WriteFile(filepath.Join(remoteDir, ".docsweb.yaml"), []byte("name: upstream\nignore:\n    - /excluded.go\n"), 0o644))
	src := "package remote\n\n/*\n    @docsweb\n    @define widget v1.0.0\n    @doc\n    Duplicate, but ignored.\n    @docsweb\n*/\n\nfunc Dup() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(remoteDir, "excluded.go"), []byte(src), 0o644))

	repo, err := git.PlainOpen(remoteDir)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	author := object.Signature{Name: "Alice", Email: "alice@example.com", When: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	_, err = wt.Commit("add ignored duplicate", &git.CommitOptions{Author: &author})
	require.NoError(t, err)

	rootDir := t.TempDir()
	cfgPath := writeRootConfig(t, rootDir, "name: root\nscope:\n    upstream:\n        git: "+remoteDir+"\n        path: .\n")

	result, err := Run(Options{ConfigPath: cfgPath})
	require.NoError(t, err)
	require.Len(t, result.Registry.Targets(), 1)
	_, ok := result.Registry.Get("upstream.widget")
	assert.True(t, ok)
}

func TestCheckScopesLocalScopeAppliesOwnIgnoreRules(t *testing.T) {
	rootDir := t.TempDir()
	scopeDir := filepath.Join(rootDir, "sub")
	require.NoError(t, os.MkdirAll(scopeDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(scopeDir, ".docsweb.yaml"), []byte("name: sub\nignore:\n    - /excluded.go\n"), 0o644))
	src := "package sub\n\n/*\n    @docsweb\n    @define widget v1.0.0\n    @doc\n    Sub docs.\n    @docsweb\n*/\n\nfunc F() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(scopeDir, "a.go"), []byte(src), 0o644))

	// Duplicate, but excluded by "sub"'s own ignore: rules, which the root
	// config never declares - only the referenced scope's own
	// .docsweb.yaml does.
	dup := "package sub\n\n/*\n    @docsweb\n    @define widget v1.0.0\n    @doc\n    Duplicate, but ignored.\n    @docsweb\n*/\n\nfunc Dup() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(scopeDir, "excluded.go"), []byte(dup), 0o644))

	cfgPath := writeRootConfig(t, rootDir, "name: root\nscope:\n    sub:\n        path: sub\n")

	result, err := Run(Options{ConfigPath: cfgPath})
	require.NoError(t, err)
	require.Len(t, result.Registry.Targets(), 1)
	_, ok := result.Registry.Get("sub.widget")
	assert.True(t, ok)
}

// The root config's own ignore: rules describe the root scope's own tree
// only - they must not reach into a local referenced scope's tree, even
// though it lives on disk underneath the root scope's own directory.
func TestCheckScopesLocalScopeIgnoresRootIgnoreRules(t *testing.T) {
	rootDir := t.TempDir()
	scopeDir := filepath.Join(rootDir, "sub")
	require.NoError(t, os.MkdirAll(scopeDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(scopeDir, ".docsweb.yaml"), []byte("name: sub\n"), 0o644))
	src := "package sub\n\n/*\n    @docsweb\n    @define widget v1.0.0\n    @doc\n    Sub docs.\n    @docsweb\n*/\n\nfunc F() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(scopeDir, "widget.go"), []byte(src), 0o644))

	// The root's own ignore: rule names this file by its full root-relative
	// path - if it were (wrongly) applied to the "sub" scope's own walk,
	// "widget" would never be found.
	cfgPath := writeRootConfig(t, rootDir, "name: root\nignore:\n    - /sub/widget.go\nscope:\n    sub:\n        path: sub\n")

	result, err := Run(Options{ConfigPath: cfgPath})
	require.NoError(t, err)
	require.Len(t, result.Registry.Targets(), 1)
	_, ok := result.Registry.Get("sub.widget")
	assert.True(t, ok)
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
