package build

import (
	"testing"

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
	assert.Contains(t, app.DocHTML, `href="lib/helper.html"`)
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

func TestRunRejectsRemoteScope(t *testing.T) {
	_, err := Run(Options{ConfigPath: "testdata/remote/.docsweb.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote scopes are not supported")
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
