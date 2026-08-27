package check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tfaller/docsweb/internal/vcs"
)

// docsweb block bodies, kept as constants so tests read as "before" vs
// "after" rather than reconstructing the grammar inline every time.
const docV1 = `package widget

/*
    @docsweb
    @define widget v1.0.0
    @doc
    Version one.
    @changelog
    Initial documentation.
    @docsweb
*/
`

const docV1BumpedNoChangelog = `package widget

/*
    @docsweb
    @define widget v1.1.0
    @doc
    Version two, with more detail.
    @changelog
    Initial documentation.
    @docsweb
*/
`

const docV1UndocumentedChange = `package widget

/*
    @docsweb
    @define widget v1.0.0
    @doc
    Version two, with more detail.
    @changelog
    Initial documentation.
    @docsweb
*/
`

const docV2Full = `package widget

/*
    @docsweb
    @define widget v1.1.0
    @doc
    Version two, with more detail.
    @changelog
    Added more detail.
    @docsweb
*/
`

// initVersionBumpRepo creates a fresh git repo at dir with a minimal
// .docsweb.yaml and a.go (containing content), committed. Returns the repo
// and the path to a.go.
func initVersionBumpRepo(t *testing.T, dir, content string) (*git.Repository, string) {
	t.Helper()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".docsweb.yaml"), []byte("name: widget\n"), 0o644))
	path := filepath.Join(dir, "a.go")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	initialSig := sig()
	_, err = wt.Commit("initial", &git.CommitOptions{Author: &initialSig})
	require.NoError(t, err)

	return repo, path
}

func sig() object.Signature {
	return object.Signature{Name: "Alice", Email: "alice@example.com"}
}

func TestCheckVersionBumpRequiresVersionBump(t *testing.T) {
	dir := t.TempDir()
	_, path := initVersionBumpRepo(t, dir, docV1)

	// Uncommitted: documentation changed, version didn't.
	require.NoError(t, os.WriteFile(path, []byte(docV1UndocumentedChange), 0o644))

	_, err := Run(Options{ConfigPath: filepath.Join(dir, ".docsweb.yaml")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version was not bumped")
}

func TestCheckVersionBumpRequiresChangelogUpdate(t *testing.T) {
	dir := t.TempDir()
	_, path := initVersionBumpRepo(t, dir, docV1)

	// Uncommitted: documentation changed and version was bumped, but the
	// changelog still just says "Initial documentation."
	require.NoError(t, os.WriteFile(path, []byte(docV1BumpedNoChangelog), 0o644))

	_, err := Run(Options{ConfigPath: filepath.Join(dir, ".docsweb.yaml")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "@changelog was not updated")
}

func TestCheckVersionBumpPassesWhenVersionAndChangelogBothUpdated(t *testing.T) {
	dir := t.TempDir()
	_, path := initVersionBumpRepo(t, dir, docV1)

	require.NoError(t, os.WriteFile(path, []byte(docV2Full), 0o644))

	_, err := Run(Options{ConfigPath: filepath.Join(dir, ".docsweb.yaml")})
	assert.NoError(t, err)
}

func TestCheckVersionBumpIgnoresChangesOutsideTheDocsweBlock(t *testing.T) {
	dir := t.TempDir()
	_, path := initVersionBumpRepo(t, dir, docV1)

	// Uncommitted: only ordinary Go code changed, nothing inside the
	// @docsweb block - not a documentation change, so no bump is required.
	content := docV1 + "\nvar _ = 1\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	_, err := Run(Options{ConfigPath: filepath.Join(dir, ".docsweb.yaml")})
	assert.NoError(t, err)
}

func TestCheckVersionBumpSkipsBrandNewTarget(t *testing.T) {
	dir := t.TempDir()
	initVersionBumpRepo(t, dir, docV1)

	// A second, previously nonexistent target: nothing to diff against.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte(`package widget

/*
    @docsweb
    @define gadget v1.0.0
    @doc
    Brand new.
    @changelog
    Initial documentation.
    @docsweb
*/
`), 0o644))

	_, err := Run(Options{ConfigPath: filepath.Join(dir, ".docsweb.yaml")})
	assert.NoError(t, err)
}

func TestCheckVersionBumpSkipsOutsideGitRepository(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".docsweb.yaml"), []byte("name: widget\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(docV1UndocumentedChange), 0o644))

	_, err := Run(Options{ConfigPath: filepath.Join(dir, ".docsweb.yaml")})
	assert.NoError(t, err)
}

func TestCheckVersionBumpExplicitBaseOverride(t *testing.T) {
	dir := t.TempDir()
	repo, path := initVersionBumpRepo(t, dir, docV1)

	wt, err := repo.Worktree()
	require.NoError(t, err)
	firstCommit, err := repo.Head()
	require.NoError(t, err)

	// A second, *bad* commit: version bumped, documentation changed, but
	// the changelog was left saying "Initial documentation." - exactly the
	// mistake this check exists to catch, just already committed.
	require.NoError(t, os.WriteFile(path, []byte(docV1BumpedNoChangelog), 0o644))
	_, err = wt.Add(".")
	require.NoError(t, err)
	secondSig := sig()
	_, err = wt.Commit("bad bump", &git.CommitOptions{Author: &secondSig})
	require.NoError(t, err)

	configPath := filepath.Join(dir, ".docsweb.yaml")

	// Default base (HEAD): working tree matches the last commit exactly,
	// so there's nothing to diff - the bad commit's mistake is invisible.
	_, err = Run(Options{ConfigPath: configPath})
	assert.NoError(t, err)

	// Explicit base pointing further back, at the first commit: now the
	// same bad commit's change is visible as a diff, and the missing
	// changelog update is caught.
	_, err = Run(Options{ConfigPath: configPath, Base: firstCommit.Hash().String()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "@changelog was not updated")
}

func TestCiBaseRevision(t *testing.T) {
	envVars := []string{
		"CI_MERGE_REQUEST_DIFF_BASE_SHA",
		"CI_MERGE_REQUEST_TARGET_BRANCH_NAME",
		"GITHUB_ACTIONS",
		"GITHUB_EVENT_NAME",
		"GITHUB_BASE_REF",
	}
	clear := func(t *testing.T) {
		for _, v := range envVars {
			t.Setenv(v, "")
		}
	}

	t.Run("no CI context", func(t *testing.T) {
		clear(t)
		_, _, ok := ciBaseRevision()
		assert.False(t, ok)
	})

	t.Run("gitlab merge request diff base sha", func(t *testing.T) {
		clear(t)
		t.Setenv("CI_MERGE_REQUEST_DIFF_BASE_SHA", "abc123")
		rev, mergeBase, ok := ciBaseRevision()
		require.True(t, ok)
		assert.Equal(t, "abc123", rev)
		assert.False(t, mergeBase)
	})

	t.Run("gitlab merge request target branch only", func(t *testing.T) {
		clear(t)
		t.Setenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME", "main")
		rev, mergeBase, ok := ciBaseRevision()
		require.True(t, ok)
		assert.Equal(t, "origin/main", rev)
		assert.True(t, mergeBase)
	})

	t.Run("github pull request", func(t *testing.T) {
		clear(t)
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_EVENT_NAME", "pull_request")
		t.Setenv("GITHUB_BASE_REF", "main")
		rev, mergeBase, ok := ciBaseRevision()
		require.True(t, ok)
		assert.Equal(t, "origin/main", rev)
		assert.True(t, mergeBase)
	})

	t.Run("github actions but not a pull request", func(t *testing.T) {
		clear(t)
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_EVENT_NAME", "push")
		t.Setenv("GITHUB_BASE_REF", "main")
		_, _, ok := ciBaseRevision()
		assert.False(t, ok)
	})
}

// TestBaseRevisionResolvesCIMergeBase confirms baseRevision, when a GitLab
// merge-request pipeline is detected but only exposes the target branch's
// name (not its SHA), computes the actual merge base against that branch -
// not the branch's own moving tip - via vcs.Repository.MergeBase.
func TestBaseRevisionResolvesCIMergeBase(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	path := filepath.Join(dir, "a.go")
	require.NoError(t, os.WriteFile(path, []byte(docV1), 0o644))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	rootSig := sig()
	rootCommit, err := wt.Commit("root", &git.CommitOptions{Author: &rootSig})
	require.NoError(t, err)

	// Simulate the target branch's remote-tracking ref sitting right at the
	// root commit (as if it hadn't moved since this feature branch forked).
	require.NoError(t, repo.Storer.SetReference(
		plumbing.NewHashReference(plumbing.NewRemoteReferenceName("origin", "main"), rootCommit)))

	// A feature-branch commit on top of root - this is HEAD.
	require.NoError(t, os.WriteFile(path, []byte(docV2Full), 0o644))
	_, err = wt.Add(".")
	require.NoError(t, err)
	featureSig := sig()
	_, err = wt.Commit("feature work", &git.CommitOptions{Author: &featureSig})
	require.NoError(t, err)

	vr, err := vcs.Open(dir)
	require.NoError(t, err)

	t.Setenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME", "main")
	base, err := baseRevision(vr, "")
	require.NoError(t, err)
	assert.Equal(t, rootCommit, base.Hash)
}
