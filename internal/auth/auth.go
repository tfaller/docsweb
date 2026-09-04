// Package auth resolves HTTP credentials for cloning/fetching a remote
// (git:) scope's repository over HTTPS, so a docsweb build can reach a
// private repository without a caller having to know which hosting
// provider it lives on or how that provider expects to be authenticated.
package auth

// @docsweb
// @define auth v0.1.0
// @name Auth
// @summary
// A central registry of credential Providers, each deciding for itself
// whether/how to authenticate an HTTPS git remote URL - used by
// check's scope collection to clone/fetch a private git: scope without a
// caller needing to know which hosting provider it lives on.
// @audience dev
// @changelog
// Initial documentation.
// @doc
// # Auth
//
// `auth` depends on no other docsweb package; it only wraps
// `github.com/go-git/go-git/v6`'s own HTTP client options.
//
// A `Provider` is asked for HTTP credentials given a repository's clone
// URL, and answers `ok == false` if it has nothing to offer for that URL -
// a different hosting provider, or its own environment (typically a CI
// predefined variable) isn't present. `Registry.Credentials` tries every
// registered `Provider` in order and returns the first one that applies;
// `Registry.ClientOptions` wraps that as go-git `client.Option`s ready to
// pass straight through to [vcs.OpenScope](@link:vcs@v0.7.0)'s variadic
// argument. No provider applying at all is not an error - a build against
// a public repository never needs credentials, and the clone/fetch
// proceeds unauthenticated exactly as it did before this package existed.
//
// `Default()` returns a `Registry` carrying docsweb's built-in providers -
// today, just GitLabCI, described below.
//
// ## GitLabCI
//
// [GitLabCI](@anchor:gitlabci) authenticates an `https://gitlab.com/...`
// clone using the current job's own `CI_JOB_TOKEN` when docsweb itself is
// running as a GitLab CI job - see
// https://docs.gitlab.com/ci/jobs/ci_job_token/. That token is scoped and
// short-lived (valid only for the job's own duration), and GitLab predefines
// it in every job automatically - so a docsweb build can clone another
// private project on the same GitLab instance with no separately
// provisioned credential at all, the same way `git clone` itself would
// inside a `.gitlab-ci.yml` job. It never applies outside of a GitLab CI
// job (`CI_JOB_TOKEN` unset - the common case for a local build), to a
// non-HTTP(S) URL (e.g. an `ssh://` or scp-like remote - this provider is
// HTTPS-only), or to a URL on a different host; `GitLabCI.Host` overrides
// the matched hostname for a self-managed GitLab instance, whose CI
// predefines `CI_JOB_TOKEN` the same way.
// @docsweb

import (
	"github.com/go-git/go-git/v6/plumbing/client"
)

// Provider resolves HTTP credentials for repoURL, or reports that it has
// nothing to offer for it (ok == false) - a URL it doesn't recognize, or an
// environment it depends on (e.g. a CI-provided token) isn't present.
// Implementations must never guess: a present-but-wrong credential fails a
// clone outright, whereas ok == false just lets the next Provider (or an
// unauthenticated attempt) try instead.
type Provider interface {
	HTTPAuth(repoURL string) (auth client.HTTPAuth, ok bool, err error)
}

// Registry holds an ordered list of credential Providers.
type Registry struct {
	providers []Provider
}

// NewRegistry builds a Registry trying providers in the given order - the
// first one that recognizes a URL wins.
func NewRegistry(providers ...Provider) *Registry {
	return &Registry{providers: providers}
}

// Default returns a Registry carrying docsweb's built-in credential
// providers.
func Default() *Registry {
	return NewRegistry(GitLabCI{})
}

// Credentials returns HTTP auth for repoURL - the first registered
// Provider that recognizes the URL wins. Returns a nil auth, without
// error, if no provider applies.
func (r *Registry) Credentials(repoURL string) (client.HTTPAuth, error) {
	for _, p := range r.providers {
		a, ok, err := p.HTTPAuth(repoURL)
		if err != nil {
			return nil, err
		}
		if ok {
			return a, nil
		}
	}
	return nil, nil
}

// ClientOptions is a convenience wrapper around Credentials, turning the
// result into go-git client options ready to pass straight through to
// vcs.OpenScope's variadic argument - nil when no provider applies, so a
// caller can pass the result on unconditionally.
func (r *Registry) ClientOptions(repoURL string) ([]client.Option, error) {
	a, err := r.Credentials(repoURL)
	if err != nil || a == nil {
		return nil, err
	}
	return []client.Option{client.WithHTTPAuth(a)}, nil
}
