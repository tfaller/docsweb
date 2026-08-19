package build

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/config"
	"github.com/tfaller/docsweb/internal/ignore"
	"github.com/tfaller/docsweb/internal/mdlink"
	"github.com/tfaller/docsweb/internal/model"
)

// Options configures a full docsweb build.
type Options struct {
	// ConfigPath is the path to the root .docsweb.yaml. Its directory is
	// the root scope's file tree; scopes it declares are read relative to
	// that directory, per README.md's "Scopes" section.
	ConfigPath string
	// RootScope names the root scope itself (the scope files directly
	// under the config's directory belong to, outside any declared
	// sub-scope). "" is a valid, and the default, root scope name.
	RootScope string
}

// ChangelogHTML is one target's @changelog entry, rendered.
type ChangelogHTML struct {
	Audiences []model.Audience
	HTML      string
}

// RenderedTarget pairs a parsed target with its Markdown pieces rendered to
// HTML (with @anchor:/@link: already resolved).
type RenderedTarget struct {
	Target        *model.Target
	SummaryHTML   string
	DocHTML       string
	ChangelogHTML []ChangelogHTML
}

// Result is everything internal/site needs to render the static output.
type Result struct {
	Targets []RenderedTarget
	Issues  []UsageIssue
}

// TargetURL returns the site-relative URL (from the output root) of ref's
// page: dot-joined scope segments become path segments, e.g. scope "a.b",
// name "c" -> "a/b/c.html"; the root scope ("") -> "c.html". There is
// exactly one page per target (its current version), since the POC has no
// version history to page through.
func TargetURL(ref model.TargetRef) string {
	if ref.Scope == "" {
		return ref.Name + ".html"
	}
	return strings.ReplaceAll(ref.Scope, ".", "/") + "/" + ref.Name + ".html"
}

// Run executes a full build: load config, collect every scope's targets,
// validate & classify @uses references, resolve @anchor:/@link:
// destinations, and render every target's Markdown to HTML.
func Run(opts Options) (*Result, error) {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	rootDir, err := filepath.Abs(filepath.Dir(opts.ConfigPath))
	if err != nil {
		return nil, err
	}

	reg := collect.NewRegistry()
	matcher := ignore.Compile(cfg.Ignore)

	excludes := make([]string, 0, len(cfg.Scopes))
	for name, sc := range cfg.Scopes {
		if sc.Remote() {
			return nil, fmt.Errorf("scope %q: remote scopes are not supported by the POC (see README.md \"After POC\")", name)
		}
		excludes = append(excludes, filepath.Join(rootDir, sc.Path))
	}
	if err := reg.AddScope(collect.Options{Scope: opts.RootScope, Root: rootDir, Exclude: excludes, Ignore: matcher, IgnoreBase: rootDir}); err != nil {
		return nil, err
	}
	for name, sc := range cfg.Scopes {
		if err := reg.AddScope(collect.Options{Scope: name, Root: filepath.Join(rootDir, sc.Path), Ignore: matcher, IgnoreBase: rootDir}); err != nil {
			return nil, err
		}
	}

	if err := remapScopeAudiences(cfg, reg, opts.RootScope); err != nil {
		return nil, err
	}

	issues, err := ResolveUses(reg)
	if err != nil {
		return nil, err
	}

	anchors, err := collectAllAnchors(reg)
	if err != nil {
		return nil, err
	}
	resolver := &registryResolver{reg: reg, anchors: anchors}

	rendered := make([]RenderedTarget, 0, len(reg.Targets()))
	for _, t := range reg.Targets() {
		rt := RenderedTarget{Target: t}

		rt.SummaryHTML, err = mdlink.RenderDoc(t.Summary, t.Scope, resolver)
		if err != nil {
			return nil, fmt.Errorf("%s: @summary: %w", t.Key(), err)
		}
		rt.DocHTML, err = mdlink.RenderDoc(t.Doc, t.Scope, resolver)
		if err != nil {
			return nil, fmt.Errorf("%s: @doc: %w", t.Key(), err)
		}
		for _, c := range t.Changelog {
			html, err := mdlink.RenderDoc(c.Body, t.Scope, resolver)
			if err != nil {
				return nil, fmt.Errorf("%s: @changelog: %w", t.Key(), err)
			}
			rt.ChangelogHTML = append(rt.ChangelogHTML, ChangelogHTML{Audiences: c.Audiences, HTML: html})
		}

		rendered = append(rendered, rt)
	}

	return &Result{Targets: rendered, Issues: issues}, nil
}

// collectAllAnchors gathers every target's declared anchors up front, across
// all of its Markdown pieces, so @link:...#anchor fragments can be validated
// and resolved regardless of definition order across targets/files.
func collectAllAnchors(reg *collect.Registry) (map[string]map[string]bool, error) {
	out := make(map[string]map[string]bool)
	for _, t := range reg.Targets() {
		set := map[string]bool{}
		pieces := append([]string{t.Summary, t.Doc}, changelogBodies(t)...)
		for _, p := range pieces {
			names, err := mdlink.CollectAnchors(p)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", t.Key(), err)
			}
			for _, n := range names {
				if set[n] {
					return nil, fmt.Errorf("%s: duplicate anchor %q", t.Key(), n)
				}
				set[n] = true
			}
		}
		out[t.Key()] = set
	}
	return out, nil
}

func changelogBodies(t *model.Target) []string {
	out := make([]string, len(t.Changelog))
	for i, c := range t.Changelog {
		out[i] = c.Body
	}
	return out
}

// registryResolver implements mdlink.Resolver against a collected target
// registry and its precomputed anchor sets.
type registryResolver struct {
	reg     *collect.Registry
	anchors map[string]map[string]bool
}

func (r *registryResolver) ResolveTarget(ref model.TargetRef) (string, bool) {
	if _, ok := r.reg.Get(ref.Key()); !ok {
		return "", false
	}
	return TargetURL(ref), true
}

func (r *registryResolver) HasAnchor(ref model.TargetRef, anchor string) bool {
	return r.anchors[ref.Key()][anchor]
}
