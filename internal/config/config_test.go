package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tfaller/docsweb/internal/model"
)

func TestLoadReadmeExample(t *testing.T) {
	cfg, err := Load("testdata/docsweb.yaml")
	require.NoError(t, err)

	assert.Equal(t, "root", cfg.Name)
	require.Contains(t, cfg.Audiences, model.Audience("user"))
	require.Contains(t, cfg.Audiences, model.Audience("tester"))
	require.Contains(t, cfg.Audiences, model.Audience("dev"))
	require.Contains(t, cfg.Audiences, model.Audience("it"))
	assert.Empty(t, cfg.Audiences["user"].Combine)
	assert.ElementsMatch(t, []model.Audience{"dev", "tester"}, cfg.Audiences["it"].Combine)

	require.Contains(t, cfg.Scopes, "pathBased")
	pathBased := cfg.Scopes["pathBased"]
	assert.Equal(t, "relative/path/to/scope/root", pathBased.Path)
	assert.False(t, pathBased.Remote())

	require.Contains(t, cfg.Scopes, "remoteBased")
	remoteBased := cfg.Scopes["remoteBased"]
	assert.Equal(t, "repoUrl", remoteBased.Git)
	assert.Equal(t, "path/inside/the/repo", remoteBased.Path)
	assert.Equal(t, "branch", remoteBased.Ref)
	assert.True(t, remoteBased.Remote())

	require.Contains(t, cfg.Scopes, "parent.child")
	parentChild := cfg.Scopes["parent.child"]
	assert.Equal(t, "some/path", parentChild.Path)
	assert.Equal(t, model.Audience("dev"), parentChild.AudienceMap["devs"])
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("testdata/does-not-exist.yaml")
	assert.Error(t, err)
}

func TestAudienceIncludes(t *testing.T) {
	cfg, err := Load("testdata/docsweb.yaml")
	require.NoError(t, err)

	tests := []struct {
		group, member model.Audience
		want          bool
	}{
		{"it", "dev", true},
		{"it", "tester", true},
		{"it", "it", true},
		{"it", "user", false},
		{"dev", "dev", true},
		{"dev", "tester", false},
		{"dev", "it", false},
		{"unknown", "dev", false},
	}
	for _, tt := range tests {
		assert.Equalf(t, tt.want, cfg.AudienceIncludes(tt.group, tt.member),
			"AudienceIncludes(%q, %q)", tt.group, tt.member)
	}
}

func TestAudienceIncludesNestedCombine(t *testing.T) {
	cfg, err := Parse([]byte(`
name: s
audience:
    a:
    b:
        combine:
            - a
    c:
        combine:
            - b
scope:
    s:
        path: x
`))
	require.NoError(t, err)
	assert.True(t, cfg.AudienceIncludes("c", "a"), "combine should be transitive")
	assert.True(t, cfg.AudienceIncludes("c", "b"))
	assert.False(t, cfg.AudienceIncludes("a", "c"))
}

func TestAudienceIncludesCycleDoesNotHang(t *testing.T) {
	cfg, err := Parse([]byte(`
name: s
audience:
    a:
        combine:
            - b
    b:
        combine:
            - a
scope:
    s:
        path: x
`))
	require.NoError(t, err)
	assert.True(t, cfg.AudienceIncludes("a", "b"))
	assert.True(t, cfg.AudienceIncludes("a", "a"))
	assert.False(t, cfg.AudienceIncludes("a", "unknown"))
}

func TestResolveScopeAudience(t *testing.T) {
	cfg, err := Load("testdata/docsweb.yaml")
	require.NoError(t, err)

	// Explicit mapping via audienceMap.
	parent, ok := cfg.ResolveScopeAudience("parent.child", "devs")
	require.True(t, ok)
	assert.Equal(t, model.Audience("dev"), parent)

	// Auto-map: "dev" exists as-is in the root audience set, regardless of
	// whether the scope's audienceMap also mentions it.
	parent, ok = cfg.ResolveScopeAudience("parent.child", "dev")
	require.True(t, ok)
	assert.Equal(t, model.Audience("dev"), parent)

	// No auto-map and no explicit entry: unresolved.
	_, ok = cfg.ResolveScopeAudience("parent.child", "unknownChildAudience")
	assert.False(t, ok)

	// Unknown scope, unknown (non-audience) name: unresolved.
	_, ok = cfg.ResolveScopeAudience("noSuchScope", "unknownChildAudience")
	assert.False(t, ok)
}

func TestParseDuplicateAudienceIsError(t *testing.T) {
	_, err := Parse([]byte(`
name: s
audience:
    dev:
    dev:
scope:
    s:
        path: x
`))
	assert.Error(t, err)
}

func TestParseDuplicateScopeIsError(t *testing.T) {
	_, err := Parse([]byte(`
name: root
audience:
    dev:
scope:
    s:
        path: x
    s:
        path: y
`))
	assert.Error(t, err)
}

func TestParseUnknownCombineReferenceIsError(t *testing.T) {
	_, err := Parse([]byte(`
name: root
audience:
    it:
        combine:
            - dev
scope:
    s:
        path: x
`))
	assert.ErrorContains(t, err, "unknown audience")
}

func TestParseUnknownAudienceMapParentIsError(t *testing.T) {
	_, err := Parse([]byte(`
name: root
audience:
    dev:
scope:
    s:
        path: x
        audienceMap:
            devs: nosuchparent
`))
	assert.ErrorContains(t, err, "unknown audience")
}

func TestParseInvalidAudienceNameIsError(t *testing.T) {
	_, err := Parse([]byte(`
name: root
audience:
    "not valid!":
scope:
    s:
        path: x
`))
	assert.Error(t, err)
}

func TestParseName(t *testing.T) {
	cfg, err := Parse([]byte(`
name: com.company.project
`))
	require.NoError(t, err)
	assert.Equal(t, "com.company.project", cfg.Name)
}

func TestParseMissingNameIsError(t *testing.T) {
	_, err := Parse([]byte(`
audience:
    dev:
`))
	assert.ErrorContains(t, err, "name is required")
}

func TestParseInvalidNameIsError(t *testing.T) {
	_, err := Parse([]byte(`
name: "not-valid"
`))
	assert.Error(t, err)
}

func TestParseInvalidScopeNameSegmentIsError(t *testing.T) {
	_, err := Parse([]byte(`
name: root
audience:
    dev:
scope:
    "parent.not-valid":
        path: x
`))
	assert.Error(t, err)
}

func TestParseIgnoreList(t *testing.T) {
	cfg, err := Parse([]byte(`
name: root
audience:
    dev:
scope:
    s:
        path: x
ignore:
    - testdata/
    - "*.tmp"
`))
	require.NoError(t, err)
	assert.Equal(t, []string{"testdata/", "*.tmp"}, cfg.Ignore)
}

func TestParseIgnoreDefaultsToEmpty(t *testing.T) {
	cfg, err := Parse([]byte(`
name: root
audience:
    dev:
scope:
    s:
        path: x
`))
	require.NoError(t, err)
	assert.Empty(t, cfg.Ignore)
}

func TestParseScopeWithoutPathOrGitIsError(t *testing.T) {
	_, err := Parse([]byte(`
name: root
audience:
    dev:
scope:
    s:
        ref: branch
`))
	assert.ErrorContains(t, err, "must specify path or git")
}
