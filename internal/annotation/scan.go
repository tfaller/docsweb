// Package annotation implements the docsweb comment-block grammar: finding
// @docsweb blocks inside source-code comments and parsing their tag
// structure. It is purely textual - it knows nothing about scopes, semver,
// or target registries, so it can be tested against raw source snippets in
// isolation. See README.md "Annotation grammar" for the spec this
// implements.
package annotation

import "strings"

// blockDelim is a pair of block-comment delimiters, e.g. "/*" and "*/".
type blockDelim struct{ open, close string }

// For the POC, comment styles are detected by fixed delimiters rather than
// by file extension/language (see PLAN.md open question #1).
var (
	linePrefixes = []string{"//", "#"}
	blockDelims  = []blockDelim{{"/*", "*/"}, {"<!--", "-->"}}
)

// rawComment is a contiguous comment region found in a source file: either a
// single block comment, or a run of consecutive line comments sharing the
// same prefix.
type rawComment struct {
	// lines are the comment's content lines with the comment
	// delimiters/prefixes already stripped.
	lines []string
}

// findComments scans src and returns every comment region, in source order.
func findComments(src string) []rawComment {
	lines := strings.Split(src, "\n")
	var out []rawComment

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if delim, ok := matchBlockOpen(trimmed); ok {
			// Collect until the matching close delimiter, which may be on
			// the same line or a later one.
			var content []string
			rest := trimmed[len(delim.open):]
			closed := false
			for {
				if idx := strings.Index(rest, delim.close); idx >= 0 {
					content = append(content, rest[:idx])
					closed = true
					i++
					break
				}
				content = append(content, rest)
				i++
				if i >= len(lines) {
					break
				}
				rest = lines[i]
			}
			_ = closed
			out = append(out, rawComment{lines: content})
			continue
		}

		if prefix, ok := matchLinePrefix(trimmed); ok {
			var content []string
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				if t == prefix {
					content = append(content, "")
					i++
					continue
				}
				p, ok := matchLinePrefixExact(t, prefix)
				if !ok {
					break
				}
				content = append(content, p)
				i++
			}
			out = append(out, rawComment{lines: content})
			continue
		}

		i++
	}

	return out
}

func matchBlockOpen(trimmed string) (blockDelim, bool) {
	for _, d := range blockDelims {
		if strings.HasPrefix(trimmed, d.open) {
			return d, true
		}
	}
	return blockDelim{}, false
}

// matchLinePrefix reports the comment prefix a run starts with, requiring a
// space/tab or end-of-line after the prefix token so e.g. "#!/bin/sh" is not
// mistaken for a "#" comment... actually shebangs do start a "#" comment
// under this simple rule; callers only care about docsweb tags so false
// positives on ordinary comments are harmless.
func matchLinePrefix(trimmed string) (string, bool) {
	for _, p := range linePrefixes {
		if strings.HasPrefix(trimmed, p) {
			return p, true
		}
	}
	return "", false
}

func matchLinePrefixExact(trimmed, prefix string) (string, bool) {
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}
	return strings.TrimPrefix(trimmed, prefix), true
}
