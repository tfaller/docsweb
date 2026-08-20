// Package ignore implements a gitignore-style pattern matcher, used to
// exclude files/directories from a docsweb scope walk via a .docsweb.yaml
// "ignore:" list (see README.md's "Scopes" section for the config property
// this implements).
//
// Supported syntax mirrors the common subset of .gitignore patterns: blank
// lines and lines starting with "#" are comments; a leading "!" negates a
// pattern (a later rule wins over an earlier one, exactly like git); a
// trailing "/" restricts a pattern to directories; a leading "/", or any "/"
// before the last character, anchors the pattern to the ignore base
// directory, otherwise it matches at any depth; "*" matches any run of
// characters within one path segment, "?" matches exactly one, and "**"
// matches any number of path segments (including zero) when used as a whole
// segment on its own, or as a leading/trailing "**/"/"/ **" segment.
// POSIX-style "[...]" character classes are not supported - treated as
// literal characters.
package ignore

// @docsweb
// @define ignore v0.1.0
// @name Ignore
// @summary
// A gitignore-style pattern matcher used to exclude files and directories
// from a scope walk via a .docsweb.yaml "ignore:" list.
// @audience dev
// @changelog
// Initial documentation.
// @doc
// # Ignore
//
// `ignore` depends on no other docsweb package; it operates purely on
// path strings.
//
// `Compile` turns a `.docsweb.yaml` "ignore:" list into a `*Matcher`.
// Supported syntax mirrors the common subset of `.gitignore`:
//
// - blank lines and `#` comments are skipped
// - a leading `!` negates a rule - a later rule wins over an earlier one
// - a trailing `/` restricts a pattern to directories
// - a leading `/`, or any `/` before the pattern's final character,
//   anchors it to the ignore base directory; otherwise it matches at any
//   depth
// - `*` matches any run of characters within one path segment, `?`
//   matches exactly one, `**` matches any number of path segments
// - POSIX `[...]` character classes are **not** supported - treated as
//   literal characters
//
// A `nil *Matcher` matches nothing, so callers can always call `Match`
// without a nil check, whether or not a config declared any `ignore:`
// rules at all.
//
// This is what makes this project's own root `.docsweb.yaml` possible: it
// `ignore`s test fixtures, generated build output, and other paths that
// would otherwise collide with or break docsweb's real documentation
// build - the one that produced this very page.
// @docsweb

import (
	"regexp"
	"strings"
)

// rule is one compiled ignore pattern.
type rule struct {
	re      *regexp.Regexp
	negate  bool
	dirOnly bool
}

// Matcher holds a compiled, ordered list of gitignore-style rules. A nil
// *Matcher matches nothing, so it is always safe to call Match on it.
type Matcher struct {
	rules []rule
}

// Compile parses patterns (one per .docsweb.yaml "ignore:" entry) into a
// Matcher. Blank entries and entries starting with "#" are ignored.
func Compile(patterns []string) *Matcher {
	m := &Matcher{}
	for _, raw := range patterns {
		pat := strings.TrimRight(raw, " \t")
		if pat == "" || strings.HasPrefix(pat, "#") {
			continue
		}
		m.rules = append(m.rules, compileRule(pat))
	}
	return m
}

// Match reports whether relPath ("/"-separated, relative to whatever
// directory the ignore patterns are anchored to) is ignored. isDir must
// reflect whether relPath names a directory, since a trailing-"/" pattern
// only matches directories. As in a real .gitignore, later rules override
// earlier ones, so a "!"-negated rule can re-include a path an earlier
// pattern excluded.
func (m *Matcher) Match(relPath string, isDir bool) bool {
	if m == nil {
		return false
	}
	relPath = strings.TrimPrefix(relPath, "./")
	ignored := false
	for _, r := range m.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.re.MatchString(relPath) {
			ignored = !r.negate
		}
	}
	return ignored
}

func compileRule(pat string) rule {
	negate := false
	if strings.HasPrefix(pat, "!") {
		negate = true
		pat = pat[1:]
	}
	pat = strings.TrimPrefix(pat, `\`)

	dirOnly := false
	if strings.HasSuffix(pat, "/") {
		dirOnly = true
		pat = strings.TrimSuffix(pat, "/")
	}

	anchored := strings.HasPrefix(pat, "/")
	if anchored {
		pat = strings.TrimPrefix(pat, "/")
	} else if strings.Contains(pat, "/") {
		// A "/" anywhere but the end anchors the pattern to the ignore
		// base, per gitignore's own rule.
		anchored = true
	}

	body := translate(pat)
	expr := "^" + body + "$"
	if !anchored {
		expr = "^(?:.*/)?" + body + "$"
	}
	return rule{re: regexp.MustCompile(expr), negate: negate, dirOnly: dirOnly}
}

// translate converts a single gitignore glob pattern (already stripped of
// its leading "!"/"/" and trailing "/") into an equivalent regexp fragment.
func translate(pat string) string {
	var b strings.Builder
	n := len(pat)
	for i := 0; i < n; {
		switch {
		case strings.HasPrefix(pat[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 3
		case pat[i:] == "**":
			b.WriteString(".*")
			i += 2
		case pat[i] == '*':
			b.WriteString("[^/]*")
			i++
		case pat[i] == '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(pat[i : i+1]))
			i++
		}
	}
	return b.String()
}
