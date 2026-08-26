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

func TestParseQualifiedName(t *testing.T) {
	scope, name, err := ParseQualifiedName("bla.bla.x")
	require.NoError(t, err)
	assert.Equal(t, "bla.bla", scope)
	assert.Equal(t, "x", name)

	scope2, name2, err := ParseQualifiedName("x")
	require.NoError(t, err)
	assert.Equal(t, "", scope2)
	assert.Equal(t, "x", name2)
}

func TestParseQualifiedNameInvalid(t *testing.T) {
	cases := []string{"", "bad-name", "scope..name"}
	for _, c := range cases {
		_, _, err := ParseQualifiedName(c)
		assert.Errorf(t, err, "expected error for %q", c)
	}
}

func TestParseDefineNameBare(t *testing.T) {
	scope, name, err := ParseDefineName("login", "docsweb")
	require.NoError(t, err)
	assert.Equal(t, "docsweb", scope)
	assert.Equal(t, "login", name)

	scope2, name2, err := ParseDefineName("login", "")
	require.NoError(t, err)
	assert.Equal(t, "", scope2)
	assert.Equal(t, "login", name2)
}

func TestParseDefineNameLeadingDot(t *testing.T) {
	scope, name, err := ParseDefineName(".auth.login", "docsweb")
	require.NoError(t, err)
	assert.Equal(t, "docsweb.auth", scope)
	assert.Equal(t, "login", name)

	scope2, name2, err := ParseDefineName(".auth.login", "")
	require.NoError(t, err)
	assert.Equal(t, "auth", scope2)
	assert.Equal(t, "login", name2)
}

func TestParseDefineNameAbsolute(t *testing.T) {
	scope, name, err := ParseDefineName("docsweb.auth.login", "docsweb")
	require.NoError(t, err)
	assert.Equal(t, "docsweb.auth", scope)
	assert.Equal(t, "login", name)

	scope2, name2, err := ParseDefineName("docsweb.login", "docsweb")
	require.NoError(t, err)
	assert.Equal(t, "docsweb", scope2)
	assert.Equal(t, "login", name2)
}

func TestParseDefineNameAbsoluteUnconstrainedUnderEmptyConfigScope(t *testing.T) {
	scope, name, err := ParseDefineName("other.login", "")
	require.NoError(t, err)
	assert.Equal(t, "other", scope)
	assert.Equal(t, "login", name)
}

func TestParseDefineNameAbsoluteMismatchIsError(t *testing.T) {
	_, _, err := ParseDefineName("other.login", "docsweb")
	assert.Error(t, err)
}

func TestParseDefineNameInvalidSegmentIsError(t *testing.T) {
	_, _, err := ParseDefineName("bad-name", "docsweb")
	assert.Error(t, err)
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
