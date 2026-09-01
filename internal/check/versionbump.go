package check

import (
	"errors"
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"

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
// paragraph). It also rejects a @changelog that was merely appended or
// prepended to the previous version's text instead of replaced - a common
// AI-generated-documentation mistake that leaves the entry describing more
// than just this version's change (see changelogRetainsOldText).
//
// Every text comparison here - documentation content and changelog wording
// alike - ignores incidental whitespace differences (see
// normalizeWhitespace), so a formatter or editor rewrapping a comment
// without changing any word never counts as a change.
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
		relScope, err := repo.RelPath(scopeRoot)
		if err != nil {
			return err
		}
		sourcePath := path.Join(relScope, t.SourceFiles[0])

		old, found, err := oldTarget(repo, base, sourcePath, t)
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
		if changelogEqual(old.Changelog, t.Changelog) {
			return fmt.Errorf("%s: documentation changed since %s but @changelog was not updated",
				t.Key(), base.Hash)
		}
		if changelogRetainsOldText(old.Changelog, t.Changelog) {
			return fmt.Errorf("%s: @changelog still contains the previous version's text (appended or prepended) instead of describing just this version's change",
				t.Key())
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

// oldTarget re-parses sourcePath (repository-tree-relative, slash-separated
// - see vcs.Repository.FileContents) as it was committed in base and looks
// up t's own target within it. found is false (with no error) if the file
// didn't exist in base at all, or existed but didn't yet define this target
// - both mean there's no prior revision to diff against (a brand-new file or
// target), not a documentation change to flag. A malformed old revision
// (e.g. one written before some annotation grammar rule existed) is treated
// the same way rather than as a hard failure - this check is best-effort
// VCS metadata, not a correctness requirement of the current working tree.
func oldTarget(repo *vcs.Repository, base *vcs.Commit, sourcePath string, t *model.Target) (*model.Target, bool, error) {
	content, ok, err := repo.FileContents(base, sourcePath)
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
//
// Text fields are compared with normalizeWhitespace so that a formatter (or
// an editor) rewrapping a comment - changing indentation or line breaks
// without touching any actual word - never counts as a documentation
// change requiring a version bump.
func docsChanged(old, cur *model.Target) bool {
	return normalizeWhitespace(old.Summary) != normalizeWhitespace(cur.Summary) ||
		normalizeWhitespace(old.Doc) != normalizeWhitespace(cur.Doc) ||
		normalizeWhitespace(old.DisplayName) != normalizeWhitespace(cur.DisplayName) ||
		!reflect.DeepEqual(old.Uses, cur.Uses) ||
		!reflect.DeepEqual(old.Audiences, cur.Audiences)
}

// normalizeWhitespace collapses every run of whitespace (including
// newlines) to a single space and trims the ends, so two texts that differ
// only in indentation or line wrapping - the kind of change a formatter
// makes - compare equal.
func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// changelogEqual reports whether old and cur are the same @changelog,
// ignoring incidental whitespace differences (see normalizeWhitespace) in
// each entry's body. Entry count and each entry's @audience override are
// compared exactly - only the body text is whitespace-normalized.
func changelogEqual(old, cur []model.ChangelogEntry) bool {
	if len(old) != len(cur) {
		return false
	}
	for i := range old {
		if !reflect.DeepEqual(old[i].Audiences, cur[i].Audiences) {
			return false
		}
		if normalizeWhitespace(old[i].Body) != normalizeWhitespace(cur[i].Body) {
			return false
		}
	}
	return true
}

// changelogText concatenates every changelog entry's body into one
// whitespace-normalized string (see normalizeWhitespace), for comparing a
// changelog's overall wording regardless of how it happens to be split
// across entries.
func changelogText(entries []model.ChangelogEntry) string {
	bodies := make([]string, len(entries))
	for i, e := range entries {
		bodies[i] = e.Body
	}
	return normalizeWhitespace(strings.Join(bodies, "\n"))
}

// changelogRetainsOldText reports whether cur's changelog text is old's
// text with something appended or prepended, rather than a genuinely new
// description of just this version's change - a common AI mistake, and
// exactly what @changelog's "just reflect the change for the current
// version" rule (README.md's "Changelog definition") forbids: readers
// shouldn't have to spot where the old entry ends and the new one begins.
func changelogRetainsOldText(old, cur []model.ChangelogEntry) bool {
	oldText := changelogText(old)
	if oldText == "" {
		return false
	}
	curText := changelogText(cur)
	if curText == oldText {
		// Already reported by changelogEqual (same or equivalent-under-
		// whitespace text) - not an append/prepend case.
		return false
	}
	return strings.HasPrefix(curText, oldText) || strings.HasSuffix(curText, oldText)
}
