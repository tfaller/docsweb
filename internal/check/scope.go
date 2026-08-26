package check

import (
	"fmt"
	"path/filepath"

	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/config"
	"github.com/tfaller/docsweb/internal/ignore"
)

// checkScopes loads the root config, verifies every declared referenced
// scope's own config against the parent's expectation, and walks every
// scope's file tree into a fresh registry - populating everything later
// checks (and the final Result) depend on: ctx.cfg, ctx.rootDir,
// ctx.scopeRoots, ctx.registry.
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

	// scopeRoots maps every scope name (including the root scope) to its
	// absolute directory, reused by internal/build to locate each target's
	// defining file for git-blame attribution.
	scopeRoots := map[string]string{cfg.Name: rootDir}
	excludes := make([]string, 0, len(cfg.Scopes))
	for name, sc := range cfg.Scopes {
		if sc.Remote() {
			return fmt.Errorf("scope %q: remote scopes are not supported by the POC (see README.md \"After POC\")", name)
		}
		if name == cfg.Name {
			return fmt.Errorf("scope %q: collides with the root scope's own name", name)
		}
		scopeRoot := filepath.Join(rootDir, sc.Path)
		refConfigPath := filepath.Join(scopeRoot, ".docsweb.yaml")
		refCfg, err := config.Load(refConfigPath)
		if err != nil {
			return fmt.Errorf("scope %q: expected referenced scope config at %s: %w", name, refConfigPath, err)
		}
		if refCfg.Name != name {
			return fmt.Errorf("scope %q: %s declares name %q, expected %q", name, refConfigPath, refCfg.Name, name)
		}
		scopeRoots[name] = scopeRoot
		excludes = append(excludes, scopeRoot)
	}
	if err := reg.AddScope(collect.Options{Scope: cfg.Name, Root: rootDir, Exclude: excludes, Ignore: matcher, IgnoreBase: rootDir}); err != nil {
		return err
	}
	for name := range cfg.Scopes {
		if err := reg.AddScope(collect.Options{Scope: name, Root: scopeRoots[name], Ignore: matcher, IgnoreBase: rootDir}); err != nil {
			return err
		}
	}

	ctx.cfg = cfg
	ctx.rootDir = rootDir
	ctx.scopeRoots = scopeRoots
	ctx.registry = reg
	return nil
}
