package build

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/model"
)

func TestResolveUsesClassifiesOutdated(t *testing.T) {
	reg := collect.NewRegistry()
	require.NoError(t, reg.AddScope(collect.Options{Root: "testdata/resolve"}))

	issues, err := ResolveUses(reg)
	require.NoError(t, err)
	require.Len(t, issues, 2)

	byUser := map[string]UsageIssue{}
	for _, i := range issues {
		byUser[i.User.Key()] = i
	}

	major, ok := byUser["consumerMajor"]
	require.True(t, ok)
	assert.Equal(t, model.DiffMajor, major.Kind)
	assert.Equal(t, model.Version{Major: 2, Minor: 1, Patch: 5}, major.Current)

	minor, ok := byUser["consumerMinor"]
	require.True(t, ok)
	assert.Equal(t, model.DiffMinor, minor.Kind)

	_, hasPatch := byUser["consumerPatch"]
	assert.False(t, hasPatch, "patch-only differences must be ignored")
	_, hasCurrent := byUser["consumerCurrent"]
	assert.False(t, hasCurrent, "up-to-date uses must not be reported")
}

func TestResolveUsesErrorsOnMissingTarget(t *testing.T) {
	reg := collect.NewRegistry()
	require.NoError(t, reg.AddScope(collect.Options{Root: "testdata/missing"}))

	_, err := ResolveUses(reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "doesNotExist")
}

func TestComputeUsedByInvertsUses(t *testing.T) {
	reg := collect.NewRegistry()
	require.NoError(t, reg.AddScope(collect.Options{Root: "testdata/resolve"}))

	usedBy := ComputeUsedBy(reg)

	// All four consumers reference "producer", just at different versions.
	entries := usedBy["producer"]
	require.Len(t, entries, 4)

	byUser := map[string]UsedByRef{}
	for _, e := range entries {
		byUser[e.User.Key()] = e
	}

	major, ok := byUser["consumerMajor"]
	require.True(t, ok)
	assert.Equal(t, model.Version{Major: 1, Minor: 0, Patch: 0}, major.Use.Version)

	// Sorted deterministically by dependant key.
	assert.Equal(t, "consumerCurrent", entries[0].User.Key())
	assert.Equal(t, "consumerMajor", entries[1].User.Key())
	assert.Equal(t, "consumerMinor", entries[2].User.Key())
	assert.Equal(t, "consumerPatch", entries[3].User.Key())

	// A target nobody depends on has no entry at all.
	assert.Empty(t, usedBy["consumerMajor"])
}
