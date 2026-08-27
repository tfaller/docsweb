package check

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/tfaller/docsweb/internal/annotation"
	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/model"
	"github.com/tfaller/docsweb/internal/vcs"
)

// checkVersionBump verifies that every target whose documentation changed
// since a comparison base also bumped its @define version and updated its
// @changelog - a documented change with no version bump would leave a
// @uses reference pinned at the old version silently stale, and a version
// bump with no changelog entry leaves readers with no way to tell what
// changed for the new version (per README.md's "Changelogs are important"
// paragraph).
//
// This is best-effort, VCS-only metadata, like internal/vcs's git-blame
// attribution: it silently does nothing outside of a git repository, and
// skips any target whose defining file didn't exist yet at the comparison
// base (nothing to diff against - the target itself is new). See
// baseRevision for how that base is chosen.
func checkVersionBump(ctx *context) error {
	repo, err := vcs.Open(ctx.rootDir)
	if errors.Is(err, vcs.ErrNoRepository) {
		return nil
	}
	if err != nil {
		return err
	}

	base, err := baseRevision(repo, ctx.opts.Base)
	if err != nil {
		return err
	}

	for _, t := range ctx.registry.Targets() {
		scopeRoot, ok := ctx.scopeRoots[t.ConfigScope]
		if !ok || len(t.SourceFiles) == 0 {
			continue
		}
		absPath := filepath.Join(scopeRoot, t.SourceFiles[0])

		old, found, err := oldTarget(repo, base, absPath, t)
		if err != nil {
			return fmt.Errorf("%s: %w", t.Key(), err)
		}
		if !found || !docsChanged(old, t) {
			continue
		}

		if old.Version == t.Version {
			return fmt.Errorf("%s: documentation changed since %s but the @define version was not bumped (still %s)",
				t.Key(), base.Hash, t.Version)
		}
		if reflect.DeepEqual(old.Changelog, t.Changelog) {
			return fmt.Errorf("%s: documentation changed since %s but @changelog was not updated",
				t.Key(), base.Hash)
		}
	}

	return nil
}

// baseRevision picks the commit a target's current documentation is diffed
// against:
//
//   - override, if non-empty (Options.Base) - an explicit caller choice
//     always wins.
//   - otherwise, in a GitLab merge-request or GitHub pull-request CI
//     pipeline (detected via their respective environment variables), the
//     merge base against the request's target branch - comparing against a
//     stable common ancestor rather than the target branch's ever-moving
//     tip, exactly like "git diff" would when reviewing the request.
//   - otherwise, the current HEAD - so a local, non-CI run compares the
//     working tree (uncommitted edits included) against the last commit.
func baseRevision(repo *vcs.Repository, override string) (*vcs.Commit, error) {
	if override != "" {
		return repo.Commit(override)
	}
	if rev, mergeBase, ok := ciBaseRevision(); ok {
		if !mergeBase {
			return repo.Commit(rev)
		}
		return repo.MergeBase("HEAD", rev)
	}
	return repo.Commit("HEAD")
}

// ciBaseRevision inspects well-known GitLab/GitHub CI environment variables
// to find what a merge/pull request currently being validated is being
// merged into. mergeBase reports whether rev still needs to be resolved to
// a merge base against HEAD (a branch name) or can be used as-is (GitLab
// exposes the merge base's SHA directly). ok is false outside of a
// recognized merge/pull-request pipeline, in which case the caller falls
// back to HEAD.
func ciBaseRevision() (rev string, mergeBase bool, ok bool) {
	// GitLab CI: https://docs.gitlab.com/ci/variables/predefined_variables/
	// CI_MERGE_REQUEST_DIFF_BASE_SHA is already the merge base itself.
	if sha := os.Getenv("CI_MERGE_REQUEST_DIFF_BASE_SHA"); sha != "" {
		return sha, false, true
	}
	if branch := os.Getenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME"); branch != "" {
		return "origin/" + branch, true, true
	}
	// GitHub Actions: https://docs.github.com/actions/learn-github-actions/variables
	// only the target branch's name is exposed, so the merge base still
	// needs to be computed against HEAD.
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		switch os.Getenv("GITHUB_EVENT_NAME") {
		case "pull_request", "pull_request_target":
			if branch := os.Getenv("GITHUB_BASE_REF"); branch != "" {
				return "origin/" + branch, true, true
			}
		}
	}
	return "", false, false
}

// oldTarget re-parses absPath as it was committed in base and looks up t's
// own target within it. found is false (with no error) if the file didn't
// exist in base at all, or existed but didn't yet define this target - both
// mean there's no prior revision to diff against (a brand-new file or
// target), not a documentation change to flag. A malformed old revision
// (e.g. one written before some annotation grammar rule existed) is treated
// the same way rather than as a hard failure - this check is best-effort
// VCS metadata, not a correctness requirement of the current working tree.
func oldTarget(repo *vcs.Repository, base *vcs.Commit, absPath string, t *model.Target) (*model.Target, bool, error) {
	content, ok, err := repo.FileContents(base, absPath)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	docs, err := annotation.ParseSource(content)
	if err != nil {
		return nil, false, nil
	}

	for _, doc := range docs {
		old, err := collect.ToTarget(t.ConfigScope, t.SourceFiles[0], doc)
		if err != nil {
			continue
		}
		if old.Key() == t.Key() {
			return old, true, nil
		}
	}
	return nil, false, nil
}

// docsChanged reports whether anything a reader would see as "the docs"
// differs between old and cur: the summary/body Markdown, display name, and
// the target's @uses/@audience declarations. @changelog and @define's
// version are deliberately excluded - those are exactly what checkVersionBump
// requires to change *because* this is true, not inputs to the comparison
// itself.
func docsChanged(old, cur *model.Target) bool {
	return old.Summary != cur.Summary ||
		old.Doc != cur.Doc ||
		old.DisplayName != cur.DisplayName ||
		!reflect.DeepEqual(old.Uses, cur.Uses) ||
		!reflect.DeepEqual(old.Audiences, cur.Audiences)
}
