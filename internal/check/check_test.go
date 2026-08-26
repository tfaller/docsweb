package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPopulatesResultWithoutRendering(t *testing.T) {
	result, err := Run(Options{ConfigPath: "testdata/links_good/.docsweb.yaml"})
	require.NoError(t, err)

	require.NotNil(t, result.Config)
	assert.Equal(t, "linksgood", result.Config.Name)
	assert.NotEmpty(t, result.RootDir)
	assert.Contains(t, result.ScopeRoots, "linksgood")

	require.Len(t, result.Registry.Targets(), 2)
	assert.Contains(t, result.Anchors["linksgood.helper"], "usage")

	// "widget" @uses nothing, so no outdated-use issues; UsedBy has no
	// entry either since nothing declares a @uses at all in this fixture.
	assert.Empty(t, result.Issues)
	assert.Empty(t, result.UsedBy)
}

func TestRunErrorsAreNamespacedByCheck(t *testing.T) {
	_, err := Run(Options{ConfigPath: "testdata/links_bad/.docsweb.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check links:")
}

// TestRunAndRunForBuildAgreeOnSharedChecks confirms Run (CheckOnly, "docsweb
// check") and RunForBuild (BuildOnly, "docsweb build") produce the same
// pass/fail outcome for every check currently tagged Both - the phase split
// only matters once a future check is scoped to just one command.
func TestRunAndRunForBuildAgreeOnSharedChecks(t *testing.T) {
	_, errCheck := Run(Options{ConfigPath: "testdata/links_good/.docsweb.yaml"})
	_, errBuild := RunForBuild(Options{ConfigPath: "testdata/links_good/.docsweb.yaml"})
	assert.NoError(t, errCheck)
	assert.NoError(t, errBuild)

	_, errCheck = Run(Options{ConfigPath: "testdata/links_bad/.docsweb.yaml"})
	_, errBuild = RunForBuild(Options{ConfigPath: "testdata/links_bad/.docsweb.yaml"})
	assert.Error(t, errCheck)
	assert.Error(t, errBuild)
}

func TestRunMissingConfig(t *testing.T) {
	_, err := Run(Options{ConfigPath: "testdata/does-not-exist/.docsweb.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check scopes:")
}
