package check

import (
	"fmt"
	"sort"

	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/model"
)

// UsageIssue is one @uses reference whose referenced version is behind the
// referenced target's current version, per README.md's "Pipeline" section:
// major differences are breaking ("outdated"), minor are informational.
// Patch-only differences are ignored entirely and never produce an issue.
type UsageIssue struct {
	// User is the target that declared the @uses reference.
	User *model.Target
	// Use is the reference as written (the old version).
	Use model.TargetRef
	// Current is the referenced target's current version.
	Current model.Version
	// Kind is either model.DiffMajor (breaking) or model.DiffMinor
	// (informational) - never DiffNone/DiffPatch.
	Kind model.DiffKind
}

// checkUses validates that every target's @uses references an existing
// target in the registry (a hard error otherwise, per README.md's "check
// that all @link and @uses land at an existing target"), and populates
// ctx.issues with every major/minor outdated usage.
func checkUses(ctx *context) error {
	issues, err := ResolveUses(ctx.registry)
	if err != nil {
		return err
	}
	ctx.issues = issues
	return nil
}

// ResolveUses validates that every target's @uses references an existing
// target in the registry (a hard error otherwise, per README.md's "check
// that all @link and @uses land at an existing target"), and returns every
// major/minor outdated usage, sorted deterministically by user then
// referenced target.
func ResolveUses(reg *collect.Registry) ([]UsageIssue, error) {
	var issues []UsageIssue

	for _, t := range reg.Targets() {
		for _, use := range t.Uses {
			referenced, ok := reg.Get(use.Key())
			if !ok {
				return nil, fmt.Errorf("%s: @uses %s: target does not exist", t.Key(), use)
			}

			kind := model.Diff(use.Version, referenced.Version)
			switch kind {
			case model.DiffMajor, model.DiffMinor:
				issues = append(issues, UsageIssue{
					User:    t,
					Use:     use,
					Current: referenced.Version,
					Kind:    kind,
				})
			case model.DiffNone, model.DiffPatch:
				// Up to date, or only a patch behind: ignored by the
				// pipeline per README.md's "Pipeline" section.
			}
		}
	}

	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.User.Key() != b.User.Key() {
			return a.User.Key() < b.User.Key()
		}
		return a.Use.Key() < b.Use.Key()
	})

	return issues, nil
}

// UsedByRef records that a target depends on another via @uses. User is the
// dependent target's own current ref; Use is the exact (possibly outdated)
// ref it declared in its @uses.
type UsedByRef struct {
	User model.TargetRef
	Use  model.TargetRef
}

// ComputeUsedBy inverts every target's @uses list into a "who depends on me"
// index keyed by the referenced target's Key(), sorted deterministically by
// dependant. Per README.md's "Usage graph" section @uses already implies its
// own reverse edge, so this is derived straight from the registry - no
// separate annotation is needed to declare a dependant. Never errors, so it
// is not itself a Check - it's derived once every check has already passed,
// as part of assembling a Result.
func ComputeUsedBy(reg *collect.Registry) map[string][]UsedByRef {
	usedBy := make(map[string][]UsedByRef)
	for _, t := range reg.Targets() {
		for _, use := range t.Uses {
			usedBy[use.Key()] = append(usedBy[use.Key()], UsedByRef{User: t.Ref(), Use: use})
		}
	}
	for key, entries := range usedBy {
		sort.Slice(entries, func(i, j int) bool { return entries[i].User.Key() < entries[j].User.Key() })
		usedBy[key] = entries
	}
	return usedBy
}
