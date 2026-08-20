// Package collect walks a scope's source tree, extracts docsweb blocks via
// internal/annotation, and builds a validated model.Target registry -
// enforcing that a target name is unique within its scope.
package collect

// @docsweb
// @define collect v0.1.0
// @name Collect
// @summary
// Walks a scope's source tree, extracts docsweb blocks, and builds a
// validated Target registry - enforcing that a target name is unique
// within its scope.
// @uses annotation@v0.1.0
// @uses ignore@v0.1.0
// @uses model@v0.1.0
// @audience dev
// @changelog
// Initial documentation.
// @doc
// # Collect
//
// `AddScope` walks a directory recursively, skips `.git`, whatever
// `Options.Exclude` names (other scopes' subtrees, so a shared root walk
// never double-scans them) and whatever `Options.Ignore` matches, then
// hands every remaining file's contents to
// [annotation](@link:annotation@v0.1.0)'s
// [block grammar](@link:annotation@v0.1.0#grammar). Non-source-looking
// extensions (images, archives, binaries) are skipped outright without
// being read.
//
// Every `annotation.TargetDoc` that comes back is converted from raw
// strings into validated [model](@link:model@v0.1.0) types here: versions
// are parsed, `@uses` references are resolved against the scope being
// walked (so a scope-less `@uses` defaults to its own scope), and
// audience lists are validated. Defining the same target name twice
// within one scope - even across two different files - is a hard error,
// reported with both files that tried to define it.
//
// A `Registry` accumulates targets across as many `AddScope` calls as a
// build needs (one root scope plus any number of declared sub-scopes),
// keyed by `scope.name`, and preserves first-discovered order for
// deterministic output.
// @docsweb

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	// Root is the filesystem directory to scan, recursively.
	Root string
	// Exclude lists subdirectories (relative to Root, or absolute) that
	// belong to other, separately-declared scopes (or to build output)
	// and must not be scanned as part of this one.
	Exclude []string
	// Ignore, if set, is matched against every file/directory found under
	// Root (as a path relative to IgnoreBase) and excludes whatever it
	// matches, same as a real .gitignore would.
	Ignore *ignore.Matcher
	// IgnoreBase is the directory Ignore's patterns are relative to.
	// Defaults to Root when empty, so a single scope can be scanned
	// standalone (e.g. in tests) without also having to set this.
	IgnoreBase string
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
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return fmt.Errorf("scope %q: %w", opts.Scope, err)
	}

	ignoreBase := root
	if opts.IgnoreBase != "" {
		ignoreBase, err = filepath.Abs(opts.IgnoreBase)
		if err != nil {
			return fmt.Errorf("scope %q: %w", opts.Scope, err)
		}
	}

	excluded := make([]string, 0, len(opts.Exclude))
	for _, e := range opts.Exclude {
		abs := e
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, e)
		}
		excluded = append(excluded, filepath.Clean(abs))
	}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		clean := filepath.Clean(path)

		if d.IsDir() {
			if clean != root && (d.Name() == ".git" || isExcluded(clean, excluded) || isIgnored(opts.Ignore, ignoreBase, clean, true)) {
				return filepath.SkipDir
			}
			return nil
		}

		if isExcluded(clean, excluded) || isIgnored(opts.Ignore, ignoreBase, clean, false) {
			return nil
		}
		if d.Name() == ".docsweb.yaml" || !isProbablySource(d.Name()) {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", rel, err)
		}

		docs, err := annotation.ParseSource(string(src))
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		for _, doc := range docs {
			if err := r.addTargetDoc(opts.Scope, rel, doc); err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
		}
		return nil
	})
}

func isExcluded(path string, excluded []string) bool {
	for _, e := range excluded {
		if path == e || strings.HasPrefix(path, e+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func isIgnored(m *ignore.Matcher, base, path string, isDir bool) bool {
	if m == nil {
		return false
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return m.Match(filepath.ToSlash(rel), isDir)
}

// isProbablySource reports whether a file is worth scanning for docsweb
// annotations. Binary/generated/vendored artifacts are skipped outright.
func isProbablySource(name string) bool {
	switch filepath.Ext(name) {
	case ".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg", ".woff", ".woff2",
		".exe", ".bin", ".zip", ".tar", ".gz", ".pdf":
		return false
	}
	return true
}

func (r *Registry) addTargetDoc(scope, file string, doc annotation.TargetDoc) error {
	if !model.ValidName(doc.Name) {
		return fmt.Errorf("target name %q is not alphanumeric", doc.Name)
	}
	version, err := model.ParseVersion(doc.VersionRaw)
	if err != nil {
		return fmt.Errorf("target %q: %w", doc.Name, err)
	}

	uses := make([]model.TargetRef, 0, len(doc.UsesRaw))
	for _, raw := range doc.UsesRaw {
		ref, err := model.ParseTargetRef(raw, scope)
		if err != nil {
			return fmt.Errorf("target %q: @uses %q: %w", doc.Name, raw, err)
		}
		uses = append(uses, ref)
	}

	audiences, err := parseAudienceList(doc.AudienceRaw)
	if err != nil {
		return fmt.Errorf("target %q: %w", doc.Name, err)
	}

	changelog := make([]model.ChangelogEntry, 0, len(doc.Changelog))
	for _, c := range doc.Changelog {
		var aud []model.Audience
		if c.AudienceRaw != "" {
			aud, err = model.ParseAudiences(c.AudienceRaw)
			if err != nil {
				return fmt.Errorf("target %q: @changelog @audience: %w", doc.Name, err)
			}
		}
		changelog = append(changelog, model.ChangelogEntry{Audiences: aud, Body: c.Body})
	}

	key := doc.Name
	if scope != "" {
		key = scope + "." + doc.Name
	}

	if existing, ok := r.targets[key]; ok {
		return fmt.Errorf("target %q already defined in scope %q (first in %s, again in %s)",
			doc.Name, scope, strings.Join(existing.SourceFiles, ", "), file)
	}

	t := &model.Target{
		Scope:       scope,
		Name:        doc.Name,
		Version:     version,
		DisplayName: doc.DisplayName,
		Summary:     doc.Summary,
		Uses:        uses,
		Audiences:   audiences,
		Changelog:   changelog,
		Doc:         doc.Doc,
		SourceFiles: []string{file},
	}
	r.targets[key] = t
	r.order = append(r.order, key)
	return nil
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
