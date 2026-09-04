package auth

import (
	"errors"
	"net/http"
	"testing"

	"github.com/go-git/go-git/v6/plumbing/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAuth is a distinguishable client.HTTPAuth implementation so tests can
// tell which stubProvider's credentials a Registry actually picked.
type fakeAuth struct{ name string }

func (f *fakeAuth) Authorizer(*http.Request) error { return nil }

// stubProvider answers ok only for a fixed repoURL, letting tests build a
// Registry out of Providers with known, controllable behavior instead of
// depending on GitLabCI's own environment/host matching.
type stubProvider struct {
	matchURL string
	auth     client.HTTPAuth
	err      error
}

func (s stubProvider) HTTPAuth(repoURL string) (client.HTTPAuth, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	if repoURL != s.matchURL {
		return nil, false, nil
	}
	return s.auth, true, nil
}

func TestRegistryCredentialsFirstMatchingProviderWins(t *testing.T) {
	url := "https://example.com/repo.git"
	first := stubProvider{matchURL: url, auth: &fakeAuth{name: "first"}}
	second := stubProvider{matchURL: url, auth: &fakeAuth{name: "second"}}

	reg := NewRegistry(first, second)

	got, err := reg.Credentials(url)
	require.NoError(t, err)
	assert.Equal(t, first.auth, got)
}

func TestRegistryCredentialsSkipsNonMatchingProviders(t *testing.T) {
	url := "https://example.com/repo.git"
	first := stubProvider{matchURL: "https://other.example.com/repo.git", auth: &fakeAuth{name: "first"}}
	second := stubProvider{matchURL: url, auth: &fakeAuth{name: "second"}}

	reg := NewRegistry(first, second)

	got, err := reg.Credentials(url)
	require.NoError(t, err)
	assert.Equal(t, second.auth, got)
}

func TestRegistryCredentialsNilWhenNoProviderApplies(t *testing.T) {
	reg := NewRegistry(stubProvider{matchURL: "https://example.com/repo.git"})

	got, err := reg.Credentials("https://elsewhere.example.com/repo.git")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestRegistryCredentialsPropagatesProviderErrorWithoutTryingLaterProviders(t *testing.T) {
	url := "https://example.com/repo.git"
	failing := stubProvider{matchURL: url, err: errors.New("boom")}
	never := stubProvider{matchURL: url, auth: &fakeAuth{name: "never"}}

	reg := NewRegistry(failing, never)

	got, err := reg.Credentials(url)
	assert.ErrorContains(t, err, "boom")
	assert.Nil(t, got)
}

func TestRegistryClientOptionsWrapsCredentials(t *testing.T) {
	url := "https://example.com/repo.git"
	reg := NewRegistry(stubProvider{matchURL: url, auth: &fakeAuth{name: "first"}})

	opts, err := reg.ClientOptions(url)
	require.NoError(t, err)
	assert.Len(t, opts, 1)
}

func TestRegistryClientOptionsEmptyWhenNoProviderApplies(t *testing.T) {
	reg := NewRegistry(stubProvider{matchURL: "https://example.com/repo.git"})

	opts, err := reg.ClientOptions("https://elsewhere.example.com/repo.git")
	require.NoError(t, err)
	assert.Empty(t, opts)
}

func TestDefaultRegistryUsesGitLabCI(t *testing.T) {
	t.Setenv("CI_JOB_TOKEN", "tok123")

	got, err := Default().Credentials("https://gitlab.com/group/project.git")
	require.NoError(t, err)
	require.NotNil(t, got)
}
