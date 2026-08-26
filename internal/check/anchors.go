package check

import (
	"fmt"

	"github.com/tfaller/docsweb/internal/mdlink"
	"github.com/tfaller/docsweb/internal/model"
)

// checkAnchors collects every target's declared anchors up front, across
// all of its Markdown pieces, so a later @link:...#anchor can be validated
// and resolved regardless of definition order across targets/files.
// Populates ctx.anchors.
func checkAnchors(ctx *context) error {
	out := make(map[string]map[string]bool)
	for _, t := range ctx.registry.Targets() {
		set := map[string]bool{}
		for _, p := range targetPieces(t) {
			names, err := mdlink.CollectAnchors(p)
			if err != nil {
				return fmt.Errorf("%s: %w", t.Key(), err)
			}
			for _, n := range names {
				if set[n] {
					return fmt.Errorf("%s: duplicate anchor %q", t.Key(), n)
				}
				set[n] = true
			}
		}
		out[t.Key()] = set
	}
	ctx.anchors = out
	return nil
}

// targetPieces returns every piece of Markdown a target carries - its
// @summary, @doc, and every @changelog entry's body - in the order anchor
// uniqueness and link resolution are checked across them.
func targetPieces(t *model.Target) []string {
	pieces := make([]string, 0, 2+len(t.Changelog))
	pieces = append(pieces, t.Summary, t.Doc)
	for _, c := range t.Changelog {
		pieces = append(pieces, c.Body)
	}
	return pieces
}
