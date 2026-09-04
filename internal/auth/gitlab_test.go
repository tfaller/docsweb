package auth

import (
	"testing"

	"github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitLabCIAppliesForMatchingHostWhenTokenSet(t *testing.T) {
	t.Setenv("CI_JOB_TOKEN", "tok123")

	auth, ok, err := GitLabCI{}.HTTPAuth("https://gitlab.com/group/project.git")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, &http.BasicAuth{Username: "gitlab-ci-token", Password: "tok123"}, auth)
}

func TestGitLabCINeverAppliesWithoutToken(t *testing.T) {
	t.Setenv("CI_JOB_TOKEN", "")

	_, ok, err := GitLabCI{}.HTTPAuth("https://gitlab.com/group/project.git")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestGitLabCINeverAppliesToOtherHosts(t *testing.T) {
	t.Setenv("CI_JOB_TOKEN", "tok123")

	_, ok, err := GitLabCI{}.HTTPAuth("https://github.com/group/project.git")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestGitLabCINeverAppliesToNonHTTPSchemes(t *testing.T) {
	t.Setenv("CI_JOB_TOKEN", "tok123")

	for _, url := range []string{
		"ssh://git@gitlab.com/group/project.git",
		"git@gitlab.com:group/project.git",
	} {
		_, ok, err := GitLabCI{}.HTTPAuth(url)
		require.NoError(t, err)
		assert.False(t, ok, "url %q must never receive gitlab.com credentials", url)
	}
}

func TestGitLabCIHostOverride(t *testing.T) {
	t.Setenv("CI_JOB_TOKEN", "tok123")

	provider := GitLabCI{Host: "gitlab.example.com"}

	auth, ok, err := provider.HTTPAuth("https://gitlab.example.com/group/project.git")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, &http.BasicAuth{Username: "gitlab-ci-token", Password: "tok123"}, auth)

	// The default host no longer matches once Host is overridden.
	_, ok, err = provider.HTTPAuth("https://gitlab.com/group/project.git")
	require.NoError(t, err)
	assert.False(t, ok)
}
