package check

import (
	"fmt"

	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/mdlink"
	"github.com/tfaller/docsweb/internal/model"
)

// checkLinks validates that every @link reference (and its optional
// #anchor) in every target's Markdown resolves to something real, by
// running mdlink.Preprocess - the same resolution a render pass would do -
// and discarding its output. This is the one check that would otherwise
// require actually rendering a page; running Preprocess alone is what lets
// "docsweb check" confirm every target is in shape to render correctly
// without ever converting a single piece of Markdown to HTML.
func checkLinks(ctx *context) error {
	resolver := &linkResolver{reg: ctx.registry, anchors: ctx.anchors}
	for _, t := range ctx.registry.Targets() {
		for _, p := range targetPieces(t) {
			if _, err := mdlink.Preprocess(p, t.Scope, resolver); err != nil {
				return fmt.Errorf("%s: %w", t.Key(), err)
			}
		}
	}
	return nil
}

// linkResolver implements mdlink.Resolver purely for validation: it only
// needs to report whether a target/anchor exists, never a real page URL,
// since checkLinks always discards Preprocess's output.
type linkResolver struct {
	reg     *collect.Registry
	anchors map[string]map[string]bool
}

func (r *linkResolver) ResolveTarget(ref model.TargetRef) (string, bool) {
	_, ok := r.reg.Get(ref.Key())
	return "", ok
}

func (r *linkResolver) HasAnchor(ref model.TargetRef, anchor string) bool {
	return r.anchors[ref.Key()][anchor]
}
