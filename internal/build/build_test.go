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

	app, ok := byKey["app"]
	require.True(t, ok)
	assert.Equal(t, "The App", app.Target.DisplayName)
	assert.Contains(t, app.DocHTML, `href="lib/helper.html"`)
	assert.Contains(t, app.DocHTML, `id="top"`)

	helper, ok := byKey["lib.helper"]
	require.True(t, ok)
	assert.Equal(t, "Helper Library", helper.Target.DisplayName)
	assert.Contains(t, helper.DocHTML, `id="usage"`)
	require.Len(t, helper.ChangelogHTML, 1)
	assert.Equal(t, []model.Audience{"dev"}, helper.ChangelogHTML[0].Audiences)

	require.Len(t, result.Issues, 1)
	assert.Equal(t, "app", result.Issues[0].User.Key())
	assert.Equal(t, model.DiffMinor, result.Issues[0].Kind)
	assert.Equal(t, model.Version{Major: 1, Minor: 2, Patch: 0}, result.Issues[0].Current)
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
	assert.Equal(t, "kept", result.Targets[0].Target.Key())
}
