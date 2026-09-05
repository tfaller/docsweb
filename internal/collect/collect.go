// Package collect walks a scope's source tree, extracts docsweb blocks via
// internal/annotation, and builds a validated model.Target registry -
// enforcing that a target name is unique within its scope.
package collect

// @docsweb
// @define collect v0.6.0
// @name Collect
// @summary
// Walks a scope's source tree, extracts docsweb blocks, and builds a
// validated Target registry - enforcing that a target name is unique
// within its scope.
// @uses annotation@v0.2.0
// @uses ignore@v0.1.0
// @uses model@v0.3.0
// @audience dev
// @changelog
// New exported `ParseFile(name, src string) ([]annotation.TargetDoc, error)`
// factors `AddScope`'s by-extension dispatch (Markdown files via
// `annotation.ParseMarkdownSource`, anything else via `annotation.ParseSource`)
// out into a reusable function, fixing a real bug: [check](@link:check@v0.7.0)'s
// version-bump check and [history](@link:history@v0.2.0)'s historic-version
// discovery both re-parsed a file's *older* git revision by calling
// `annotation.ParseSource` directly, unconditionally - so a Markdown-defined
// target's past content was silently reconstructed with an empty `Doc`
// (never reading the file's body the way `ParseMarkdownSource` does),
// causing false "documentation changed but the version wasn't bumped"
// failures for any unchanged Markdown target (this repo's own README.md
// included) and silently empty historic pages for one that did change. Both
// callers now go through `ParseFile` instead, the same way `AddScope`
// always has.
// @doc
// # Collect
//
// `AddScope` walks `Options.Root` (an `fs.FS` - a local scope's own OS
// directory wrapped in `os.DirFS`, or a remote scope's git tree straight
// from [vcs.OpenScope](@link:vcs@v0.4.0), with no worktree checkout
// involved either way) recursively, skips `.git`, whatever
// `Options.Exclude` names (other scopes' subtrees, so a shared root walk
// never double-scans them) and whatever `Options.Ignore` matches, then
// hands every remaining file's contents to `ParseFile`, which dispatches by
// extension to either [annotation](@link:annotation@v0.1.0)'s
// [block grammar](@link:annotation@v0.1.0#grammar) or, for a `.md`/
// `.markdown` file, its Markdown frontend. Non-source-looking extensions
// (images, archives, binaries) are skipped outright without being read.
// `ParseFile` is exported so a caller re-parsing a single file outside of a
// full `AddScope` walk - e.g. an older git revision of a file, to diff
// against the current one - dispatches exactly the same way a live
// collection would.
//
// Every `annotation.TargetDoc` that comes back is converted from raw
// strings into validated [model](@link:model@v0.1.0) types by `ToTarget`:
// versions are parsed, `@define`'s name is resolved via
// `model.ParseDefineName` against the scope being walked (`Options.Scope`)
// - see [model](@link:model@v0.1.0)'s three-form grammar - `@uses`
// references are resolved against the resulting target's own scope (so a
// scope-less `@uses` defaults to its own, possibly further-qualified,
// scope), and audience lists are validated. `ToTarget` is exported for
// callers that need this same conversion outside of a registry walk - e.g.
// diffing an old git revision of a file. Defining the same target name
// twice within one scope - even across two different files - is a hard
// error, reported with both files that tried to define it.
//
// A `Registry` accumulates targets across as many `AddScope` calls as a
// build needs (one root scope plus any number of declared referenced scopes),
// keyed by `scope.name`, and preserves first-discovered order for
// deterministic output.
// @docsweb

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/tfaller/docsweb/internal/annotation"
	"github.com/tfaller/docsweb/internal/ignore"
	"github.com/tfaller/docsweb/internal/model"
)

// Options describes one scope to scan.
type Options struct {
	// Scope is the dot-joined scope name assigned to every target found
	// under Root ("" for the root/unscoped tree).
	Scope string
	// Root is the file system to scan, recursively - a local scope wraps
	// its OS directory in os.DirFS, while a remote (git) scope's tree
	// comes straight from vcs.OpenScope, with no on-disk checkout at all.
	Root fs.FS
	// Exclude lists subdirectories (slash-separated, relative to Root)
	// that belong to other, separately-declared scopes (or to build
	// output) and must not be scanned as part of this one.
	Exclude []string
	// Ignore, if set, is matched against every file/directory found under
	// Root (as a path relative to Root itself) and excludes whatever it
	// matches, same as a real .gitignore would. Each scope is walked with
	// only its own .docsweb.yaml's ignore: rules - a referenced scope's
	// tree is never filtered by another scope's rules.
	Ignore *ignore.Matcher
}

// Registry accumulates targets discovered across one or more scopes.
type Registry struct {
	targets map[string]*model.Target
	order   []string
}

// NewRegistry creates an empty target registry.
func NewRegistry() *Registry {
	return &Registry{targets: make(map[string]*model.Target)}
}

// Targets returns every collected target, in first-discovered order.
func (r *Registry) Targets() []*model.Target {
	out := make([]*model.Target, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, r.targets[k])
	}
	return out
}

// Get looks up a target by its "scope.name" key (see model.Target.Key).
func (r *Registry) Get(key string) (*model.Target, bool) {
	t, ok := r.targets[key]
	return t, ok
}

// AddScope walks opts.Root, parses every file's docsweb blocks, and adds the
// resulting targets to the registry under opts.Scope. Returns an error if a
// file's annotations are malformed, a reference fails to parse, or a target
// name collides with one already in the same scope.
func (r *Registry) AddScope(opts Options) error {
	excluded := make([]string, 0, len(opts.Exclude))
	for _, e := range opts.Exclude {
		excluded = append(excluded, path.Clean(e))
	}

	return fs.WalkDir(opts.Root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if p != "." && (d.Name() == ".git" || isExcluded(p, excluded) || isIgnored(opts.Ignore, p, true)) {
				return fs.SkipDir
			}
			return nil
		}

		if isExcluded(p, excluded) || isIgnored(opts.Ignore, p, false) {
			return nil
		}
		if d.Name() == ".docsweb.yaml" || !isProbablySource(d.Name()) {
			return nil
		}

		src, err := fs.ReadFile(opts.Root, p)
		if err != nil {
			return fmt.Errorf("reading %s: %w", p, err)
		}

		docs, err := ParseFile(p, string(src))
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		for _, doc := range docs {
			if err := r.addTargetDoc(opts.Scope, p, doc); err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}
		}
		return nil
	})
}

// ParseFile parses a file's content into zero or more TargetDocs, dispatching
// by name's extension exactly like AddScope does: a Markdown file (.md/
// .markdown) is parsed via annotation.ParseMarkdownSource (at most one
// target, from a single leading @docsweb comment - an empty result, not an
// error, if the file has none), anything else via annotation.ParseSource
// (any number of @docsweb blocks via the regular comment-block grammar).
// Exported so a caller re-parsing a single file outside of a full AddScope
// walk - e.g. an older git revision of a file, to diff against the current
// one - dispatches exactly the same way a live collection would, rather than
// always assuming the non-Markdown grammar.
func ParseFile(name string, src string) ([]annotation.TargetDoc, error) {
	if isMarkdownFile(name) {
		doc, err := annotation.ParseMarkdownSource(src)
		if err != nil {
			return nil, err
		}
		if doc == nil {
			return nil, nil
		}
		return []annotation.TargetDoc{*doc}, nil
	}
	return annotation.ParseSource(src)
}

func isExcluded(p string, excluded []string) bool {
	for _, e := range excluded {
		if p == e || strings.HasPrefix(p, e+"/") {
			return true
		}
	}
	return false
}

func isIgnored(m *ignore.Matcher, p string, isDir bool) bool {
	if m == nil {
		return false
	}
	return m.Match(p, isDir)
}

// isProbablySource reports whether a file is worth scanning for docsweb
// annotations. Binary/generated/vendored artifacts are skipped outright.
func isProbablySource(name string) bool {
	switch path.Ext(name) {
	case ".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg", ".woff", ".woff2",
		".exe", ".bin", ".zip", ".tar", ".gz", ".pdf":
		return false
	}
	return true
}

// isMarkdownFile reports whether a file should be parsed via the Markdown
// frontend (annotation.ParseMarkdownSource) instead of the regular
// comment-block grammar (annotation.ParseSource).
func isMarkdownFile(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

func (r *Registry) addTargetDoc(configScope, file string, doc annotation.TargetDoc) error {
	t, err := ToTarget(configScope, file, doc)
	if err != nil {
		return err
	}

	key := t.Key()
	if existing, ok := r.targets[key]; ok {
		return fmt.Errorf("target %q already defined in scope %q (first in %s, again in %s)",
			t.Name, t.Scope, strings.Join(existing.SourceFiles, ", "), file)
	}

	r.targets[key] = t
	r.order = append(r.order, key)
	return nil
}

// ToTarget converts a single file's parsed annotation.TargetDoc into a
// validated model.Target: its @define name is resolved against configScope
// (see model.ParseDefineName), its version and every @uses reference are
// parsed, and its audience lists are validated. Exported so a caller that
// needs a one-off conversion outside of a full AddScope walk - e.g.
// re-parsing an old revision of a file to diff against the current one -
// can reuse the exact same validation a normal scope collection applies,
// without going through a Registry.
func ToTarget(configScope, file string, doc annotation.TargetDoc) (*model.Target, error) {
	scope, name, err := model.ParseDefineName(doc.Name, configScope)
	if err != nil {
		return nil, fmt.Errorf("target %q: %w", doc.Name, err)
	}
	version, err := model.ParseVersion(doc.VersionRaw)
	if err != nil {
		return nil, fmt.Errorf("target %q: %w", doc.Name, err)
	}

	uses := make([]model.TargetRef, 0, len(doc.UsesRaw))
	for _, raw := range doc.UsesRaw {
		ref, err := model.ParseTargetRef(raw, scope)
		if err != nil {
			return nil, fmt.Errorf("target %q: @uses %q: %w", doc.Name, raw, err)
		}
		uses = append(uses, ref)
	}

	audiences, err := parseAudienceList(doc.AudienceRaw)
	if err != nil {
		return nil, fmt.Errorf("target %q: %w", doc.Name, err)
	}

	changelog := make([]model.ChangelogEntry, 0, len(doc.Changelog))
	for _, c := range doc.Changelog {
		var aud []model.Audience
		if c.AudienceRaw != "" {
			aud, err = model.ParseAudiences(c.AudienceRaw)
			if err != nil {
				return nil, fmt.Errorf("target %q: @changelog @audience: %w", doc.Name, err)
			}
		}
		changelog = append(changelog, model.ChangelogEntry{Audiences: aud, Body: c.Body})
	}

	return &model.Target{
		Scope:       scope,
		ConfigScope: configScope,
		Name:        name,
		Version:     version,
		DisplayName: doc.DisplayName,
		Summary:     doc.Summary,
		Uses:        uses,
		Audiences:   audiences,
		Changelog:   changelog,
		Doc:         doc.Doc,
		SourceFiles: []string{file},
		DefineLine:  doc.DefineLine,
	}, nil
}

func parseAudienceList(raws []string) ([]model.Audience, error) {
	var out []model.Audience
	for _, raw := range raws {
		a, err := model.ParseAudiences(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, a...)
	}
	return out, nil
}

// SortedKeys returns every target key in the registry, sorted, useful for
// deterministic iteration in callers (e.g. rendering).
func (r *Registry) SortedKeys() []string {
	keys := make([]string, 0, len(r.targets))
	for k := range r.targets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
