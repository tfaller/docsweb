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

// ParseQualifiedName splits a dot-joined "[scope.]name" path into its scope
// and name parts: the last segment is the name, everything before it
// (rejoined with ".") is the scope. Each segment must be a ValidName. A path
// with no dots returns scope "".
func ParseQualifiedName(path string) (scope, name string, err error) {
	segs := strings.Split(path, ".")
	for _, seg := range segs {
		if !ValidName(seg) {
			return "", "", fmt.Errorf("%q is not a valid alphanumeric name", seg)
		}
	}
	name = segs[len(segs)-1]
	scope = strings.Join(segs[:len(segs)-1], ".")
	return scope, name, nil
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

	scope, name, err := ParseQualifiedName(path)
	if err != nil {
		return TargetRef{}, fmt.Errorf("invalid target reference %q: %w", orig, err)
	}
	if scope == "" {
		scope = defaultScope
	}
	return TargetRef{Scope: scope, Name: name, Version: version}, nil
}

// ParseDefineName parses an @define target name, which may be written in one
// of three forms:
//
//   - bare, e.g. "login": relative shorthand, implicitly scoped under
//     configScope.
//   - leading-dot, e.g. ".auth.login": explicit relative form - configScope
//     with the (possibly multi-segment) remainder appended, without having
//     to retype configScope itself.
//   - fully qualified, e.g. "docsweb.auth.login" (multiple segments, no
//     leading dot): taken literally as the target's absolute scope, and
//     validated to equal or descend from configScope (skipped entirely when
//     configScope is "", since there's then no self-declared identity to
//     check against).
//
// The two relative forms are derived by concatenation, never asserted, so
// they can never fail the configScope check the absolute form is subject to.
func ParseDefineName(raw, configScope string) (scope, name string, err error) {
	relative := strings.HasPrefix(raw, ".")
	body := raw
	if relative {
		body = raw[1:]
	}

	bodyScope, bodyName, err := ParseQualifiedName(body)
	if err != nil {
		return "", "", err
	}

	if relative || bodyScope == "" {
		full := configScope
		if bodyScope != "" {
			if full != "" {
				full += "."
			}
			full += bodyScope
		}
		return full, bodyName, nil
	}

	if configScope != "" && bodyScope != configScope && !strings.HasPrefix(bodyScope, configScope+".") {
		return "", "", fmt.Errorf("declared scope %q does not match or descend from its scope %q", bodyScope, configScope)
	}
	return bodyScope, bodyName, nil
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
