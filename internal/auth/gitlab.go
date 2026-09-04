package auth

import (
	"net/url"
	"os"

	"github.com/go-git/go-git/v6/plumbing/client"
	"github.com/go-git/go-git/v6/plumbing/transport/http"
)

// GitLabCI is a Provider that authenticates an HTTPS clone of a
// gitlab.com (or Host-overridden) repository using the current GitLab CI
// job's own CI_JOB_TOKEN. See auth.go's docsweb block for details.
type GitLabCI struct {
	// Host is the GitLab instance's hostname repoURL must match. Empty
	// defaults to "gitlab.com".
	Host string
}

// HTTPAuth implements Provider. It applies only when CI_JOB_TOKEN is set,
// repoURL is an http(s) URL, and its host matches Host (or "gitlab.com" if
// Host is empty) - never guessing at credentials for a URL it doesn't
// recognize.
func (p GitLabCI) HTTPAuth(repoURL string) (client.HTTPAuth, bool, error) {
	token := os.Getenv("CI_JOB_TOKEN")
	if token == "" {
		return nil, false, nil
	}

	u, err := url.Parse(repoURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, false, nil
	}

	host := p.Host
	if host == "" {
		host = "gitlab.com"
	}
	if u.Hostname() != host {
		return nil, false, nil
	}

	return &http.BasicAuth{Username: "gitlab-ci-token", Password: token}, true, nil
}
