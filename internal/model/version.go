// Package model holds the core domain types for docsweb: versions, target
// references, and the fully-parsed representation of a target.
package model

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
