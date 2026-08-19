package model

import (
	"fmt"
	"regexp"
	"strings"
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// ValidName reports whether s is a valid target/scope-segment/audience name:
// alphanumeric only, case-sensitive, non-empty.
func ValidName(s string) bool {
	return nameRe.MatchString(s)
}

// TargetRef identifies a specific version of a target, as written in a
// @uses line or an @link markdown destination: "scope.targetName@vX.Y.Z" or,
// scope-less, "targetName@vX.Y.Z" (resolved against a default scope).
type TargetRef struct {
	Scope   string // dot-joined scope path; "" means the default/root scope
	Name    string
	Version Version
}

// Key returns the scope+name identity of the referenced target, ignoring
// version - this is what uniquely identifies a target within a build.
func (r TargetRef) Key() string {
	if r.Scope == "" {
		return r.Name
	}
	return r.Scope + "." + r.Name
}

// String renders the ref back to its canonical "scope.name@version" form.
func (r TargetRef) String() string {
	if r.Scope == "" {
		return fmt.Sprintf("%s@%s", r.Name, r.Version)
	}
	return fmt.Sprintf("%s.%s@%s", r.Scope, r.Name, r.Version)
}

// ParseTargetRef parses "[scope.]targetName@vX.Y.Z". When the ref has no
// scope prefix, defaultScope is used (typically the scope of the file the
// reference was written in).
func ParseTargetRef(s string, defaultScope string) (TargetRef, error) {
	orig := s
	s = strings.TrimSpace(s)
	at := strings.LastIndex(s, "@")
	if at < 0 {
		return TargetRef{}, fmt.Errorf("invalid target reference %q: missing @version", orig)
	}
	path, verStr := s[:at], s[at+1:]
	if path == "" {
		return TargetRef{}, fmt.Errorf("invalid target reference %q: missing target name", orig)
	}
	version, err := ParseVersion(verStr)
	if err != nil {
		return TargetRef{}, fmt.Errorf("invalid target reference %q: %w", orig, err)
	}

	segs := strings.Split(path, ".")
	for _, seg := range segs {
		if !ValidName(seg) {
			return TargetRef{}, fmt.Errorf("invalid target reference %q: %q is not a valid alphanumeric name", orig, seg)
		}
	}

	name := segs[len(segs)-1]
	scope := strings.Join(segs[:len(segs)-1], ".")
	if scope == "" {
		scope = defaultScope
	}
	return TargetRef{Scope: scope, Name: name, Version: version}, nil
}

// ParseAudiences splits a comma-separated @audience list, trimming
// whitespace around names and commas.
func ParseAudiences(s string) ([]Audience, error) {
	parts := strings.Split(s, ",")
	out := make([]Audience, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !ValidName(p) {
			return nil, fmt.Errorf("invalid audience name %q", p)
		}
		out = append(out, Audience(p))
	}
	return out, nil
}
