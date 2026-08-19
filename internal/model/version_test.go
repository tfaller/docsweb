package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	v, err := ParseVersion("v1.0.1")
	require.NoError(t, err)
	assert.Equal(t, Version{1, 0, 1}, v)
	assert.Equal(t, "v1.0.1", v.String())

	v2, err := ParseVersion("2.10.3")
	require.NoError(t, err)
	assert.Equal(t, Version{2, 10, 3}, v2)
}

func TestParseVersionInvalid(t *testing.T) {
	cases := []string{"", "v1.0", "v1.0.0.0", "v1.x.0", "1.0.-1", "v.1.0"}
	for _, c := range cases {
		_, err := ParseVersion(c)
		assert.Errorf(t, err, "expected error for %q", c)
	}
}

func TestVersionCompare(t *testing.T) {
	a := Version{1, 2, 3}
	assert.Equal(t, 0, a.Compare(Version{1, 2, 3}))
	assert.Equal(t, -1, a.Compare(Version{2, 0, 0}))
	assert.Equal(t, 1, a.Compare(Version{1, 1, 9}))
	assert.Equal(t, -1, a.Compare(Version{1, 2, 4}))
	assert.True(t, a.Equal(Version{1, 2, 3}))
}

func TestDiff(t *testing.T) {
	assert.Equal(t, DiffNone, Diff(Version{1, 0, 0}, Version{1, 0, 0}))
	assert.Equal(t, DiffPatch, Diff(Version{1, 0, 0}, Version{1, 0, 1}))
	assert.Equal(t, DiffMinor, Diff(Version{1, 0, 5}, Version{1, 1, 0}))
	assert.Equal(t, DiffMajor, Diff(Version{1, 9, 9}, Version{2, 0, 0}))
}
