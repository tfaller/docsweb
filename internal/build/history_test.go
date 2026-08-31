package build_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tfaller/docsweb/internal/build"
)

// historyRepo builds up a sequence of real commits, for exercising Run's
// wiring of internal/history end to end.
type historyRepo struct {
	t   *testing.T
	dir string
	wt  *git.Worktree
}

func newHistoryRepo(t *testing.T) *historyRepo {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	return &historyRepo{t: t, dir: dir, wt: wt}
}

func (r *historyRepo) write(rel, content string) {
	r.t.Helper()
	path := filepath.Join(r.dir, rel)
	require.NoError(r.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(r.t, os.WriteFile(path, []byte(content), 0o644))
}

var historyCommitSeq = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func (r *historyRepo) commit() {
	r.t.Helper()
	require.NoError(r.t, r.wt.AddWithOptions(&git.AddOptions{All: true}))
	historyCommitSeq = historyCommitSeq.Add(time.Hour)
	sig := object.Signature{Name: "Alice", Email: "alice@example.com", When: historyCommitSeq}
	_, err := r.wt.Commit("commit", &git.CommitOptions{Author: &sig, AllowEmptyCommits: true})
	require.NoError(r.t, err)
}

// TestRunDiscoversAndRendersHistoricVersions exercises the full pipeline
// against a real multi-commit repository: a target's own past versions are
// discovered, get their own rendered pages, and - the key behavior a
// version-unaware build could never get right - an old version's own @uses
// reference resolves to the exact historic version it named, not always
// whatever the referenced target's current version happens to be.
func TestRunDiscoversAndRendersHistoricVersions(t *testing.T) {
	repo := newHistoryRepo(t)
	repo.write(".docsweb.yaml", "name: proj\n")

	repo.write("helper.go", "package proj\n\n"+
		"// @docsweb\n"+
		"// @define helper v1.0.0\n"+
		"// @name Helper\n"+
		"// @docsweb\n")
	repo.write("main.go", "package proj\n\n"+
		"// @docsweb\n"+
		"// @define app v1.0.0\n"+
		"// @name App\n"+
		"// @uses helper@v1.0.0\n"+
		"// @docsweb\n")
	repo.commit()

	repo.write("helper.go", "package proj\n\n"+
		"// @docsweb\n"+
		"// @define helper v2.0.0\n"+
		"// @name Helper\n"+
		"// @docsweb\n")
	repo.commit()

	repo.write("main.go", "package proj\n\n"+
		"// @docsweb\n"+
		"// @define app v1.1.0\n"+
		"// @name App\n"+
		"// @uses helper@v2.0.0\n"+
		"// @changelog\n"+
		"// Bumped to match helper's new major version.\n"+
		"// @docsweb\n")
	repo.commit()

	result, err := build.Run(build.Options{ConfigPath: filepath.Join(repo.dir, ".docsweb.yaml")})
	require.NoError(t, err)

	byKey := map[string]*build.RenderedTarget{}
	for i := range result.Targets {
		byKey[result.Targets[i].Target.Key()] = &result.Targets[i]
	}

	app := byKey["proj.app"]
	require.NotNil(t, app)
	require.Len(t, app.History, 1, "app's v1.0.0 should be discovered as a past version")
	assert.Equal(t, "v1.0.0", app.History[0].Target.Version.String())
	require.Len(t, app.Versions, 2)
	assert.True(t, app.Versions[0].Current)
	assert.Equal(t, "v1.1.0", app.Versions[0].Version.String())
	assert.Equal(t, "v1.0.0", app.Versions[1].Version.String())
	assert.Equal(t, "proj/app/v1.0.0.html", app.Versions[1].URL)

	// Current version (v1.1.0, the 3rd commit) and its historic v1.0.0 (the
	// 1st commit) are each attributed to their own distinct introducing
	// commit - not to the same (e.g. HEAD's) commit.
	assert.Len(t, app.CommitHash, 7)
	assert.NotEqual(t, app.CommitHash, app.History[0].CommitHash)
	assert.Equal(t, app.CommitHash, app.Versions[0].CommitHash)
	assert.Equal(t, app.History[0].CommitHash, app.Versions[1].CommitHash)
	wantCurrent := historyCommitSeq // set by the 3rd (most recent) repo.commit() call above
	assert.WithinDuration(t, wantCurrent, app.CommitTime, 0)
	wantHistoric := wantCurrent.Add(-2 * time.Hour) // the 1st repo.commit() call
	assert.WithinDuration(t, wantHistoric, app.History[0].CommitTime, 0)

	helper := byKey["proj.helper"]
	require.NotNil(t, helper)
	require.Len(t, helper.History, 1, "helper's v1.0.0 should be discovered as a past version")
	assert.Equal(t, "v1.0.0", helper.History[0].Target.Version.String())

	// The key assertion: app's *historic* v1.0.0 revision referenced
	// helper@v1.0.0 - it must resolve to helper's own historic v1.0.0 page,
	// not helper's current v2.0.0 page.
	require.Len(t, app.History[0].Uses, 1)
	assert.True(t, app.History[0].Uses[0].Found)
	assert.Equal(t, "../../proj/helper/v1.0.0.html", app.History[0].Uses[0].URL)

	// app's *current* v1.1.0 revision references helper@v2.0.0, which is
	// exactly helper's current version - resolves straight to its page.
	require.Len(t, app.Uses, 1)
	assert.True(t, app.Uses[0].Found)
	assert.Equal(t, "../proj/helper.html", app.Uses[0].URL)
}

// TestRunNoHistoryOutsideGitRepository confirms build.Run still succeeds,
// with every target's Versions holding only its current entry and no
// History, when the root scope isn't inside a git repository at all.
func TestRunNoHistoryOutsideGitRepository(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".docsweb.yaml"), []byte("name: standalone\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(
		"package standalone\n\n"+
			"// @docsweb\n"+
			"// @define app v1.0.0\n"+
			"// @docsweb\n"), 0o644))

	result, err := build.Run(build.Options{ConfigPath: filepath.Join(dir, ".docsweb.yaml")})
	require.NoError(t, err)
	require.Len(t, result.Targets, 1)
	assert.Empty(t, result.Targets[0].History)
	require.Len(t, result.Targets[0].Versions, 1)
	assert.True(t, result.Targets[0].Versions[0].Current)
	assert.Empty(t, result.Targets[0].CommitHash)
	assert.True(t, result.Targets[0].CommitTime.IsZero())
	assert.Empty(t, result.Targets[0].Versions[0].CommitHash)
	assert.True(t, result.Targets[0].Versions[0].CommitTime.IsZero())
}
