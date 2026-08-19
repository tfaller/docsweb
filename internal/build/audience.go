package build

import (
	"fmt"

	"github.com/tfaller/docsweb/internal/collect"
	"github.com/tfaller/docsweb/internal/config"
	"github.com/tfaller/docsweb/internal/model"
)

// remapScopeAudiences applies README.md's "Scopes" audience-mapping rule to
// every target outside the root scope: an audience with the same name as
// one declared in the root config auto-maps to itself; any other name must
// be listed in that scope's audienceMap, or it's a hard error. The reserved
// "all" audience always passes through unmapped.
func remapScopeAudiences(cfg *config.Config, reg *collect.Registry, rootScope string) error {
	for _, t := range reg.Targets() {
		if t.Scope == rootScope {
			continue
		}
		for i, a := range t.Audiences {
			mapped, err := mapScopeAudience(cfg, t.Scope, a)
			if err != nil {
				return fmt.Errorf("%s: @audience: %w", t.Key(), err)
			}
			t.Audiences[i] = mapped
		}
		for ci, c := range t.Changelog {
			for i, a := range c.Audiences {
				mapped, err := mapScopeAudience(cfg, t.Scope, a)
				if err != nil {
					return fmt.Errorf("%s: @changelog @audience: %w", t.Key(), err)
				}
				t.Changelog[ci].Audiences[i] = mapped
			}
		}
	}
	return nil
}

func mapScopeAudience(cfg *config.Config, scope string, a model.Audience) (model.Audience, error) {
	if a == model.AudienceAll {
		return a, nil
	}
	mapped, ok := cfg.ResolveScopeAudience(scope, a)
	if !ok {
		return "", fmt.Errorf("scope %q: audience %q does not auto-map to a declared audience and has no audienceMap entry", scope, a)
	}
	return mapped, nil
}
