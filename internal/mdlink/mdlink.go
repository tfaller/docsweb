// Package mdlink implements the @anchor:/@link: preprocessing step described
// in README.md's "Linking between documentations" section, plus a thin
// goldmark rendering wrapper. It operates purely on text: a markdown link
// [label](@anchor:name) or [label](@link:scope.target@vX.X.X#anchor) is
// rewritten into something goldmark understands, before the surrounding
// text is ever handed to the Markdown renderer.
package mdlink

// @docsweb
// @define mdlink v0.2.0
// @name Markdown links
// @summary
// Resolves @anchor:/@link: pseudo-URLs in Markdown to real HTML before
// handing the text to a goldmark renderer, strictly or leniently.
// @uses model@v0.3.0
// @audience dev
// @changelog
// New lenient variants, `PreprocessLenient`/`RenderDocLenient`, for
// rendering markdown captured at a past commit (see
// [history](@link:history@v0.1.0)): an invalid anchor name, an unparseable
// `@link`, or a `@link`/anchor the resolver can't find degrades to the
// link's plain label text instead of erroring, since a broken reference in
// old content can never be fixed after the fact the way one in current
// content can. `Preprocess`/`RenderDoc` are unchanged - still a hard error,
// exactly as before. Non-breaking.
// @doc
// # Markdown links
//
// A Markdown link whose destination is an `@anchor:` or `@link:`
// pseudo-URL is rewritten into something goldmark understands *before*
// the surrounding text is ever handed to the renderer - so the
// destinations below can be written as plain Markdown link destinations
// without any special renderer support:
//
// ```
// [label](@anchor:name)
// [label](@link:scope.target@vX.Y.Z#anchor)
// ```
//
// The first becomes an invisible `<a id="name"></a>` placed immediately
// before `label`, marking the anchor point in the flowing text without
// turning `label` itself into a dead self-link. The second becomes a
// normal `[label](url#anchor)` link once the reference is resolved.
//
// Both directions work line by line and skip fenced (```) code blocks -
// which is exactly how the pair above gets to appear on this page as a
// literal example instead of a resolved link.
//
// ## The [Resolver](@anchor:resolver)
//
// `mdlink` knows nothing about site URL layout or which targets actually
// exist - it asks a caller-supplied `Resolver` (`ResolveTarget`,
// `HasAnchor`) instead. An unparseable reference, or a `@link`/anchor the
// resolver reports as missing, is a hard error: per the project's
// pipeline rules, every `@link` must land at an existing target (and
// anchor, if one is given) before a build is allowed to succeed.
//
// `PreprocessLenient`/`RenderDocLenient` are the same resolution, but never
// error: a broken reference degrades to its plain label text instead. This
// is for rendering markdown that was never validated the way current
// content is - e.g. a historic target snapshot - where a broken `@link`
// can't be fixed after the fact and so can't be allowed to fail a build.
//
// `CollectAnchors` runs the same line-by-line scan just to gather
// `@anchor:name` declarations, so a caller can build the full set of
// known anchors across every target before resolving any `@link` against
// them - anchor and use-site can appear in either order across the
// source tree.
// @docsweb

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/tfaller/docsweb/internal/model"
)

// linkRe matches a markdown link whose destination is an @anchor: or @link:
// pseudo-URL. Destinations are never expected to contain a literal ")", so a
// simple non-greedy-free character class is enough for the POC.
var linkRe = regexp.MustCompile(`\[([^\]]*)\]\((@anchor:[^)]*|@link:[^)]*)\)`)

// Resolver resolves @link: destinations against the full set of targets
// known to the build. It is supplied by the caller (internal/build /
// internal/site), since mdlink itself knows nothing about site URL layout.
type Resolver interface {
	// ResolveTarget reports the page URL for ref, or ok=false if no such
	// target exists.
	ResolveTarget(ref model.TargetRef) (url string, ok bool)
	// HasAnchor reports whether ref's target declared an @anchor: with the
	// given name (see CollectAnchors).
	HasAnchor(ref model.TargetRef, anchor string) bool
}

// forEachNonFencedLine rewrites markdown line by line, skipping lines inside
// fenced (```) code blocks, so @anchor:/@link: syntax can be shown as a
// literal example inside documentation without being rewritten.
func forEachNonFencedLine(markdown string, fn func(line string) (string, error)) (string, error) {
	lines := strings.Split(markdown, "\n")
	inFence := false
	for i, l := range lines {
		trimmed := strings.TrimLeft(l, " \t")
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		nl, err := fn(l)
		if err != nil {
			return "", err
		}
		lines[i] = nl
	}
	return strings.Join(lines, "\n"), nil
}

// CollectAnchors scans markdown for [label](@anchor:name) declarations and
// returns the declared names, in source order. Each name is validated
// against model.ValidName, and repeats within markdown itself are rejected.
//
// A target's anchors must be unique across all of its pieces (@summary,
// @doc, @changelog entries) combined, per README.md - since mdlink only ever
// sees one piece of text at a time, the caller is responsible for checking
// uniqueness across the results of multiple CollectAnchors calls belonging
// to the same target.
func CollectAnchors(markdown string) ([]string, error) {
	seen := map[string]bool{}
	var names []string
	_, err := forEachNonFencedLine(markdown, func(line string) (string, error) {
		for _, m := range linkRe.FindAllStringSubmatch(line, -1) {
			name, ok := strings.CutPrefix(m[2], "@anchor:")
			if !ok {
				continue
			}
			if !model.ValidName(name) {
				return "", fmt.Errorf("invalid anchor name %q", name)
			}
			if seen[name] {
				return "", fmt.Errorf("duplicate anchor %q", name)
			}
			seen[name] = true
			names = append(names, name)
		}
		return line, nil
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

// Preprocess rewrites every @anchor:/@link: markdown link destination in
// markdown into output goldmark can render directly:
//
//   - [label](@anchor:name) becomes the raw HTML `<a id="name"></a>label`:
//     an empty, invisible anchor placed exactly where the declaration sits,
//     immediately followed by label as ordinary Markdown text (so any
//     nested formatting inside label still renders). This marks the anchor
//     point in the flowing text without turning label into a dead link to
//     itself.
//   - [label](@link:scope.target@vX.X.X#anchor) becomes a normal
//     [label](url#anchor) link, once ref is parsed against defaultScope and
//     resolved through resolver.
//
// An invalid anchor name, an unparseable @link reference, or an @link/
// anchor that resolver reports as nonexistent is a hard error (see
// README.md: "check that all @link and @uses land at an existing target").
func Preprocess(markdown, defaultScope string, resolver Resolver) (string, error) {
	return preprocess(markdown, defaultScope, resolver, true)
}

// PreprocessLenient is like Preprocess, but never errors: an invalid anchor
// name, an unparseable @link reference, or an @link/anchor resolver reports
// as nonexistent degrades to the link's plain label text (no anchor tag, no
// hyperlink) instead of failing. This matters for markdown captured at a
// past commit - internal/history's historic target snapshots - where a
// broken reference can never be fixed after the fact, so rendering it can't
// be allowed to fail an entire build the way it would for current content.
func PreprocessLenient(markdown, defaultScope string, resolver Resolver) string {
	out, _ := preprocess(markdown, defaultScope, resolver, false)
	return out
}

func preprocess(markdown, defaultScope string, resolver Resolver, strict bool) (string, error) {
	return forEachNonFencedLine(markdown, func(line string) (string, error) {
		var lineErr error
		out := linkRe.ReplaceAllStringFunc(line, func(match string) string {
			if lineErr != nil {
				return match
			}
			sub := linkRe.FindStringSubmatch(match)
			label, dest := sub[1], sub[2]

			fail := func(err error) string {
				if strict {
					lineErr = err
					return match
				}
				return label
			}

			switch {
			case strings.HasPrefix(dest, "@anchor:"):
				name := strings.TrimPrefix(dest, "@anchor:")
				if !model.ValidName(name) {
					return fail(fmt.Errorf("invalid anchor name %q", name))
				}
				return fmt.Sprintf(`<a id="%s"></a>%s`, name, label)

			case strings.HasPrefix(dest, "@link:"):
				refStr := strings.TrimPrefix(dest, "@link:")
				refPart, anchor, _ := strings.Cut(refStr, "#")
				ref, err := model.ParseTargetRef(refPart, defaultScope)
				if err != nil {
					return fail(fmt.Errorf("invalid @link %q: %w", refStr, err))
				}
				url, ok := resolver.ResolveTarget(ref)
				if !ok {
					return fail(fmt.Errorf("@link %q does not resolve to an existing target", refStr))
				}
				if anchor != "" {
					if !resolver.HasAnchor(ref, anchor) {
						return fail(fmt.Errorf("@link %q: target has no anchor %q", refStr, anchor))
					}
					url += "#" + anchor
				}
				return fmt.Sprintf("[%s](%s)", label, url)

			default:
				return match
			}
		})
		if lineErr != nil {
			return "", lineErr
		}
		return out, nil
	})
}

// renderer is shared across Render calls: WithUnsafe is required so the raw
// <a id="..."> anchors Preprocess injects are emitted as-is rather than
// stripped. docsweb content comes from source-controlled doc comments, not
// untrusted input, so this is an acceptable POC tradeoff.
var renderer = goldmark.New(goldmark.WithRendererOptions(html.WithUnsafe()))

// Render converts already-preprocessed markdown (see Preprocess) to HTML.
func Render(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := renderer.Convert([]byte(markdown), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderDoc runs Preprocess followed by Render, for callers that don't need
// the intermediate markdown (e.g. the final HTML-generation pass, once
// every target and its anchors are already known). Callers that need to
// validate @link references before all targets/anchors are collected should
// call Preprocess directly and discard its output.
func RenderDoc(markdown, defaultScope string, resolver Resolver) (string, error) {
	pre, err := Preprocess(markdown, defaultScope, resolver)
	if err != nil {
		return "", err
	}
	return Render(pre)
}

// RenderDocLenient is RenderDoc's PreprocessLenient counterpart: it never
// fails over an unresolved @link/anchor (see PreprocessLenient), only over a
// genuine rendering error from Render itself.
func RenderDocLenient(markdown, defaultScope string, resolver Resolver) (string, error) {
	return Render(PreprocessLenient(markdown, defaultScope, resolver))
}
