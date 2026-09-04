package check

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/tfaller/docsweb/internal/auth"
	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/config"
	"github.com/tfaller/docsweb/internal/ignore"
	"github.com/tfaller/docsweb/internal/vcs"
)

// cacheDirName is the directory (relative to the root scope's own
// directory) remote scopes are mirrored into - see README.md's "Scopes"
// section.
const cacheDirName = "docsweb-cache"

// checkScopes loads the root config, opens/fetches every remote scope's
// resolved commit into the local cache directory, verifies every declared
// referenced scope's own config against the parent's expectation, and walks
// every scope's file tree into a fresh registry - populating everything
// later checks (and the final Result) depend on: ctx.cfg, ctx.rootDir,
// ctx.scopeRoots, ctx.remoteScopes, ctx.registry.
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
	rootFS := os.DirFS(rootDir)

	// scopeRoots maps every scope name that has a real on-disk root
	// (including the root scope, plus every local referenced scope) to its
	// absolute directory, reused by internal/build to locate each target's
	// defining file for git-blame attribution. A remote scope has no such
	// directory - it's looked up in remoteScopes instead.
	scopeRoots := map[string]string{cfg.Name: rootDir}
	// remoteScopes carries, for every remote (git:) scope, its resolved
	// Repository (pinned to the commit its file tree was read from) and its
	// own path within that repository's tree - both needed by
	// internal/build's git-blame attribution, since a remote scope has no
	// on-disk checkout for vcs.Open to discover.
	remoteScopes := map[string]RemoteScope{}
	// scopeFS collects every scope's own file system, keyed by scope name,
	// ready to hand to collect.AddScope.
	scopeFS := map[string]fs.FS{cfg.Name: rootFS}
	// excludes keeps the root scope's own walk from descending into any
	// referenced scope's directory - a local scope's own path, or (once,
	// regardless of how many remote scopes exist) the whole cache
	// directory every remote scope is mirrored into.
	excludes := []string{cacheDirName}
	// credentials resolves HTTP auth for a remote scope's git URL (e.g. a
	// GitLab CI job token for a private gitlab.com repository) - see
	// internal/auth. A public repository needs no credentials, so a nil
	// result here is the common case, not an error.
	credentials := auth.Default()

	for name, sc := range cfg.Scopes {
		if name == cfg.Name {
			return fmt.Errorf("scope %q: collides with the root scope's own name", name)
		}

		var (
			sFS       fs.FS
			configLoc string
		)

		if sc.Remote() {
			clientOpts, err := credentials.ClientOptions(sc.Git)
			if err != nil {
				return fmt.Errorf("scope %q: resolving credentials for %s: %w", name, sc.Git, err)
			}
			treeFS, repo, err := vcs.OpenScope(cacheDir, sc.Git, sc.Ref, clientOpts...)
			if err != nil {
				return fmt.Errorf("scope %q: %w", name, err)
			}
			sFS = treeFS
			if sc.Path != "" {
				sFS, err = fs.Sub(sFS, sc.Path)
				if err != nil {
					return fmt.Errorf("scope %q: %w", name, err)
				}
			}
			remoteScopes[name] = RemoteScope{Repo: repo, Path: sc.Path}
			configLoc = fmt.Sprintf("%s@%s:%s", sc.Git, sc.Ref, path.Join(sc.Path, ".docsweb.yaml"))
		} else {
			scopeRoot := filepath.Join(rootDir, sc.Path)
			sFS = os.DirFS(scopeRoot)
			scopeRoots[name] = scopeRoot
			excludes = append(excludes, sc.Path)
			configLoc = filepath.Join(scopeRoot, ".docsweb.yaml")
		}
		scopeFS[name] = sFS

		refCfg, err := config.LoadFS(sFS, ".docsweb.yaml")
		if err != nil {
			return fmt.Errorf("scope %q: expected referenced scope config at %s: %w", name, configLoc, err)
		}
		if refCfg.Name != name {
			return fmt.Errorf("scope %q: %s declares name %q, expected %q", name, configLoc, refCfg.Name, name)
		}
	}
	if err := reg.AddScope(collect.Options{Scope: cfg.Name, Root: rootFS, Exclude: excludes, Ignore: matcher}); err != nil {
		return err
	}
	for name := range cfg.Scopes {
		opts := collect.Options{Scope: name, Root: scopeFS[name]}
		if _, remote := remoteScopes[name]; !remote {
			opts.Ignore = matcher
			opts.IgnoreOffset = cfg.Scopes[name].Path
		}
		if err := reg.AddScope(opts); err != nil {
			return err
		}
	}

	ctx.cfg = cfg
	ctx.rootDir = rootDir
	ctx.scopeRoots = scopeRoots
	ctx.remoteScopes = remoteScopes
	ctx.registry = reg
	return nil
}
