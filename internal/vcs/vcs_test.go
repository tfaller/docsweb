package vcs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initRepo creates a fresh git repository at dir with a single file
// containing lines, committed with the given author, and returns the
// absolute path to that file.
func initRepo(t *testing.T, dir string, lines []string, author object.Signature) string {
	t.Helper()

	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	path := filepath.Join(dir, "doc.go")
	writeLines(t, path, lines)

	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("doc.go")
	require.NoError(t, err)
	_, err = wt.Commit("initial", &git.CommitOptions{Author: &author})
	require.NoError(t, err)

	return path
}

func writeLines(t *testing.T, path string, lines []string) {
	t.Helper()
	content := ""
	for i, l := range lines {
		if i > 0 {
			content += "\n"
		}
		content += l
	}
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestOpenNotARepository(t *testing.T) {
	_, err := Open(t.TempDir())
	assert.ErrorIs(t, err, ErrNoRepository)
}

func TestOpenSubdirectoryFindsRepoRoot(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc", "", "// @define foo v1.0.0"}, alice)

	sub := filepath.Join(dir, "nested", "deeper")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	repo, err := Open(sub)
	require.NoError(t, err)
	assert.NotNil(t, repo)
}

func TestBlameAuthorMatchesByLineNumber(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{
		"package doc",
		"",
		"// @define foo v1.0.0",
	}, alice)

	repo, err := Open(dir)
	require.NoError(t, err)

	author, ok, err := repo.BlameAuthor("doc.go", 3, "// @define foo v1.0.0")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Author{Name: "Alice", Email: "alice@example.com"}, author)
}

func TestBlameAuthorMatchesSubstringNotFullLine(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	// A caller shouldn't have to reproduce the line's exact text (comment
	// markers, indentation, trailing content) - just a substring built from
	// structured data it already has in memory (e.g. a target's name and
	// version) is enough.
	initRepo(t, dir, []string{
		"package doc",
		"",
		"    // @define foo v1.0.0 (current)",
	}, alice)

	repo, err := Open(dir)
	require.NoError(t, err)

	author, ok, err := repo.BlameAuthor("doc.go", 3, "@define foo v1.0.0")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Author{Name: "Alice", Email: "alice@example.com"}, author)
}

func TestBlameAuthorFallsBackToContentWhenLineNumberDrifted(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	path := initRepo(t, dir, []string{
		"package doc",
		"",
		"// @define foo v1.0.0",
	}, alice)

	repo, err := Open(dir)
	require.NoError(t, err)

	// Simulate the working tree having drifted from HEAD (e.g. an
	// unrelated, uncommitted edit shifted every following line down by
	// one) - the caller's line-number hint (4) no longer matches where
	// the @define line actually sits in the committed blob (3), but the
	// substring is unchanged and should still be found.
	writeLines(t, path, []string{
		"package doc",
		"// unrelated uncommitted edit",
		"",
		"// @define foo v1.0.0",
	})

	author, ok, err := repo.BlameAuthor("doc.go", 4, "// @define foo v1.0.0")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Author{Name: "Alice", Email: "alice@example.com"}, author)
}

func TestBlameAuthorReflectsLastCommitToTouchTheLine(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	path := initRepo(t, dir, []string{
		"package doc",
		"// @define foo v1.0.0",
	}, alice)

	repo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	writeLines(t, path, []string{
		"package doc",
		"// @define foo v2.0.0",
	})
	_, err = wt.Add("doc.go")
	require.NoError(t, err)
	bob := object.Signature{Name: "Bob", Email: "bob@example.com", When: fixedTime().Add(time.Hour)}
	_, err = wt.Commit("bump version", &git.CommitOptions{Author: &bob})
	require.NoError(t, err)

	vr, err := Open(dir)
	require.NoError(t, err)

	author, ok, err := vr.BlameAuthor("doc.go", 2, "// @define foo v2.0.0")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Author{Name: "Bob", Email: "bob@example.com"}, author)
}

func TestBlameAuthorUntrackedFileReturnsNotOK(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc"}, alice)

	untracked := filepath.Join(dir, "untracked.go")
	require.NoError(t, os.WriteFile(untracked, []byte("package doc\n// @define bar v1.0.0"), 0o644))

	repo, err := Open(dir)
	require.NoError(t, err)

	author, ok, err := repo.BlameAuthor("untracked.go", 2, "// @define bar v1.0.0")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, Author{}, author)
}

func TestBlameAuthorNoMatchingLineReturnsNotOK(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc", "// @define foo v1.0.0"}, alice)

	repo, err := Open(dir)
	require.NoError(t, err)

	author, ok, err := repo.BlameAuthor("doc.go", 2, "// this text was never committed")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, Author{}, author)
}

func TestAuthorString(t *testing.T) {
	assert.Equal(t, "Alice <alice@example.com>", Author{Name: "Alice", Email: "alice@example.com"}.String())
	assert.Equal(t, "Alice", Author{Name: "Alice"}.String())
	assert.Equal(t, "alice@example.com", Author{Email: "alice@example.com"}.String())
	assert.Equal(t, "", Author{}.String())
}

func fixedTime() time.Time {
	return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
}

func TestCommitResolvesHEAD(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc"}, alice)

	repo, err := Open(dir)
	require.NoError(t, err)

	head, err := repo.Commit("HEAD")
	require.NoError(t, err)
	assert.Equal(t, repo.commit.Hash, head.Hash)
}

func TestCommitInvalidRevisionIsError(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc"}, alice)

	repo, err := Open(dir)
	require.NoError(t, err)

	_, err = repo.Commit("does-not-exist")
	assert.Error(t, err)
}

func TestMergeBase(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	path := initRepo(t, dir, []string{"package doc"}, alice)

	gitRepo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := gitRepo.Worktree()
	require.NoError(t, err)

	root, err := gitRepo.Head()
	require.NoError(t, err)

	// A branch that stays at root - simulating a target branch that hasn't
	// moved since a feature branch forked off it.
	require.NoError(t, gitRepo.Storer.SetReference(
		plumbing.NewHashReference(plumbing.NewBranchReferenceName("target"), root.Hash())))

	writeLines(t, path, []string{"package doc", "// feature work"})
	_, err = wt.Add("doc.go")
	require.NoError(t, err)
	bob := object.Signature{Name: "Bob", Email: "bob@example.com", When: fixedTime().Add(time.Hour)}
	_, err = wt.Commit("feature work", &git.CommitOptions{Author: &bob})
	require.NoError(t, err)

	repo, err := Open(dir)
	require.NoError(t, err)

	base, err := repo.MergeBase("HEAD", "target")
	require.NoError(t, err)
	assert.Equal(t, root.Hash(), base.Hash)
}

func TestMergeBaseInvalidRevisionIsError(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc"}, alice)

	repo, err := Open(dir)
	require.NoError(t, err)

	_, err = repo.MergeBase("HEAD", "does-not-exist")
	assert.Error(t, err)
}

func TestFileContentsAtCommit(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc", "// v1"}, alice)

	repo, err := Open(dir)
	require.NoError(t, err)

	content, ok, err := repo.FileContents(repo.commit, "doc.go")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "package doc\n// v1", content)
}

func TestFileContentsMissingFileIsNotOK(t *testing.T) {
	dir := t.TempDir()
	alice := object.Signature{Name: "Alice", Email: "alice@example.com", When: fixedTime()}
	initRepo(t, dir, []string{"package doc"}, alice)

	repo, err := Open(dir)
	require.NoError(t, err)

	content, ok, err := repo.FileContents(repo.commit, "never-committed.go")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, content)
}
