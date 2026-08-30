package build

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tfaller/docsweb/internal/model"
)

func TestRunFullIntegration(t *testing.T) {
	result, err := Run(Options{ConfigPath: "testdata/integration/.docsweb.yaml"})
	require.NoError(t, err)
	require.Len(t, result.Targets, 2)

	byKey := map[string]RenderedTarget{}
	for _, rt := range result.Targets {
		byKey[rt.Target.Key()] = rt
	}

	app, ok := byKey["integration.app"]
	require.True(t, ok)
	assert.Equal(t, "The App", app.Target.DisplayName)
	assert.Contains(t, app.DocHTML, `href="../lib/helper.html"`)
	assert.Contains(t, app.DocHTML, `id="top"`)

	// "lib" is a referenced scope with its own self-declared name, so its
	// targets are never re-prefixed by the root's own "integration" name.
	helper, ok := byKey["lib.helper"]
	require.True(t, ok)
	assert.Equal(t, "Helper Library", helper.Target.DisplayName)
	assert.Contains(t, helper.DocHTML, `id="usage"`)
	require.Len(t, helper.ChangelogHTML, 1)
	assert.Equal(t, []model.Audience{"dev"}, helper.ChangelogHTML[0].Audiences)

	// "integration.app" @uses "lib.helper" -> the reverse edge shows up on
	// helper's UsedBy, and app itself has no dependants.
	require.Len(t, helper.UsedBy, 1)
	assert.Equal(t, "integration.app", helper.UsedBy[0].User.Key())
	assert.Equal(t, model.Version{Major: 1, Minor: 0, Patch: 0}, helper.UsedBy[0].Use.Version)
	assert.Empty(t, app.UsedBy)

	require.Len(t, result.Issues, 1)
	assert.Equal(t, "integration.app", result.Issues[0].User.Key())
	assert.Equal(t, model.DiffMinor, result.Issues[0].Kind)
	assert.Equal(t, model.Version{Major: 1, Minor: 2, Patch: 0}, result.Issues[0].Current)
}

func TestRunErrorsOnMissingReferencedScopeConfig(t *testing.T) {
	_, err := Run(Options{ConfigPath: "testdata/scope_missing_config/.docsweb.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected referenced scope config")
}

func TestRunErrorsOnReferencedScopeNameMismatch(t *testing.T) {
	_, err := Run(Options{ConfigPath: "testdata/scope_name_mismatch/.docsweb.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `declares name "wrong"`)
	assert.Contains(t, err.Error(), `expected "sub"`)
}

// A referenced scope's config existing but declaring no name at all fails
// the same way an entirely missing config does (config.Load itself now
// requires name: on every .docsweb.yaml, root or referenced) - this fixture
// still exercises that the failure is clearly attributed to the referenced
// scope's own config path, not swallowed or misattributed to something else.
func TestRunErrorsOnReferencedScopeNameEmpty(t *testing.T) {
	_, err := Run(Options{ConfigPath: "testdata/scope_name_empty/.docsweb.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected referenced scope config")
	assert.Contains(t, err.Error(), "name is required")
}

func TestRunErrorsOnReferencedScopeNameCollidesWithRoot(t *testing.T) {
	_, err := Run(Options{ConfigPath: "testdata/scope_name_collision/.docsweb.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collides with the root scope's own name")
}

// TestRunFullIntegrationWithRemoteScope mirrors TestRunFullIntegration, but
// "lib" is a remote (git:) scope instead of a local, path-based one - a
// fresh git repository standing in for the referenced repository
// CloneOrFetch clones into the root scope's docsweb-cache directory. Proves
// rendering (@link resolution, UsedBy, outdated-use classification) and
// git-blame author attribution all work the same way across that boundary,
// with the author correctly attributed from the remote repo's own commit
// history rather than the root's.
func TestRunFullIntegrationWithRemoteScope(t *testing.T) {
	remoteDir := t.TempDir()
	repo, err := git.PlainInit(remoteDir, false)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(remoteDir, ".docsweb.yaml"), []byte("name: lib\n"), 0o644))
	helperSrc := "package lib\n\n/*\n    @docsweb\n    @define helper v1.2.0\n    @name Helper Library\n    @changelog\n    @audience dev\n    Added a new option.\n    @doc\n    Helper docs. [Anchor here](@anchor:usage)\n    @docsweb\n*/\n\nfunc Helper() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(remoteDir, "helper.go"), []byte(helperSrc), 0o644))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	bob := object.Signature{Name: "Bob", Email: "bob@example.com", When: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	_, err = wt.Commit("add helper", &git.CommitOptions{Author: &bob})
	require.NoError(t, err)

	rootDir := t.TempDir()
	rootCfg := "name: integration\naudience:\n    dev:\n    user:\nscope:\n    lib:\n        git: " + remoteDir + "\n        path: .\n"
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, ".docsweb.yaml"), []byte(rootCfg), 0o644))
	appSrc := "package integration\n\n/*\n    @docsweb\n    @define app v1.0.0\n    @name The App\n    @uses lib.helper@v1.0.0\n    @audience user\n    @doc\n    See [the helper](@link:lib.helper@v1.2.0) for details.\n\n    [Jump back to top](@anchor:top)\n    @docsweb\n*/\n\nfunc App() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "main.go"), []byte(appSrc), 0o644))

	result, err := Run(Options{ConfigPath: filepath.Join(rootDir, ".docsweb.yaml")})
	require.NoError(t, err)
	require.Len(t, result.Targets, 2)

	byKey := map[string]RenderedTarget{}
	for _, rt := range result.Targets {
		byKey[rt.Target.Key()] = rt
	}

	app, ok := byKey["integration.app"]
	require.True(t, ok)
	assert.Contains(t, app.DocHTML, `href="../lib/helper.html"`)

	helper, ok := byKey["lib.helper"]
	require.True(t, ok)
	assert.Contains(t, helper.DocHTML, `id="usage"`)
	// Blamed from the remote repo's own commit history, not the root's -
	// the root scope's own directory was never committed to any repo here.
	assert.Equal(t, "Bob <bob@example.com>", helper.Author)

	require.Len(t, helper.UsedBy, 1)
	assert.Equal(t, "integration.app", helper.UsedBy[0].User.Key())

	require.Len(t, result.Issues, 1)
	assert.Equal(t, model.DiffMinor, result.Issues[0].Kind)
}

func TestRunRemapsScopeAudiences(t *testing.T) {
	result, err := Run(Options{ConfigPath: "testdata/audience/.docsweb.yaml"})
	require.NoError(t, err)
	require.Len(t, result.Targets, 1)
	assert.Equal(t, []model.Audience{"dev", "tester"}, result.Targets[0].Target.Audiences)
}

func TestRunErrorsOnUnmappedScopeAudience(t *testing.T) {
	_, err := Run(Options{ConfigPath: "testdata/audience_bad/.docsweb.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmapped")
}

func TestRunErrorsOnUndeclaredRootAudience(t *testing.T) {
	_, err := Run(Options{ConfigPath: "testdata/audience_bad_root/.docsweb.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `audience "undeclared" is not declared`)
}

// testdata/ignore_bad is the same fixture as testdata/ignore, minus the
// "ignore:" rule, confirming skip/broken.go really would break the build if
// it weren't excluded.
func TestRunWithoutIgnoreErrorsOnBrokenFile(t *testing.T) {
	_, err := Run(Options{ConfigPath: "testdata/ignore_bad/.docsweb.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "doesNotExist")
}

func TestRunIgnoreExcludesMatchedDirectory(t *testing.T) {
	result, err := Run(Options{ConfigPath: "testdata/ignore/.docsweb.yaml"})
	require.NoError(t, err)
	require.Len(t, result.Targets, 1)
	assert.Equal(t, "ignoretest.kept", result.Targets[0].Target.Key())
}
