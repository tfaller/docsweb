// Package model holds the core domain types for docsweb: versions, target
// references, and the fully-parsed representation of a target.
package model

// @docsweb
// @define model v0.2.0
// @name Model
// @summary
// Core domain types: exact SemVer versions, target references, and the
// fully-merged Target representation every other package builds on.
// @audience dev
// @changelog
// Target now carries DefineLine, the source line number of its @define -
// used to attribute a version bump to whoever last changed that line via
// git blame (see internal/vcs). Non-breaking addition.
// @doc
// # Model
//
// `model` depends on no other docsweb package - every other package either
// produces or consumes these types.
//
// ## Versions
//
// `Version` is deliberately narrow: exact `major.minor.patch`, never a
// range and never pre-release/build metadata. `ParseVersion` accepts an
// optional leading `v` (`v1.0.1` or `1.0.1`).
//
// Comparing a `@uses` reference's old version against the referenced
// target's current version yields a [DiffKind](@anchor:diffkind):
//
// - `DiffMajor` - breaking, reported as outdated.
// - `DiffMinor` - non-breaking, reported as informational.
// - `DiffPatch` - ignored entirely by the pipeline.
// - `DiffNone` - up to date.
//
// `Diff(oldV, newV)` computes this classification. It is the single source
// of truth behind the outdated-uses reporting done during a build.
//
// ## References
//
// `TargetRef` identifies a specific version of a target, exactly as
// written in a `@uses` line or an `@link` destination:
// `[scope.]name@vX.Y.Z`. `ParseTargetRef` resolves a scope-less reference
// against a caller-supplied default scope - normally the scope of the file
// the reference was written in.
//
// `TargetRef.Key()` and `Target.Key()` both return the scope+name identity
// used everywhere else as a map key, deliberately ignoring the version - a
// target has exactly one live version at a time.
//
// ## Target
//
// `Target` is the fully-parsed and merged result of every docsweb block
// that defines or continues a given target name within one scope: display
// name, summary, `@uses` list, audiences, changelog entries, and the main
// `@doc` body, plus `SourceFiles` for traceability back to where each
// piece was written.
// @docsweb

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is an exact SemVer (major.minor.patch). docsweb never deals with
// version ranges or pre-release/build metadata.
type Version struct {
	Major, Minor, Patch int
}

// ParseVersion parses a version string with an optional leading "v", e.g.
// "v1.0.1" or "1.0.1".
func ParseVersion(s string) (Version, error) {
	orig := s
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version %q: expected format vX.Y.Z", orig)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		if p == "" {
			return Version{}, fmt.Errorf("invalid version %q: expected format vX.Y.Z", orig)
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("invalid version %q: component %q is not a non-negative integer", orig, p)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}, nil
}

// String renders the version as "vX.Y.Z".
func (v Version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare returns -1, 0, or 1 if v is less than, equal to, or greater than o.
func (v Version) Compare(o Version) int {
	if v.Major != o.Major {
		return cmp(v.Major, o.Major)
	}
	if v.Minor != o.Minor {
		return cmp(v.Minor, o.Minor)
	}
	return cmp(v.Patch, o.Patch)
}

// Equal reports whether v and o denote the same version.
func (v Version) Equal(o Version) bool { return v.Compare(o) == 0 }

func cmp(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// DiffKind classifies how far apart two versions of the same target are.
type DiffKind int

const (
	// DiffNone means both versions are identical.
	DiffNone DiffKind = iota
	// DiffPatch means only the patch component differs (ignored by the
	// pipeline: no outdated reporting).
	DiffPatch
	// DiffMinor means the minor component differs (non-breaking,
	// informational).
	DiffMinor
	// DiffMajor means the major component differs (breaking, reported as
	// outdated).
	DiffMajor
)

// Diff classifies the difference between an old version (e.g. referenced by
// a @uses) and the current/new version of the same target.
func Diff(oldV, newV Version) DiffKind {
	switch {
	case oldV.Major != newV.Major:
		return DiffMajor
	case oldV.Minor != newV.Minor:
		return DiffMinor
	case oldV.Patch != newV.Patch:
		return DiffPatch
	default:
		return DiffNone
	}
}
