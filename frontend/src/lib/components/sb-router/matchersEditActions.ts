import { api } from '$lib/api/client';
import type { SingboxRouterRule } from '$lib/types';
import {
  parseCustomList,
  ValidationError,
} from './addWizardActions';
import {
  stripWizardRuleOnlyFieldsFromInlineRules,
  collectSourceIpCidrFromInlineRules,
  collectPortFromInlineRules,
} from '$lib/utils/singboxInlineRules';

function assertNoWizardRuleOnlyFieldsInList(parsed: Record<string, unknown>[]): void {
  if (collectSourceIpCidrFromInlineRules(parsed)) {
    throw new ValidationError('Source IP задаётся в эксперт-режиме на правиле, не в списке доменов');
  }
  if (collectPortFromInlineRules(parsed)) {
    throw new ValidationError('Порты задаются в эксперт-режиме на правиле, не в списке доменов');
  }
}

const WIZARD_RULE_TEXT_KEYS = ['domain', 'domain_suffix', 'ip_cidr'] as const;

/** Сливает parsed inline-записи в поля matcher'ов на router rule. */
export function matchersFromParsedRules(
  rules: Record<string, unknown>[],
): Pick<SingboxRouterRule, 'domain' | 'domain_suffix' | 'ip_cidr'> {
  const acc: Record<(typeof WIZARD_RULE_TEXT_KEYS)[number], string[]> = {
    domain: [],
    domain_suffix: [],
    ip_cidr: [],
  };
  for (const r of rules) {
    for (const key of WIZARD_RULE_TEXT_KEYS) {
      const v = r[key];
      if (Array.isArray(v)) {
        acc[key].push(...v.filter((x): x is string => typeof x === 'string'));
      }
    }
  }
  const result: Pick<SingboxRouterRule, 'domain' | 'domain_suffix' | 'ip_cidr'> = {};
  for (const key of WIZARD_RULE_TEXT_KEYS) {
    const unique = [...new Set(acc[key])];
    if (unique.length > 0) result[key] = unique;
  }
  return result;
}

export interface SubmitTextMatchersEditArgs {
  ruleIndex: number;
  domain_suffix: string[];
  ip_cidr: string[];
  originalRule: SingboxRouterRule;
}

/** Обновляет domain_suffix / ip_cidr на правиле; outbound не трогает. */
export async function submitTextMatchersEdit(args: SubmitTextMatchersEditArgs): Promise<void> {
  if (args.domain_suffix.length === 0 && args.ip_cidr.length === 0) {
    throw new ValidationError('Нужен хотя бы один домен или IP');
  }

  const built: SingboxRouterRule = {};
  if (args.domain_suffix.length > 0) built.domain_suffix = args.domain_suffix;
  if (args.ip_cidr.length > 0) built.ip_cidr = args.ip_cidr;

  if (args.originalRule.action === 'reject') {
    built.action = 'reject';
  } else {
    built.action = 'route';
    if (args.originalRule.outbound) built.outbound = args.originalRule.outbound;
  }

  await api.singboxRouterUpdateRule(args.ruleIndex, built);
}

export interface SubmitMatchersOnlyEditArgs {
  rulesList: string;
  inlineRuleSetTag: string;
}

/** Обновляет inline rule_set по списку (простой режим). */
export async function submitMatchersOnlyEdit(args: SubmitMatchersOnlyEditArgs): Promise<void> {
  const parsed = await parseCustomList(args.rulesList);
  assertNoWizardRuleOnlyFieldsInList(parsed);
  const customRules = stripWizardRuleOnlyFieldsFromInlineRules(parsed);
  if (customRules.length === 0) throw new ValidationError('Нет валидных строк');

  const tag = args.inlineRuleSetTag;
  await api.singboxRouterUpdateRuleSet(tag, { tag, type: 'inline', rules: customRules });
}
