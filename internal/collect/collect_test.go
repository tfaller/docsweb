package collect

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tfaller/docsweb/internal/ignore"
	"github.com/tfaller/docsweb/internal/model"
)

func TestAddScopeCollectsTargets(t *testing.T) {
	r := NewRegistry()
	err := r.AddScope(Options{Scope: "", Root: "testdata/simple"})
	require.NoError(t, err)

	alpha, ok := r.Get("alpha")
	require.True(t, ok)
	assert.Equal(t, model.Version{Major: 1, Minor: 0, Patch: 0}, alpha.Version)
	assert.Equal(t, "Alpha Target", alpha.DisplayName)
	assert.Equal(t, []model.Audience{"dev", "user"}, alpha.Audiences)
	require.Len(t, alpha.Uses, 1)
	assert.Equal(t, "beta", alpha.Uses[0].Name)
	require.Len(t, alpha.Changelog, 1)
	assert.Equal(t, "Initial release.", alpha.Changelog[0].Body)

	beta, ok := r.Get("beta")
	require.True(t, ok)
	assert.Equal(t, "Beta Target", beta.DisplayName)

	assert.Len(t, r.Targets(), 2)
}

func TestAddScopeDuplicateTargetErrors(t *testing.T) {
	r := NewRegistry()
	err := r.AddScope(Options{Scope: "", Root: "testdata/dup"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already defined")
}

func TestAddScopeAssignsScopeName(t *testing.T) {
	r := NewRegistry()
	err := r.AddScope(Options{Scope: "myscope", Root: "testdata/simple"})
	require.NoError(t, err)

	tgt, ok := r.Get("myscope.alpha")
	require.True(t, ok)
	assert.Equal(t, "myscope", tgt.Scope)
	// @uses without an explicit scope prefix resolves against the target's
	// own scope.
	assert.Equal(t, "myscope", tgt.Uses[0].Scope)
}

func TestAddScopeExcludesSubdirectory(t *testing.T) {
	r := NewRegistry()
	err := r.AddScope(Options{Scope: "", Root: "testdata/simple", Exclude: []string{"sub"}})
	require.NoError(t, err)

	_, ok := r.Get("beta")
	assert.False(t, ok)
	_, ok = r.Get("alpha")
	assert.True(t, ok)
}

// testdata/ignoretest/skip/b.go redefines the same target as
// testdata/ignoretest/a.go, so without an ignore rule this scope fails with
// a duplicate-target error.
func TestAddScopeWithoutIgnoreErrorsOnDuplicate(t *testing.T) {
	r := NewRegistry()
	err := r.AddScope(Options{Scope: "", Root: "testdata/ignoretest"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already defined")
}

func TestAddScopeIgnoreSkipsMatchedDirectory(t *testing.T) {
	r := NewRegistry()
	err := r.AddScope(Options{
		Scope: "", Root: "testdata/ignoretest",
		Ignore: ignore.Compile([]string{"skip/"}),
	})
	require.NoError(t, err)

	_, ok := r.Get("kept")
	assert.True(t, ok)
	assert.Len(t, r.Targets(), 1)
}

func TestAddScopeIgnoreBaseIsRelativeToGivenDir(t *testing.T) {
	r := NewRegistry()
	err := r.AddScope(Options{
		Scope: "", Root: "testdata/ignoretest",
		Ignore:     ignore.Compile([]string{"ignoretest/skip/"}),
		IgnoreBase: "testdata",
	})
	require.NoError(t, err)

	_, ok := r.Get("kept")
	assert.True(t, ok)
	assert.Len(t, r.Targets(), 1)
}
