import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  matchersFromParsedRules,
  submitMatchersOnlyEdit,
  submitTextMatchersEdit,
} from './matchersEditActions';
import { ValidationError } from './addWizardActions';

vi.mock('$lib/api/client', () => ({
  api: {
    expandGeoTag: vi.fn(),
    singboxRouterUpdateRule: vi.fn(),
    singboxRouterUpdateRuleSet: vi.fn(),
  },
}));

import { api } from '$lib/api/client';

describe('matchersFromParsedRules', () => {
  it('merges domains and ips from multiple records', () => {
    expect(
      matchersFromParsedRules([
        { domain_suffix: ['a.com', 'b.com'] },
        { ip_cidr: ['1.1.1.1/32'] },
      ]),
    ).toEqual({
      domain_suffix: ['a.com', 'b.com'],
      ip_cidr: ['1.1.1.1/32'],
    });
  });

  it('deduplicates values', () => {
    expect(
      matchersFromParsedRules([
        { domain_suffix: ['a.com', 'a.com'] },
      ]),
    ).toEqual({ domain_suffix: ['a.com'] });
  });

  it('includes domain exact match', () => {
    expect(matchersFromParsedRules([{ domain: ['host.example'] }])).toEqual({
      domain: ['host.example'],
    });
  });

  it('empty input → empty object', () => {
    expect(matchersFromParsedRules([])).toEqual({});
  });
});

describe('submitMatchersOnlyEdit', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('updates inline rule_set', async () => {
    await submitMatchersOnlyEdit({
      rulesList: 'example.com',
      inlineRuleSetTag: 'custom-1',
    });
    expect(api.singboxRouterUpdateRuleSet).toHaveBeenCalledWith('custom-1', {
      tag: 'custom-1',
      type: 'inline',
      rules: [{ domain_suffix: ['example.com'] }],
    });
    expect(api.singboxRouterUpdateRule).not.toHaveBeenCalled();
  });

  it('пустой список → ValidationError', async () => {
    await expect(
      submitMatchersOnlyEdit({ rulesList: '   ', inlineRuleSetTag: 'custom-1' }),
    ).rejects.toThrow(ValidationError);
  });

  it('port в списке → ValidationError', async () => {
    await expect(
      submitMatchersOnlyEdit({ rulesList: 'port:443', inlineRuleSetTag: 'custom-1' }),
    ).rejects.toThrow(/порты/i);
  });

  it('src_ip в списке → ValidationError', async () => {
    await expect(
      submitMatchersOnlyEdit({ rulesList: 'src_ip:192.168.1.1', inlineRuleSetTag: 'custom-1' }),
    ).rejects.toThrow(/source ip/i);
  });
});

describe('submitTextMatchersEdit', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('updates domain_suffix, preserves outbound', async () => {
    await submitTextMatchersEdit({
      ruleIndex: 2,
      domain_suffix: ['foo.com'],
      ip_cidr: [],
      originalRule: { domain_suffix: ['old.com'], outbound: 'warp', action: 'route' },
    });
    expect(api.singboxRouterUpdateRule).toHaveBeenCalledWith(2, {
      domain_suffix: ['foo.com'],
      action: 'route',
      outbound: 'warp',
    });
  });

  it('updates ip_cidr', async () => {
    await submitTextMatchersEdit({
      ruleIndex: 0,
      domain_suffix: [],
      ip_cidr: ['10.0.0.0/8'],
      originalRule: { outbound: 'direct', action: 'route' },
    });
    expect(api.singboxRouterUpdateRule).toHaveBeenCalledWith(0, {
      ip_cidr: ['10.0.0.0/8'],
      action: 'route',
      outbound: 'direct',
    });
  });

  it('reject action сохраняется', async () => {
    await submitTextMatchersEdit({
      ruleIndex: 1,
      domain_suffix: ['blocked.com'],
      ip_cidr: [],
      originalRule: { action: 'reject', domain_suffix: ['old.com'] },
    });
    expect(api.singboxRouterUpdateRule).toHaveBeenCalledWith(1, {
      domain_suffix: ['blocked.com'],
      action: 'reject',
    });
  });

  it('пустые matchers → ValidationError', async () => {
    await expect(
      submitTextMatchersEdit({
        ruleIndex: 0,
        domain_suffix: [],
        ip_cidr: [],
        originalRule: { outbound: 'warp', action: 'route' },
      }),
    ).rejects.toThrow(/домен или IP/i);
  });
});
