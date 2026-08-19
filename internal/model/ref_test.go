package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTargetRef(t *testing.T) {
	r, err := ParseTargetRef("bla.bla.x@v1.0.0", "defaultScope")
	require.NoError(t, err)
	assert.Equal(t, TargetRef{Scope: "bla.bla", Name: "x", Version: Version{1, 0, 0}}, r)
	assert.Equal(t, "bla.bla.x@v1.0.0", r.String())
	assert.Equal(t, "bla.bla.x", r.Key())

	r2, err := ParseTargetRef("xxx@v2.1.0", "defaultScope")
	require.NoError(t, err)
	assert.Equal(t, TargetRef{Scope: "defaultScope", Name: "xxx", Version: Version{2, 1, 0}}, r2)
	assert.Equal(t, "defaultScope.xxx", r2.Key())
}

func TestParseTargetRefRootScope(t *testing.T) {
	r, err := ParseTargetRef("xxx@v2.1.0", "")
	require.NoError(t, err)
	assert.Equal(t, "", r.Scope)
	assert.Equal(t, "xxx", r.Key())
	assert.Equal(t, "xxx@v2.1.0", r.String())
}

func TestParseTargetRefInvalid(t *testing.T) {
	cases := []string{"", "noversion", "@v1.0.0", "bad-name@v1.0.0", "scope..name@v1.0.0", "x@notaversion"}
	for _, c := range cases {
		_, err := ParseTargetRef(c, "")
		assert.Errorf(t, err, "expected error for %q", c)
	}
}

func TestParseAudiences(t *testing.T) {
	a, err := ParseAudiences(" dev, tester ,user")
	require.NoError(t, err)
	assert.Equal(t, []Audience{"dev", "tester", "user"}, a)

	a2, err := ParseAudiences("")
	require.NoError(t, err)
	assert.Empty(t, a2)
}

func TestParseAudiencesInvalid(t *testing.T) {
	_, err := ParseAudiences("dev, bad name")
	assert.Error(t, err)
}
