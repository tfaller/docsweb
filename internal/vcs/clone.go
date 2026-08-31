package vcs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	gitfs "github.com/tfaller/go-git-fs"
)

// OpenScope ensures a local bare mirror of repoURL exists under cacheDir,
// resolves ref to a commit, and returns a read-only fs.FS over that
// commit's file tree - straight out of git's object store via
// gitfs.OpenFromCommit, no worktree checkout involved - alongside a
// Repository ready for blame/content lookups pinned to that same commit. If
// cacheDir has no mirror of repoURL yet, one is created with a fresh bare
// "git clone"; otherwise the existing mirror's origin is fetched instead of
// cloning from scratch, so repeated builds against the same repository stay
// cheap. ref is resolved as a branch, a tag, or a commit SHA (in that
// order); an empty ref uses the repository's default branch (HEAD at clone
// time).
func OpenScope(cacheDir, repoURL, ref string) (fs.FS, *Repository, error) {
	dir := filepath.Join(cacheDir, cloneDirName(repoURL))

	repo, err := git.PlainOpen(dir)
	switch {
	case errors.Is(err, git.ErrRepositoryNotExists):
		repo, err = git.PlainClone(dir, &git.CloneOptions{URL: repoURL, Tags: git.AllTags, Bare: true})
		if err != nil {
			return nil, nil, fmt.Errorf("vcs: cloning %s: %w", repoURL, err)
		}
	case err != nil:
		return nil, nil, fmt.Errorf("vcs: opening cached clone of %s at %s: %w", repoURL, dir, err)
	default:
		err = repo.Fetch(&git.FetchOptions{RemoteName: "origin", Tags: git.AllTags, Force: true})
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil, nil, fmt.Errorf("vcs: fetching %s: %w", repoURL, err)
		}
	}

	hash, err := resolveCloneRef(repo, ref)
	if err != nil {
		return nil, nil, fmt.Errorf("vcs: resolving ref %q in %s: %w", ref, repoURL, err)
	}
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, nil, fmt.Errorf("vcs: reading commit %s of %s: %w", hash, repoURL, err)
	}

	treeFS, err := gitfs.OpenFromCommit(commit)
	if err != nil {
		return nil, nil, fmt.Errorf("vcs: opening tree of %s in %s: %w", hash, repoURL, err)
	}

	return treeFS, &Repository{repo: repo, commit: commit, blame: map[blameKey]*git.BlameResult{}}, nil
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
		// A bare mirror's fetch (below) only ever moves refs/remotes/origin/*
		// - the local refs/heads/<default> ref created once, by the initial
		// clone, is never touched again since nothing here ever creates
		// another - so on a later call there's no symbolic HEAD left that
		// reflects new commits. The clone's sole local branch ref survives
		// across fetches, so its name is used to look up the matching
		// remote-tracking ref instead, which - unlike the local branch
		// itself - does pick up new commits.
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
