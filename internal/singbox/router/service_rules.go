package router

import (
	"context"
	"fmt"
)

func (s *ServiceImpl) ListRules(ctx context.Context) ([]Rule, error) {
	cfg, err := s.loadRouterConfig()
	if err != nil {
		return nil, err
	}
	rules := s.ruleSetMaterializer().restoreConfig(cfg).Route.Rules
	filtered, _ := stripSelectiveManagedRules(rules)
	return filtered, nil
}

func (s *ServiceImpl) AddRule(ctx context.Context, r Rule) error {
	return s.withConfig(ctx, "rules", func(c *RouterConfig) error { return c.AddRule(r) })
}

func (s *ServiceImpl) UpdateRule(ctx context.Context, index int, r Rule) error {
	return s.withConfig(ctx, "rules", func(c *RouterConfig) error { return c.UpdateRule(index, r) })
}

func (s *ServiceImpl) DeleteRule(ctx context.Context, index int) error {
	return s.withConfig(ctx, "rules", func(c *RouterConfig) error { return c.DeleteRule(index) })
}

func (s *ServiceImpl) MoveRule(ctx context.Context, from, to int) error {
	return s.withConfig(ctx, "rules", func(c *RouterConfig) error { return c.MoveRule(from, to) })
}

func (s *ServiceImpl) SetRouteFinal(ctx context.Context, tag string) error {
	if err := s.withConfig(ctx, "route", func(c *RouterConfig) error {
		if !s.isKnownOutboundTag(ctx, tag, c) {
			return fmt.Errorf("unknown outbound tag %q for route.final", tag)
		}
		return c.SetRouteFinal(tag)
	}); err != nil {
		return err
	}
	if tag != "direct" {
		return s.disableSelectiveBypassIfEnabled(ctx)
	}
	return nil
}

// isKnownOutboundTag returns true if tag is a sing-box built-in or matches
// an outbound from any known catalog (router composites, subscription
// composites, AWG, sing-box tunnels).
func (s *ServiceImpl) isKnownOutboundTag(ctx context.Context, tag string, cfg *RouterConfig) bool {
	if tag == "direct" || tag == "block" || tag == "dns" {
		return true
	}
	// Router-managed composites
	for _, o := range cfg.Outbounds {
		if o.Tag == tag {
			return true
		}
	}
	// Subscription composites (40-subscriptions.json)
	if s.deps.SubscriptionComposites != nil {
		for _, o := range s.deps.SubscriptionComposites.ListSubscriptionComposites() {
			if o.Tag == tag {
				return true
			}
		}
	}
	// AWG-direct outbounds (managed + system)
	if s.deps.AWGTags != nil {
		if tags, err := s.deps.AWGTags.ListTags(ctx); err == nil {
			for _, t := range tags {
				if t.Tag == tag {
					return true
				}
			}
		}
	}
	// Sing-box tunnels (10-tunnels.json)
	if s.deps.SingboxTunnels != nil {
		if tags, err := s.deps.SingboxTunnels.ListTunnelTags(ctx); err == nil {
			for _, t := range tags {
				if t == tag {
					return true
				}
			}
		}
	}
	return false
}

func (s *ServiceImpl) ListRuleSets(ctx context.Context) ([]RuleSet, error) {
	cfg, err := s.loadRouterConfig()
	if err != nil {
		return nil, err
	}
	restored := s.ruleSetMaterializer().restoreConfig(cfg)
	return restored.Route.RuleSet, nil
}

func (s *ServiceImpl) AddRuleSet(ctx context.Context, rs RuleSet) error {
	if rs.Type == "" {
		rs.Type = "remote"
	}
	if rs.Format == "" && rs.Type != "inline" {
		rs.Format = "binary"
	}
	if rs.UpdateInterval == "" && rs.Type == "remote" {
		rs.UpdateInterval = "24h"
	}
	return s.withConfig(ctx, "rulesets", func(c *RouterConfig) error { return c.AddRuleSet(rs) })
}

func (s *ServiceImpl) UpdateRuleSet(ctx context.Context, tag string, rs RuleSet) error {
	if rs.Type == "" {
		rs.Type = "remote"
	}
	if rs.Format == "" && rs.Type != "inline" {
		rs.Format = "binary"
	}
	if rs.UpdateInterval == "" && rs.Type == "remote" {
		rs.UpdateInterval = "24h"
	}
	return s.withConfig(ctx, "rulesets", func(c *RouterConfig) error { return c.UpdateRuleSet(tag, rs) })
}

func (s *ServiceImpl) DeleteRuleSet(ctx context.Context, tag string, force bool) error {
	inlineTag := tag
	if base, ok := inlineTagFromSRSTag(tag); ok {
		inlineTag = base
	}
	return s.withConfig(ctx, "rulesets", func(c *RouterConfig) error {
		if err := c.DeleteRuleSet(inlineTag, force); err != nil {
			return err
		}
		if s.deps.Orch == nil {
			s.ruleSetMaterializer().removeInlineArtifacts(inlineTag)
		}
		return nil
	})
}
