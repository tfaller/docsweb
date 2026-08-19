package annotation

import (
	"fmt"
	"strings"
)

// tagNames are the fixed docsweb tag names, in the order they're documented
// in README.md. "docsweb" is deliberately excluded: block-boundary markers
// are consumed by extractBlocksFromComment and never reach section parsing.
var tagNames = []string{"define", "name", "summary", "uses", "audience", "changelog", "doc"}

// rawSection is one mechanically-parsed tag section: a tag line plus every
// following line that isn't itself a recognized tag line (and isn't inside
// a fenced code block).
type rawSection struct {
	Tag   string
	Lines []string
}

// ChangelogEntry is one raw (unvalidated) @changelog section.
type ChangelogEntry struct {
	// AudienceRaw is the raw comma-separated @audience override for this
	// entry, or "" if none was given (whole target audience applies).
	AudienceRaw string
	Body        string
}

// TargetDoc is a single file's fully-merged view of one target: the result
// of concatenating every docsweb block in the file that defines or
// continues it. Fields are still raw strings - annotation does not know
// about semver or scopes, so validation/parsing of versions and references
// happens in the collect package.
type TargetDoc struct {
	Name        string
	VersionRaw  string
	DisplayName string
	Summary     string
	UsesRaw     []string
	AudienceRaw []string
	Changelog   []ChangelogEntry
	Doc         string
}

// ParseSource extracts and merges every docsweb target defined in a single
// file's source text, in the order they're defined.
func ParseSource(src string) ([]TargetDoc, error) {
	var blocks [][]string
	for _, c := range findComments(src) {
		blocks = append(blocks, extractBlocksFromComment(c)...)
	}

	var targets []TargetDoc
	for bi, blockLines := range blocks {
		sections := parseSections(blockLines)
		bd, err := semanticize(sections)
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", bi+1, err)
		}

		if bd.hasDefine {
			targets = append(targets, TargetDoc{
				Name:        bd.name,
				VersionRaw:  bd.versionRaw,
				DisplayName: bd.displayName,
				Summary:     bd.summary,
				UsesRaw:     bd.usesRaw,
				AudienceRaw: bd.audienceRaw,
				Changelog:   bd.changelog,
				Doc:         bd.doc,
			})
			continue
		}

		if len(targets) == 0 {
			return nil, fmt.Errorf("block %d: first docsweb block in a file must @define a target", bi+1)
		}
		t := &targets[len(targets)-1]
		t.DisplayName = joinNonEmpty(t.DisplayName, bd.displayName)
		t.Summary = joinNonEmpty(t.Summary, bd.summary)
		t.Doc = joinNonEmpty(t.Doc, bd.doc)
		t.UsesRaw = append(t.UsesRaw, bd.usesRaw...)
		t.AudienceRaw = append(t.AudienceRaw, bd.audienceRaw...)
		t.Changelog = append(t.Changelog, bd.changelog...)
	}

	return targets, nil
}

// extractBlocksFromComment splits one comment's content lines into the
// (dedented) line sets of every @docsweb ... @docsweb block it contains. A
// block left unclosed simply runs to the end of the comment.
func extractBlocksFromComment(c rawComment) [][]string {
	var blocks [][]string
	var cur []string
	inBlock := false
	inFence := false

	for _, l := range c.lines {
		if strings.HasPrefix(strings.TrimLeft(l, " \t"), "```") {
			inFence = !inFence
			if inBlock {
				cur = append(cur, l)
			}
			continue
		}
		if !inFence && strings.TrimSpace(l) == "@docsweb" {
			if inBlock {
				blocks = append(blocks, dedent(cur))
				cur = nil
				inBlock = false
			} else {
				inBlock = true
				cur = nil
			}
			continue
		}
		if inBlock {
			cur = append(cur, l)
		}
	}
	if inBlock {
		blocks = append(blocks, dedent(cur))
	}
	return blocks
}

// dedent strips the common leading whitespace of all non-blank lines,
// exactly once, per README.md's indentation rule.
func dedent(lines []string) []string {
	min := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if min == -1 || n < min {
			min = n
		}
	}
	if min <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if len(l) >= min {
			out[i] = l[min:]
		} else {
			out[i] = strings.TrimLeft(l, " \t")
		}
	}
	return out
}

// parseSections mechanically splits a dedented block's lines into tag
// sections: a line starting with a recognized tag token opens a new
// section; every other line (outside a fenced code block) belongs to
// whichever section was opened last.
func parseSections(lines []string) []rawSection {
	var sections []rawSection
	inFence := false

	for _, l := range lines {
		trimmedLeft := strings.TrimLeft(l, " \t")
		if strings.HasPrefix(trimmedLeft, "```") {
			inFence = !inFence
			appendLine(&sections, l)
			continue
		}
		if !inFence {
			if tag, rest, ok := matchTag(l); ok {
				sections = append(sections, rawSection{Tag: tag, Lines: []string{rest}})
				continue
			}
		}
		appendLine(&sections, l)
	}
	return sections
}

func appendLine(sections *[]rawSection, l string) {
	if len(*sections) == 0 {
		// Content before any recognized tag has nothing to attach to;
		// silently discarded (malformed/empty preamble).
		return
	}
	last := &(*sections)[len(*sections)-1]
	last.Lines = append(last.Lines, l)
}

func matchTag(l string) (tag, rest string, ok bool) {
	if !strings.HasPrefix(l, "@") {
		return "", "", false
	}
	for _, name := range tagNames {
		token := "@" + name
		if l == token {
			return name, "", true
		}
		if strings.HasPrefix(l, token+" ") || strings.HasPrefix(l, token+"\t") {
			return name, strings.TrimPrefix(l[len(token):], " "), true
		}
	}
	return "", "", false
}

// blockData is the semantic (but still raw/unvalidated) result of one
// docsweb block.
type blockData struct {
	hasDefine   bool
	name        string
	versionRaw  string
	displayName string
	summary     string
	usesRaw     []string
	audienceRaw []string
	changelog   []ChangelogEntry
	doc         string
}

// semanticize walks a block's mechanically-parsed sections and applies the
// tag-specific rules from README.md - most importantly the @changelog
// "optionally followed by its own @audience override" carve-out, where
// content after that override line still belongs to the changelog body
// rather than to the audience line, per the "Changelog definition" section.
func semanticize(sections []rawSection) (blockData, error) {
	var bd blockData
	defineSeen := false

	i := 0
	for i < len(sections) {
		s := sections[i]
		switch s.Tag {
		case "define":
			if defineSeen {
				return blockData{}, fmt.Errorf("multiple @define tags in one docsweb block")
			}
			defineSeen = true
			bd.hasDefine = true
			name, version, err := splitDefine(join(s.Lines))
			if err != nil {
				return blockData{}, err
			}
			bd.name, bd.versionRaw = name, version
			i++
		case "name":
			bd.displayName = joinNonEmpty(bd.displayName, trimBlock(join(s.Lines)))
			i++
		case "summary":
			bd.summary = joinNonEmpty(bd.summary, trimBlock(join(s.Lines)))
			i++
		case "uses":
			ref := strings.TrimSpace(join(s.Lines))
			if ref != "" {
				bd.usesRaw = append(bd.usesRaw, ref)
			}
			i++
		case "audience":
			a := strings.TrimSpace(join(s.Lines))
			if a != "" {
				bd.audienceRaw = append(bd.audienceRaw, a)
			}
			i++
		case "changelog":
			entry, next := buildChangelogEntry(sections, i)
			bd.changelog = append(bd.changelog, entry)
			i = next
		case "doc":
			bd.doc = joinNonEmpty(bd.doc, trimBlock(join(s.Lines)))
			i++
		default:
			i++
		}
	}

	return bd, nil
}

// buildChangelogEntry consumes the @changelog section at sections[i] and,
// if immediately followed by an @audience section, reattributes that
// section's first line as the audience override and everything after it
// back to the changelog body - see semanticize's doc comment.
func buildChangelogEntry(sections []rawSection, i int) (ChangelogEntry, int) {
	body := append([]string{}, sections[i].Lines...)
	audienceRaw := ""
	next := i + 1

	if next < len(sections) && sections[next].Tag == "audience" {
		lines := sections[next].Lines
		if len(lines) > 0 {
			audienceRaw = strings.TrimSpace(lines[0])
			body = append(body, lines[1:]...)
		}
		next++
	}

	return ChangelogEntry{AudienceRaw: audienceRaw, Body: trimBlock(join(body))}, next
}

func splitDefine(content string) (name, version string, err error) {
	fields := strings.Fields(content)
	if len(fields) != 2 {
		return "", "", fmt.Errorf("invalid @define %q: expected \"name vX.Y.Z\"", content)
	}
	return fields[0], fields[1], nil
}

func join(lines []string) string {
	return strings.Join(lines, "\n")
}

func joinNonEmpty(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "\n\n" + b
}

// trimBlock drops leading/trailing fully-blank lines while preserving
// interior blank lines (Markdown paragraph breaks) and interior
// indentation.
func trimBlock(s string) string {
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}
