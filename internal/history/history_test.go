package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tfaller/docsweb/internal/config"
	"github.com/tfaller/docsweb/internal/vcs"
)

// testRepo builds up a sequence of real commits in a throwaway git
// repository, for exercising Walk against genuine history.
type testRepo struct {
	t   *testing.T
	dir string
	wt  *git.Worktree
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	return &testRepo{t: t, dir: dir, wt: wt}
}

func (r *testRepo) write(rel, content string) {
	r.t.Helper()
	path := filepath.Join(r.dir, rel)
	require.NoError(r.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(r.t, os.WriteFile(path, []byte(content), 0o644))
}

func (r *testRepo) remove(rel string) {
	r.t.Helper()
	require.NoError(r.t, os.Remove(filepath.Join(r.dir, rel)))
}

var commitSeq = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func (r *testRepo) commit(msg string) {
	r.t.Helper()
	require.NoError(r.t, r.wt.AddWithOptions(&git.AddOptions{All: true}))
	commitSeq = commitSeq.Add(time.Hour)
	sig := object.Signature{Name: "Author", Email: "author@example.com", When: commitSeq}
	_, err := r.wt.Commit(msg, &git.CommitOptions{Author: &sig, AllowEmptyCommits: true})
	require.NoError(r.t, err)
}

// open opens the repository and parses the root config at configRel
// (relative to r.dir) as it stands in the working tree right now - the
// "live" seed Walk expects.
func (r *testRepo) open(configRel string) (*vcs.Repository, string, *config.Config) {
	r.t.Helper()
	repo, err := vcs.Open(r.dir)
	require.NoError(r.t, err)
	configPath, cfg := r.liveConfig(configRel)
	return repo, configPath, cfg
}

// liveConfig parses the root config at configRel as it stands in the
// working tree right now, without opening the repository.
func (r *testRepo) liveConfig(configRel string) (string, *config.Config) {
	r.t.Helper()
	configPath := filepath.Join(r.dir, configRel)
	cfg, err := config.Load(configPath)
	require.NoError(r.t, err)
	return configPath, cfg
}

func TestWalkDiscoversEveryVersionAcrossHistory(t *testing.T) {
	repo := newTestRepo(t)
	repo.write(".docsweb.yaml", "name: root\n")
	repo.write("app.go", docBlockFor("app", "v1.0.0", "App"))
	repo.commit("v1.0.0")
	repo.write("app.go", docBlockFor("app", "v1.1.0", "App"))
	repo.commit("v1.1.0")
	repo.write("app.go", docBlockFor("app", "v1.2.0", "App"))
	repo.commit("v1.2.0")

	r, configPath, cfg := repo.open(".docsweb.yaml")
	found, err := Walk(r, configPath, cfg)
	require.NoError(t, err)

	versions := found["root.app"]
	require.Len(t, versions, 3)
	got := map[string]bool{}
	for _, v := range versions {
		got[v.Target.Version.String()] = true
		assert.Equal(t, "app.go", v.Path)
	}
	assert.Equal(t, map[string]bool{"v1.0.0": true, "v1.1.0": true, "v1.2.0": true}, got)
}

func TestWalkRootCommitCountsAsAdded(t *testing.T) {
	repo := newTestRepo(t)
	repo.write(".docsweb.yaml", "name: root\n")
	repo.write("app.go", docBlockFor("app", "v1.0.0", "App"))
	repo.commit("initial")

	r, configPath, cfg := repo.open(".docsweb.yaml")
	found, err := Walk(r, configPath, cfg)
	require.NoError(t, err)

	require.Len(t, found["root.app"], 1)
	assert.Equal(t, "v1.0.0", found["root.app"][0].Target.Version.String())
}

func TestWalkDuplicateVersionKeepsNewestCommitOnly(t *testing.T) {
	repo := newTestRepo(t)
	repo.write(".docsweb.yaml", "name: root\n")
	repo.write("app.go", docBlockFor("app", "v1.0.0", "App"))
	repo.commit("v1.0.0")
	repo.write("app.go", docBlockFor("app", "v2.0.0", "App"))
	repo.commit("v2.0.0")
	// A revert: v1.0.0 reappears verbatim, added again as a diff line.
	repo.write("app.go", docBlockFor("app", "v1.0.0", "App"))
	repo.commit("revert to v1.0.0")

	r, configPath, cfg := repo.open(".docsweb.yaml")
	found, err := Walk(r, configPath, cfg)
	require.NoError(t, err)

	versions := found["root.app"]
	counts := map[string]int{}
	for _, v := range versions {
		counts[v.Target.Version.String()]++
	}
	assert.Equal(t, 1, counts["v1.0.0"], "the first (newest) occurrence of a repeated version wins")
	assert.Equal(t, 1, counts["v2.0.0"])
}

func TestWalkAppliesConfigAsOfEachCommit(t *testing.T) {
	repo := newTestRepo(t)
	// Before any ignore rule existed, lib/app.go already defined a target.
	repo.write(".docsweb.yaml", "name: root\n")
	repo.write("lib/app.go", docBlockFor("app", "v1.0.0", "App"))
	repo.commit("v1.0.0, no ignore yet")

	// Now the root config starts ignoring lib/ entirely (e.g. it became
	// generated/vendored code) and the file is deleted.
	repo.write(".docsweb.yaml", "name: root\nignore:\n  - /lib/\n")
	repo.remove("lib/app.go")
	repo.commit("ignore lib/")

	r, configPath, cfg := repo.open(".docsweb.yaml")
	found, err := Walk(r, configPath, cfg)
	require.NoError(t, err)

	// The historic version from before the ignore rule existed must still
	// be found - Walk uses the config as it was at each commit, not today's.
	require.Len(t, found["root.app"], 1)
	assert.Equal(t, "v1.0.0", found["root.app"][0].Target.Version.String())
}

func TestWalkAttributesFilesToTheirDeclaredLocalScope(t *testing.T) {
	repo := newTestRepo(t)
	repo.write(".docsweb.yaml", "name: root\nscope:\n  lib:\n    path: lib\n")
	repo.write("lib/.docsweb.yaml", "name: lib\n")
	repo.write("lib/app.go", docBlockFor("app", "v1.0.0", "App"))
	repo.commit("initial")

	r, configPath, cfg := repo.open(".docsweb.yaml")
	found, err := Walk(r, configPath, cfg)
	require.NoError(t, err)

	versions := found["lib.app"]
	require.Len(t, versions, 1)
	assert.Equal(t, "lib.app", versions[0].Target.Key())
	assert.Equal(t, "lib", versions[0].Target.ConfigScope)
	assert.Equal(t, []string{"app.go"}, versions[0].Target.SourceFiles)
	assert.Equal(t, "lib/app.go", versions[0].Path)
}

func TestWalkAttributesToRootBeforeScopeWasDeclared(t *testing.T) {
	repo := newTestRepo(t)
	// lib/app.go already existed and defined a target before the root
	// config declared "lib" as its own referenced scope.
	repo.write(".docsweb.yaml", "name: root\n")
	repo.write("lib/app.go", docBlockFor("app", "v1.0.0", "App"))
	repo.commit("before scope declared")

	repo.write(".docsweb.yaml", "name: root\nscope:\n  lib:\n    path: lib\n")
	repo.write("lib/.docsweb.yaml", "name: lib\n")
	repo.write("lib/app.go", docBlockFor("app", "v1.1.0", "App"))
	repo.commit("declare lib scope, bump version")

	r, configPath, cfg := repo.open(".docsweb.yaml")
	found, err := Walk(r, configPath, cfg)
	require.NoError(t, err)

	// The newer version, discovered under today's config, belongs to "lib".
	require.Len(t, found["lib.app"], 1)
	assert.Equal(t, "v1.1.0", found["lib.app"][0].Target.Version.String())

	// The older version, discovered once Walk reloads the config as of the
	// commit before "lib" was declared, has no declared scope to belong to
	// and is attributed to the root scope instead.
	require.Len(t, found["root.app"], 1)
	assert.Equal(t, "v1.0.0", found["root.app"][0].Target.Version.String())
}

func TestWalkStopsAtCommitWithNoRootConfig(t *testing.T) {
	repo := newTestRepo(t)
	repo.write("app.go", "package doc\n// nothing to see here yet\n")
	repo.commit("before docsweb existed")

	repo.write(".docsweb.yaml", "name: root\n")
	repo.write("app.go", docBlockFor("app", "v1.0.0", "App"))
	repo.commit("adopt docsweb")

	r, configPath, cfg := repo.open(".docsweb.yaml")
	found, err := Walk(r, configPath, cfg)
	require.NoError(t, err)

	require.Len(t, found["root.app"], 1)
	assert.Equal(t, "v1.0.0", found["root.app"][0].Target.Version.String())
}

func TestWalkNoOnDiskRootReturnsNil(t *testing.T) {
	repo := newTestRepo(t)
	repo.write(".docsweb.yaml", "name: root\n")
	repo.write("app.go", docBlockFor("app", "v1.0.0", "App"))
	repo.commit("initial")

	configPath, cfg := repo.liveConfig(".docsweb.yaml")
	found, err := Walk(&vcs.Repository{}, configPath, cfg)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func docBlockFor(name, version, display string) string {
	return "package doc\n\n" +
		"// @docsweb\n" +
		"// @define " + name + " " + version + "\n" +
		"// @name " + display + "\n" +
		"// @docsweb\n"
}
