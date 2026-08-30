package check

import (
	"fmt"
	"path/filepath"

	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/config"
	"github.com/tfaller/docsweb/internal/ignore"
	"github.com/tfaller/docsweb/internal/vcs"
)

// cacheDirName is the directory (relative to the root scope's own
// directory) remote scopes are cloned into - see README.md's "Scopes"
// section.
const cacheDirName = "docsweb-cache"

// checkScopes loads the root config, clones/fetches every remote scope into
// the local cache directory, verifies every declared referenced scope's own
// config against the parent's expectation, and walks every scope's file
// tree into a fresh registry - populating everything later checks (and the
// final Result) depend on: ctx.cfg, ctx.rootDir, ctx.scopeRoots,
// ctx.registry.
func checkScopes(ctx *context) error {
	cfg, err := config.Load(ctx.opts.ConfigPath)
	if err != nil {
		return err
	}
	rootDir, err := filepath.Abs(filepath.Dir(ctx.opts.ConfigPath))
	if err != nil {
		return err
	}

	reg := collect.NewRegistry()
	matcher := ignore.Compile(cfg.Ignore)
	cacheDir := filepath.Join(rootDir, cacheDirName)

	// scopeRoots maps every scope name (including the root scope) to its
	// absolute directory, reused by internal/build to locate each target's
	// defining file for git-blame attribution.
	scopeRoots := map[string]string{cfg.Name: rootDir}
	// remoteScope tracks which scope names came from a git clone rather
	// than a local path, so the root config's own ignore: rules - relative
	// to the root scope's own directory - aren't applied to an unrelated
	// repository's content below.
	remoteScope := make(map[string]bool, len(cfg.Scopes))
	// excludes keeps the root scope's own walk from descending into any
	// referenced scope's directory - a local scope's own path, or (once,
	// regardless of how many remote scopes exist) the whole cache
	// directory every remote scope is cloned into.
	excludes := []string{cacheDir}

	for name, sc := range cfg.Scopes {
		if name == cfg.Name {
			return fmt.Errorf("scope %q: collides with the root scope's own name", name)
		}

		scopeRoot := filepath.Join(rootDir, sc.Path)
		if sc.Remote() {
			cloneDir, err := vcs.CloneOrFetch(cacheDir, sc.Git, sc.Ref)
			if err != nil {
				return fmt.Errorf("scope %q: %w", name, err)
			}
			scopeRoot = filepath.Join(cloneDir, sc.Path)
			remoteScope[name] = true
		} else {
			excludes = append(excludes, scopeRoot)
		}

		refConfigPath := filepath.Join(scopeRoot, ".docsweb.yaml")
		refCfg, err := config.Load(refConfigPath)
		if err != nil {
			return fmt.Errorf("scope %q: expected referenced scope config at %s: %w", name, refConfigPath, err)
		}
		if refCfg.Name != name {
			return fmt.Errorf("scope %q: %s declares name %q, expected %q", name, refConfigPath, refCfg.Name, name)
		}
		scopeRoots[name] = scopeRoot
	}
	if err := reg.AddScope(collect.Options{Scope: cfg.Name, Root: rootDir, Exclude: excludes, Ignore: matcher, IgnoreBase: rootDir}); err != nil {
		return err
	}
	for name := range cfg.Scopes {
		opts := collect.Options{Scope: name, Root: scopeRoots[name]}
		if !remoteScope[name] {
			opts.Ignore = matcher
			opts.IgnoreBase = rootDir
		}
		if err := reg.AddScope(opts); err != nil {
			return err
		}
	}

	ctx.cfg = cfg
	ctx.rootDir = rootDir
	ctx.scopeRoots = scopeRoots
	ctx.registry = reg
	return nil
}
