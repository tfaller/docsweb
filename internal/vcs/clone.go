package vcs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

// CloneOrFetch ensures a local, up-to-date clone of repoURL exists under
// cacheDir, checked out to ref, and returns that clone's working-tree root.
// If cacheDir has no clone of repoURL yet, one is created with a fresh
// "git clone"; otherwise the existing clone's origin is fetched instead of
// cloning from scratch, so repeated builds against the same repository stay
// cheap. ref is resolved as a branch, a tag, or a commit SHA (in that
// order); an empty ref uses the repository's default branch (HEAD at clone
// time).
func CloneOrFetch(cacheDir, repoURL, ref string) (string, error) {
	dir := filepath.Join(cacheDir, cloneDirName(repoURL))

	repo, err := git.PlainOpen(dir)
	switch {
	case errors.Is(err, git.ErrRepositoryNotExists):
		repo, err = git.PlainClone(dir, &git.CloneOptions{URL: repoURL, Tags: git.AllTags})
		if err != nil {
			return "", fmt.Errorf("vcs: cloning %s: %w", repoURL, err)
		}
	case err != nil:
		return "", fmt.Errorf("vcs: opening cached clone of %s at %s: %w", repoURL, dir, err)
	default:
		err = repo.Fetch(&git.FetchOptions{RemoteName: "origin", Tags: git.AllTags, Force: true})
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return "", fmt.Errorf("vcs: fetching %s: %w", repoURL, err)
		}
	}

	hash, err := resolveCloneRef(repo, ref)
	if err != nil {
		return "", fmt.Errorf("vcs: resolving ref %q in %s: %w", ref, repoURL, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("vcs: %s: %w", repoURL, err)
	}
	if err := wt.Checkout(&git.CheckoutOptions{Hash: hash, Force: true}); err != nil {
		return "", fmt.Errorf("vcs: checking out %q in %s: %w", ref, repoURL, err)
	}

	return dir, nil
}

// resolveCloneRef resolves ref against repo, trying it in turn as a local
// or remote-tracking branch, a tag, and finally any revision go-git itself
// understands (a commit SHA, "HEAD", "HEAD~1", ...) - the same fallback
// order Repository.Commit's callers get for free, but ref here comes from a
// referenced scope's config, not the local working tree, so remote-tracking
// branches are tried explicitly. An empty ref resolves to the current HEAD,
// i.e. the repository's default branch right after a fresh clone.
func resolveCloneRef(repo *git.Repository, ref string) (plumbing.Hash, error) {
	if ref == "" {
		// CloneOrFetch always checks a cached clone's worktree out by exact
		// commit hash (below), which leaves HEAD detached - so on a later
		// call there's no symbolic HEAD left to read the default branch's
		// name from. The clone's sole local branch ref (created once, by
		// the initial "git clone", and never touched again since nothing
		// here ever creates another) survives that detachment, so it's
		// used instead: a fetch only moves the matching remote-tracking
		// ref, not this local, possibly-stale one, so that remote-tracking
		// ref - not the local branch itself - is what actually picks up
		// new commits.
		if branches, err := repo.Branches(); err == nil {
			defer branches.Close()
			if b, err := branches.Next(); err == nil {
				if hash, err := repo.ResolveRevision(plumbing.Revision(plumbing.NewRemoteReferenceName("origin", b.Name().Short()))); err == nil {
					return *hash, nil
				}
			}
		}
		head, err := repo.Head()
		if err != nil {
			return plumbing.ZeroHash, err
		}
		return head.Hash(), nil
	}

	candidates := []plumbing.Revision{
		plumbing.Revision(plumbing.NewRemoteReferenceName("origin", ref)),
		plumbing.Revision(plumbing.NewTagReferenceName(ref)),
		plumbing.Revision(ref),
	}
	var lastErr error
	for _, c := range candidates {
		hash, err := repo.ResolveRevision(c)
		if err == nil {
			return *hash, nil
		}
		lastErr = err
	}
	return plumbing.ZeroHash, lastErr
}

// cloneDirName turns repoURL into a filesystem-safe, deterministic
// directory name under a cache directory - a hash rather than a sanitized
// URL, since git URLs can take forms (scp-like ssh, "://" schemes, embedded
// credentials) that don't sanitize into a short, collision-free path
// segment.
func cloneDirName(repoURL string) string {
	sum := sha256.Sum256([]byte(repoURL))
	return hex.EncodeToString(sum[:])[:16]
}
