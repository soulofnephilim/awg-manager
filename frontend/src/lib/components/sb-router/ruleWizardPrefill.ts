import type { SingboxRouterPreset, SingboxRouterRule, SingboxRouterRuleSet } from '$lib/types';
import { stringifyInlineRuleListForWizard } from '$lib/utils/singboxInlineRules';
import type { OutboundCategory } from './addWizardStore';
import { classifyRuleSimplicity, templateIdForExternalRuleSetTag } from './simpleRule';

export type WizardEditMode = 'inline' | 'external';

export interface WizardPrefill {
  /** Режим визарда при редактировании простого правила. */
  editMode?: WizardEditMode;
  templateIds: string[];
  rulesList: string;
  outboundCategory: OutboundCategory;
  tunnelTag: string | null;
  /** Inline rule_set для update in-place (inline-set). */
  existingInlineRuleSetTag?: string;
  /** Было inline-text — при сохранении конвертировать в новый custom-N. */
  wasInlineText?: boolean;
}

function outboundFromRule(rule: SingboxRouterRule): {
  outboundCategory: OutboundCategory;
  tunnelTag: string | null;
} {
  if (rule.action === 'reject') {
    return { outboundCategory: 'block', tunnelTag: null };
  }
  if (rule.outbound === 'direct' || !rule.outbound) {
    return { outboundCategory: 'direct', tunnelTag: null };
  }
  return { outboundCategory: 'tunnel', tunnelTag: rule.outbound };
}

/** Map a simple rule into wizard form state. Caller must verify rule is simple. */
export function prefillWizardFromRule(
  rule: SingboxRouterRule,
  presets: SingboxRouterPreset[],
  ruleSets: SingboxRouterRuleSet[],
): WizardPrefill {
  const info = classifyRuleSimplicity(rule, ruleSets);
  const outbound = outboundFromRule(rule);

  if (!info.simple) {
    return { templateIds: [], rulesList: '', ...outbound };
  }

  if (info.kind === 'external' && info.externalRuleSetTag) {
    return {
      editMode: 'external',
      templateIds: [templateIdForExternalRuleSetTag(info.externalRuleSetTag, presets)],
      rulesList: '',
      ...outbound,
    };
  }

  if (info.kind === 'inline-set' && info.inlineRuleSetTag) {
    const rs = ruleSets.find((r) => r.tag === info.inlineRuleSetTag);
    const rulesList = rs?.type === 'inline' ? stringifyInlineRuleListForWizard(rs.rules) : '';
    return {
      editMode: 'inline',
      templateIds: [],
      rulesList,
      existingInlineRuleSetTag: info.inlineRuleSetTag,
      ...outbound,
    };
  }

  // inline-text
  const rulesList = stringifyInlineRuleListForWizard([rule as Record<string, unknown>]);
  return {
    editMode: 'inline',
    templateIds: [],
    rulesList,
    wasInlineText: true,
    ...outbound,
  };
}
