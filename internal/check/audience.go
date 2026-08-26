package check

import (
	"fmt"

	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/config"
	"github.com/tfaller/docsweb/internal/model"
)

// checkAudiences applies README.md's "Scopes" audience-mapping rule to every
// target: a root-scope target's audience must itself be declared in the
// root config's audience: map. A non-root-scope target's audience with the
// same name as a declared audience auto-maps to itself; any other name must
// be listed in that scope's audienceMap. Either way, an undeclared name is a
// hard error. The reserved "all" audience always passes through unmapped.
func checkAudiences(ctx *context) error {
	return remapScopeAudiences(ctx.cfg, ctx.registry)
}

func remapScopeAudiences(cfg *config.Config, reg *collect.Registry) error {
	for _, t := range reg.Targets() {
		isRoot := t.ConfigScope == cfg.Name
		for i, a := range t.Audiences {
			mapped, err := resolveAudience(cfg, t.ConfigScope, isRoot, a)
			if err != nil {
				return fmt.Errorf("%s: @audience: %w", t.Key(), err)
			}
			t.Audiences[i] = mapped
		}
		for ci, c := range t.Changelog {
			for i, a := range c.Audiences {
				mapped, err := resolveAudience(cfg, t.ConfigScope, isRoot, a)
				if err != nil {
					return fmt.Errorf("%s: @changelog @audience: %w", t.Key(), err)
				}
				t.Changelog[ci].Audiences[i] = mapped
			}
		}
	}
	return nil
}

func resolveAudience(cfg *config.Config, scope string, isRoot bool, a model.Audience) (model.Audience, error) {
	if a == model.AudienceAll {
		return a, nil
	}
	if isRoot {
		if _, ok := cfg.Audiences[a]; !ok {
			return "", fmt.Errorf("audience %q is not declared in this config's audience: map", a)
		}
		return a, nil
	}
	mapped, ok := cfg.ResolveScopeAudience(scope, a)
	if !ok {
		return "", fmt.Errorf("scope %q: audience %q does not auto-map to a declared audience and has no audienceMap entry", scope, a)
	}
	return mapped, nil
}
