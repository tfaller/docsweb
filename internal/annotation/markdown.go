package annotation

import (
	"fmt"
	"strings"
)

// ParseMarkdownSource parses src as a Markdown file that may define a single
// docsweb target via one leading @docsweb annotation comment at the very
// top of the file. Unlike ParseSource, the target's documentation is not
// written with @doc inside the comment - it's the file's own Markdown body,
// everything after the comment closes.
//
// Returns (nil, nil) if the file has no such leading comment, or the leading
// comment has no @docsweb marker at all - callers should treat that as "not
// a docsweb target", exactly like ParseSource returns no TargetDoc for a
// file with no @docsweb block.
func ParseMarkdownSource(src string) (*TargetDoc, error) {
	comments := findComments(src)
	if len(comments) == 0 || !leadsFile(src, comments[0]) {
		return nil, nil
	}
	first := comments[0]

	blocks := extractBlocksFromComment(first)
	if len(blocks) == 0 {
		return nil, nil
	}
	if len(blocks) > 1 {
		return nil, fmt.Errorf("a Markdown file's annotation comment may contain only one @docsweb block")
	}

	sections := parseSections(blocks[0].lines, blocks[0].lineNos)
	for _, s := range sections {
		if s.Tag == "doc" {
			return nil, fmt.Errorf("@doc is not allowed in a Markdown file's annotation comment; the file's Markdown body after the comment is used as its documentation")
		}
	}

	bd, err := semanticize(sections)
	if err != nil {
		return nil, err
	}
	if !bd.hasDefine {
		return nil, fmt.Errorf("a Markdown file's leading @docsweb block must @define a target")
	}

	return &TargetDoc{
		Name:        bd.name,
		VersionRaw:  bd.versionRaw,
		DisplayName: bd.displayName,
		Summary:     bd.summary,
		UsesRaw:     bd.usesRaw,
		AudienceRaw: bd.audienceRaw,
		Changelog:   bd.changelog,
		Doc:         bodyAfterComment(src, first),
		DefineLine:  bd.defineLine,
	}, nil
}

// leadsFile reports whether c is the file's first non-blank content - i.e.
// nothing but blank lines precede it - which is what makes a comment a
// Markdown file's "toplevel" annotation comment rather than one buried
// further down, e.g. inside a fenced example in the Markdown body.
func leadsFile(src string, c rawComment) bool {
	if len(c.lineNos) == 0 {
		return false
	}
	lines := strings.Split(src, "\n")
	for _, l := range lines[:c.lineNos[0]-1] {
		if strings.TrimSpace(l) != "" {
			return false
		}
	}
	return true
}

// bodyAfterComment returns the trimmed Markdown text of src following c's
// last line, verbatim - not dedented, since this is the file's real
// top-level content, not comment-indented text.
func bodyAfterComment(src string, c rawComment) string {
	lines := strings.Split(src, "\n")
	return trimBlock(join(lines[c.lineNos[len(c.lineNos)-1]:]))
}
