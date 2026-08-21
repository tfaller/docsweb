// Package annotation implements the docsweb comment-block grammar: finding
// @docsweb blocks inside source-code comments and parsing their tag
// structure. It is purely textual - it knows nothing about scopes, semver,
// or target registries, so it can be tested against raw source snippets in
// isolation. See README.md "Annotation grammar" for the spec this
// implements.
package annotation

// @docsweb
// @define annotation v0.2.0
// @name Annotation
// @summary
// Comment scanning and the @docsweb block grammar: turns raw source text
// into a per-file list of merged, still-unvalidated target docs.
// @audience dev
// @changelog
// Every parsed TargetDoc now carries DefineLine, the 1-based source line
// number of its @define line, so a caller (git-blame attribution) can find
// exactly which line introduced the current version. Non-breaking addition.
// @doc
// # Annotation
//
// `annotation` depends on no other docsweb package - it is purely
// textual, knowing nothing about scopes, SemVer, or target registries
// (that's the job of scope collection). Everything it produces, including
// `@uses`/`@audience` content, stays a raw string; validating those
// strings into real references and versions happens one layer up, once a
// caller can supply a scope to resolve against.
//
// ## Finding comments
//
// `findComments` scans a file for line-comment runs (`//`, `#`) and block
// comments (`/* */`, `<!-- -->`), independent of file extension - the POC
// detects comment syntax by fixed delimiter, not by language.
//
// ## The block grammar
//
// A [docsweb block](@anchor:grammar) starts at a line reading exactly
// `@docsweb` and ends at the next `@docsweb` line, or the natural end of
// the surrounding comment: a block comment's closing delimiter, or the
// first non-comment/blank line ending a run of line comments. Before the
// block is read, the common leading whitespace of all its lines is
// stripped once, so indentation in the source file never leaks into the
// documentation itself.
//
// Recognized tags: `@define`, `@name`, `@summary`, `@uses`, `@audience`,
// `@changelog`, `@doc`. Anything else belongs to whichever tag was opened
// last, verbatim, as Markdown - including blank lines, which are a
// paragraph break rather than a section boundary. A fenced code block
// suppresses tag recognition, including `@docsweb` itself, so the grammar
// can be shown as a literal example without being parsed as one:
//
// ```
// @docsweb
// @define example v1.0.0
// @doc
// Shown here as an example only - this fence keeps it from being parsed.
// @docsweb
// ```
//
// `@changelog` has one carve-out: it may be immediately followed by its
// own `@audience` override line before the entry body continues. The
// override line is reattributed to the changelog entry rather than parsed
// as a second, empty `@audience` section.
//
// ## Multiple blocks, one file
//
// `ParseSource` merges every block in a file in source order: the first
// block must `@define` a target; any later block without `@define`
// concatenates onto the *previous* target instead of starting a new one -
// singular fields (name, summary, doc) join with a blank line, list
// fields (`@uses`, `@audience`, `@changelog`) simply append.
// @docsweb

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
	// lineNos are lines' 1-based source line numbers, parallel to lines -
	// tracked so a tag's original source position (e.g. an @define line, for
	// git-blame attribution) survives comment-stripping/dedenting.
	lineNos []int
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
			var contentLines []int
			rest := trimmed[len(delim.open):]
			closed := false
			for {
				if idx := strings.Index(rest, delim.close); idx >= 0 {
					content = append(content, rest[:idx])
					contentLines = append(contentLines, i+1)
					closed = true
					i++
					break
				}
				content = append(content, rest)
				contentLines = append(contentLines, i+1)
				i++
				if i >= len(lines) {
					break
				}
				rest = lines[i]
			}
			_ = closed
			out = append(out, rawComment{lines: content, lineNos: contentLines})
			continue
		}

		if prefix, ok := matchLinePrefix(trimmed); ok {
			var content []string
			var contentLines []int
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				if t == prefix {
					content = append(content, "")
					contentLines = append(contentLines, i+1)
					i++
					continue
				}
				p, ok := matchLinePrefixExact(t, prefix)
				if !ok {
					break
				}
				content = append(content, p)
				contentLines = append(contentLines, i+1)
				i++
			}
			out = append(out, rawComment{lines: content, lineNos: contentLines})
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
