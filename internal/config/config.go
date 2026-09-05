// Package config loads and validates .docsweb.yaml scope/audience
// configuration files. See README.md's "Scopes" section for the file
// format this implements.
package config

// @docsweb
// @define config v0.3.2
// @name Config
// @summary
// Loads and validates .docsweb.yaml: a scope's own self-declared name,
// declared audiences (with combine), referenced scopes (local or remote),
// and a scope's own ignore rules.
// @uses model@v0.3.0
// @audience dev
// @changelog
// Fixed the `Ignore` bullet below: it claimed this config's `ignore:` list
// is "applied to every scope", which described
// [check](@link:check@v0.10.0)'s actual behavior at the time, but not the
// intended one - a local referenced scope's tree was filtered by the
// *root* config's `ignore:` rules (re-anchored to its subdirectory)
// instead of its own, and a remote scope's own `ignore:` list was never
// applied at all. Now that check's scope collection walks every
// referenced scope (local or remote) with only its own `.docsweb.yaml`
// `ignore:` list, this bullet describes that as the actual contract - the
// root config's list never reaches into a referenced scope's tree.
// @doc
// # Config
//
// `Load`/`LoadFS`/`Parse` turn a `.docsweb.yaml` file into a validated
// `Config`:
//
// - `Name` - the top-level `name:` key: this scope's own self-declared,
//   required, complete identity (dot-joined). Every `.docsweb.yaml` must
//   declare one - there is no implicit "unscoped" default. See the
//   README's "Scopes" section.
// - `Audiences` - the `audience:` map, each entry naming the other
//   audiences (if any) it's an umbrella over via `combine`.
//   `AudienceIncludes` walks that combine graph (cycle-safe) to decide
//   whether one audience transitively includes another.
// - `Scopes` - the `scope:` map, keyed by the *expected* name of each
//   referenced scope (`"parent.child"` is a valid key on its own, not a
//   nested structure) - `build.Run` verifies that expectation against the
//   referenced scope's own self-declared `Name` at build time. An entry
//   with `git` set is a remote scope: [check](@link:check@v0.4.0)'s scope
//   collection opens it (via [vcs.OpenScope](@link:vcs@v0.4.0), reading its
//   resolved commit's tree directly rather than checking out a worktree)
//   before verifying and walking it exactly like a local one.
// - `Ignore` - this config's own `ignore:` list, handed to
//   [ignore](@link:ignore@v0.1.0) and applied only to the scope that
//   declares it: [check](@link:check@v0.10.0)'s scope collection walks
//   each scope (root or referenced, local or remote) with only its own
//   `.docsweb.yaml`'s `Ignore`, never a parent's - see check's "scopes"
//   check.
//
// Duplicate audience or scope names are rejected for free, since
// `yaml.v3` errors on a mapping with a repeated key rather than silently
// keeping the last one.
//
// ## Referenced-scope [audience mapping](@anchor:audiencemap)
//
// `ResolveScopeAudience` implements the README's "Scopes" mapping rule:
// an audience name that matches one declared in this config auto-maps to
// itself; anything else must appear in that scope's own `audienceMap`, or
// resolution fails. This is applied to every non-root-scope target's
// `@audience` names, including changelog-entry overrides, during a build.
// @docsweb

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/tfaller/docsweb/internal/model"
	"gopkg.in/yaml.v3"
)

// Audience is one entry from a config's `audience:` map: a declared
// audience name, and the other audiences (if any) it's an umbrella over.
type Audience struct {
	Name    model.Audience
	Combine []model.Audience
}

// Scope is one entry from a config's `scope:` map. The map key itself is
// the scope's full name (dot-joined for nested names, e.g. "parent.child")
// - see PLAN.md assumption #2. An entry with Git set is a remote scope;
// internal/check's scope collection opens it (see internal/vcs's
// OpenScope) before walking it like any other referenced scope.
type Scope struct {
	// Name is the scope's full dot-joined name, exactly as written as the
	// scope map's key.
	Name string
	// Path is relative: to the config file's directory for a local scope,
	// or to the repository root for a remote scope.
	Path string
	// Git is the remote repository URL. Empty for a local scope.
	Git string
	// Ref is the git branch/ref to use. Only meaningful when Git != "".
	Ref string
	// AudienceMap maps this scope's own local audience names to this
	// config's audience names, for audiences that don't auto-map by
	// identical name (see Config.ResolveScopeAudience).
	AudienceMap map[model.Audience]model.Audience
}

// Remote reports whether s is a remote (git-based) scope.
func (s Scope) Remote() bool { return s.Git != "" }

// normalizeScopePath turns a human-written scope path - which may use any
// of ".", "./", "/", "someDir/", "./someDir/", "/someDir", "./someDir", ...
// to mean the same thing - into the canonical form the rest of docsweb
// expects: "" for the path's own root, otherwise a slash-separated relative
// path with no leading "/" or "./" and no trailing "/". This matters most
// for a remote scope's Path, which internal/check later passes to fs.Sub -
// fs.Sub's fs.ValidPath requirement rejects most of those human-written
// forms outright. Prepending "/" before path.Clean also neutralizes a
// leading ".." (e.g. "../escape" -> "escape") rather than letting it climb
// out of the intended root.
func normalizeScopePath(p string) string {
	return strings.TrimPrefix(path.Clean("/"+p), "/")
}

// Config is one parsed and validated .docsweb.yaml.
type Config struct {
	// Name is this scope's own self-declared canonical name (dot-joined) -
	// required, every .docsweb.yaml must declare one, including the root
	// config; there is no implicit/default unscoped identity. A parent
	// config referencing this scope must use this exact name as its
	// scope: key - see the README's "Scopes" section.
	Name      string
	Audiences map[model.Audience]Audience
	Scopes    map[string]Scope
	// Ignore lists gitignore-style patterns (see internal/ignore) of files
	// and directories to exclude from every scope this config declares,
	// relative to this config's own directory.
	Ignore []string
}

// rawConfig mirrors the yaml file shape directly; Parse turns it into the
// validated Config type above.
type rawConfig struct {
	Name     string                 `yaml:"name"`
	Audience map[string]rawAudience `yaml:"audience"`
	Scope    map[string]rawScope    `yaml:"scope"`
	Ignore   []string               `yaml:"ignore"`
}

type rawAudience struct {
	Combine []string `yaml:"combine"`
}

type rawScope struct {
	Path        string            `yaml:"path"`
	Git         string            `yaml:"git"`
	Ref         string            `yaml:"ref"`
	AudienceMap map[string]string `yaml:"audienceMap"`
}

// Load reads and parses the .docsweb.yaml file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

// LoadFS reads and parses the .docsweb.yaml file at name within fsys - the
// fs.FS-based counterpart to Load, used for a scope whose file tree isn't
// necessarily an OS directory (e.g. a remote scope's git tree, read
// straight out of a commit via vcs.OpenScope with no worktree checkout).
func LoadFS(fsys fs.FS, name string) (*Config, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", name, err)
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", name, err)
	}
	return cfg, nil
}

// Parse parses and validates a .docsweb.yaml file's raw content.
//
// Duplicate audience or scope names are caught for free by yaml.v3, which
// errors on a mapping with a repeated key rather than silently keeping the
// last one.
func Parse(data []byte) (*Config, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid yaml: %w", err)
	}

	if raw.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	for _, seg := range strings.Split(raw.Name, ".") {
		if !model.ValidName(seg) {
			return nil, fmt.Errorf("invalid name %q: %q is not a valid alphanumeric name", raw.Name, seg)
		}
	}

	audiences := make(map[model.Audience]Audience, len(raw.Audience))
	for name, ra := range raw.Audience {
		if !model.ValidName(name) {
			return nil, fmt.Errorf("invalid audience name %q: must be alphanumeric", name)
		}
		combine := make([]model.Audience, 0, len(ra.Combine))
		for _, c := range ra.Combine {
			combine = append(combine, model.Audience(c))
		}
		audiences[model.Audience(name)] = Audience{Name: model.Audience(name), Combine: combine}
	}
	for _, a := range audiences {
		for _, c := range a.Combine {
			if _, ok := audiences[c]; !ok {
				return nil, fmt.Errorf("audience %q combines unknown audience %q", a.Name, c)
			}
		}
	}

	scopes := make(map[string]Scope, len(raw.Scope))
	for name, rs := range raw.Scope {
		for _, seg := range strings.Split(name, ".") {
			if !model.ValidName(seg) {
				return nil, fmt.Errorf("invalid scope name %q: %q is not a valid alphanumeric name", name, seg)
			}
		}
		if rs.Path == "" && rs.Git == "" {
			return nil, fmt.Errorf("scope %q must specify path or git", name)
		}

		audienceMap := make(map[model.Audience]model.Audience, len(rs.AudienceMap))
		for child, parent := range rs.AudienceMap {
			if !model.ValidName(child) {
				return nil, fmt.Errorf("scope %q: invalid audienceMap key %q: must be alphanumeric", name, child)
			}
			if !model.ValidName(parent) {
				return nil, fmt.Errorf("scope %q: invalid audienceMap value %q: must be alphanumeric", name, parent)
			}
			pa := model.Audience(parent)
			if _, ok := audiences[pa]; !ok {
				return nil, fmt.Errorf("scope %q: audienceMap %q maps to unknown audience %q", name, child, parent)
			}
			audienceMap[model.Audience(child)] = pa
		}

		scopes[name] = Scope{
			Name:        name,
			Path:        normalizeScopePath(rs.Path),
			Git:         rs.Git,
			Ref:         rs.Ref,
			AudienceMap: audienceMap,
		}
	}

	return &Config{Name: raw.Name, Audiences: audiences, Scopes: scopes, Ignore: raw.Ignore}, nil
}

// AudienceIncludes reports whether member is group itself, or is
// transitively reachable from group via one or more `combine` lists (e.g.
// with `it: {combine: [dev, tester]}`, AudienceIncludes("it", "dev") is
// true). Unknown audience names never include anything. Cycles in
// `combine` (direct or indirect) are tolerated and simply don't add any
// further membership.
func (c *Config) AudienceIncludes(group, member model.Audience) bool {
	return c.includes(group, member, make(map[model.Audience]bool))
}

func (c *Config) includes(group, member model.Audience, seen map[model.Audience]bool) bool {
	if group == member {
		return true
	}
	if seen[group] {
		return false
	}
	seen[group] = true
	for _, sub := range c.Audiences[group].Combine {
		if c.includes(sub, member, seen) {
			return true
		}
	}
	return false
}

// ResolveScopeAudience maps a scope-local audience name (as used by
// @audience annotations under the named referenced scope) to the audience name
// declared in this config. Per README.md's "Scopes" section: audiences
// with the same name auto-map to themselves; any other name must be
// listed as an audienceMap key on that scope. It reports false if neither
// applies.
func (c *Config) ResolveScopeAudience(scopeName string, child model.Audience) (model.Audience, bool) {
	if _, ok := c.Audiences[child]; ok {
		return child, true
	}
	parent, ok := c.Scopes[scopeName].AudienceMap[child]
	return parent, ok
}
