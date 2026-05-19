import { describe, it, expect } from 'vitest';
import { parseInlineRuleList } from '$lib/utils/singboxInlineRules';

describe('parseInlineRuleList', () => {
	// ── domains ────────────────────────────────────────────────
	it('groups plain domain, wildcard and URL into domain+suffix groups', () => {
		const input = [
			'openai.com',
			'*.perplexity.ai',
			'https://gemini.google.com/app',
		].join('\n');
		const { rules, errors, warnings } = parseInlineRuleList(input);
		expect(errors).toHaveLength(0);
		expect(warnings).toHaveLength(0);
		expect(rules).toHaveLength(1);
		const g = rules[0];
		expect(g['domain']).toEqual(['openai.com', 'gemini.google.com']);
		expect(g['domain_suffix'] as string[]).toContain('.openai.com');
		expect(g['domain_suffix'] as string[]).toContain('.gemini.google.com');
		// *.perplexity.ai → extracted suffix "perplexity.ai" with leading dot
		expect(g['domain_suffix'] as string[]).toContain('.perplexity.ai');
	});

	it('handles dot-prefix domain as domain_suffix only', () => {
		const { rules, errors } = parseInlineRuleList('.example.com');
		expect(errors).toHaveLength(0);
		expect(rules).toEqual([{ domain_suffix: ['.example.com'] }]);
	});

	// ── adblock ────────────────────────────────────────────────
	it('parses ||claude.ai^ as claude.ai', () => {
		const { rules, errors, warnings } = parseInlineRuleList('||claude.ai^');
		expect(errors).toHaveLength(0);
		expect(warnings).toHaveLength(0);
		expect(rules).toEqual([{ domain: ['claude.ai'], domain_suffix: ['.claude.ai'] }]);
	});

	it('warns on @@ exception and skips it', () => {
		const { rules, errors, warnings } = parseInlineRuleList('@@||example.com^');
		expect(rules).toHaveLength(0);
		expect(errors).toHaveLength(0);
		expect(warnings).toHaveLength(1);
		expect(warnings[0]).toBe('Строка 1: исключения @@ пока не поддерживаются в inline rule set');
	});

	// ── IP/CIDR ───────────────────────────────────────────────
	it('creates separate ip_cidr group', () => {
		const input = ['1.1.1.1', '8.8.8.0/24'].join('\n');
		const { rules, errors } = parseInlineRuleList(input);
		expect(errors).toHaveLength(0);
		expect(rules).toEqual([{ ip_cidr: ['1.1.1.1', '8.8.8.0/24'] }]);
	});

	// ── prefix fields ─────────────────────────────────────────
	it('creates separate groups for each prefix field', () => {
		const r = parseInlineRuleList([
			'keyword:youtube',
			'regex:^(.+\\.)?example-cdn\\.',
			'port:443,8443',
			'process:curl',
			'package:com.termux',
			'network:tcp',
		].join('\n'));
		expect(r.errors).toHaveLength(0);
		expect(r.rules).toHaveLength(5);

		// keyword + regex share one group when present together
		expect(r.rules[0]).toEqual({ domain_keyword: ['youtube'], domain_regex: ['^(.+\\.)?example-cdn\\.'] });
		expect(r.rules[1]).toEqual({ port: [443, 8443] });
		expect(r.rules[2]).toEqual({ process_name: ['curl'] });
		expect(r.rules[3]).toEqual({ package_name: ['com.termux'] });
		expect(r.rules[4]).toEqual({ network: ['tcp'] });
	});

	// ── unsupported network ───────────────────────────────────
	it('rejects unsupported network value as error, does not add to group', () => {
		const { rules, errors, warnings } = parseInlineRuleList('network:http');
		expect(errors).toHaveLength(1);
		expect(warnings).toHaveLength(0);
		expect(errors[0]).toContain('http');
		expect(rules).toHaveLength(0);
	});

	// ── invalid port ──────────────────────────────────────────
	it('reports error on invalid port boundary value', () => {
		const { errors } = parseInlineRuleList('port:99999');
		expect(errors).toHaveLength(1);
		expect(errors[0]).toContain('99999');
	});

	it('rejects non-numeric port token', () => {
		const { rules, errors, warnings } = parseInlineRuleList('port:443abc');
		expect(errors).toHaveLength(1);
		expect(errors[0]).toContain('443abc');
		expect(rules).toHaveLength(0);
		expect(warnings).toHaveLength(0);
	});

	it('rejects port range token', () => {
		const { rules, errors, warnings } = parseInlineRuleList('port:80-90');
		expect(errors).toHaveLength(1);
		expect(errors[0]).toContain('80-90');
		expect(rules).toHaveLength(0);
		expect(warnings).toHaveLength(0);
	});

	// ── empty input ───────────────────────────────────────────
	it('returns no-valid-lines error on empty input', () => {
		const { rules, errors } = parseInlineRuleList('');
		expect(rules).toHaveLength(0);
		expect(errors).toHaveLength(1);
		expect(errors[0]).toBe('Нет валидных строк для inline rule set');
	});

	// ── comments ──────────────────────────────────────────────
	it('skips #, // and ; comments silently', () => {
		const input = [
			'# comment',
			'// line comment',
			'; semicolon comment',
			'openai.com',
		].join('\n');
		const r = parseInlineRuleList(input);
		expect(r.errors).toHaveLength(0);
		expect(r.rules).toHaveLength(1);
		expect(r.warnings).toHaveLength(0);
	});

	// ── inline comments ───────────────────────────────────────
	it('strips inline trailing comments', () => {
		const { rules } = parseInlineRuleList('openai.com # this is a comment');
		expect(rules).toEqual([{ domain: ['openai.com'], domain_suffix: ['.openai.com'] }]);
	});

	// ── src_ip / source_ip ────────────────────────────────────
	it('validates src_ip and source_ip values; invalid IP produces error', () => {
		const input = ['src_ip:192.168.1.50', 'source_ip:not-an-ip', 'source_ip:10.0.0.1/24'].join('\n');
		const { rules, errors } = parseInlineRuleList(input);
		expect(errors).toHaveLength(1);
		expect(errors[0]).toContain('src_ip');
		expect(rules).toEqual([{ source_ip_cidr: ['192.168.1.50', '10.0.0.1/24'] }]);
	});

	// ── dedup ─────────────────────────────────────────────────
	it('deduplicates values within each group', () => {
		const input = ['openai.com', 'openai.com', 'port:443,443'].join('\n');
		const { rules } = parseInlineRuleList(input);
		const g = rules.find((r) => (r as Record<string, unknown>).domain);
		expect((g!['domain'] as string[])).toEqual(['openai.com']);
	});

	// ── domain:/suffix:/domain_suffix: prefixes ───────────────
	it('handles domain: prefix with leading dot in domain_suffix', () => {
		const { rules } = parseInlineRuleList('domain:example.com');
		expect(rules).toEqual([{ domain: ['example.com'], domain_suffix: ['.example.com'] }]);
	});

	it('handles domain_suffix with leading dot', () => {
		const { rules } = parseInlineRuleList('domain_suffix:.example.com');
		expect(rules).toEqual([{ domain_suffix: ['.example.com'] }]);
	});

	it('handles suffix with wildcard', () => {
		const { rules } = parseInlineRuleList('suffix:*.example.org');
		expect(rules).toEqual([{ domain_suffix: ['.example.org'] }]);
	});

	it('handles domain_keyword: and domain_regex: prefixes', () => {
		const { rules } = parseInlineRuleList(['domain_keyword:youtube', 'domain_regex:^ad\\.'].join('\n'));
		expect(rules).toEqual([{ domain_keyword: ['youtube'], domain_regex: ['^ad\\.'] }]);
	});

	// ── IP validation ────────────────────────────────────────
	it('rejects CIDR with out-of-range mask (999)', () => {
		const { rules, errors } = parseInlineRuleList('cidr:8.8.8.0/999');
		expect(rules).toHaveLength(0);
		expect(errors).toHaveLength(1);
	});

	it('accepts valid bare IPv4 and CIDR', () => {
		const { rules, errors } = parseInlineRuleList('1.2.3.4\n5.6.7.8/16');
		expect(errors).toHaveLength(0);
		expect(rules).toHaveLength(1);
	});

	// ── domain: invalid value ────────────────────────────────
	it('rejects invalid domain value for domain: prefix', () => {
		const { rules, errors } = parseInlineRuleList('domain:!!!invalid!!!');
		expect(rules).toHaveLength(0);
		expect(errors).toHaveLength(1);
	});

	it('port token keeps valid entries after garbage token', () => {
		// port:443abc is invalid, port:443 is still processed
		const r = parseInlineRuleList('port:443,443abc,8443');
		expect(r.errors).toHaveLength(1);
		expect(r.errors[0]).toContain('443abc');
		expect(r.warnings).toHaveLength(1);
		expect(r.warnings[0]).toContain('широкий трафик');
	});

	// ── wide-matcher warnings ──────────────────────────────────
	it('emits single wide-traffic warning for single port', () => {
		const { rules, errors, warnings } = parseInlineRuleList('port:443');
		expect(errors).toHaveLength(0);
		expect(rules).toEqual([{ port: [443] }]);
		expect(warnings).toHaveLength(1);
		expect(warnings[0]).toContain('широкий трафик');
	});

	it('emits single wide-traffic warning for multi-port list', () => {
		const { rules, warnings } = parseInlineRuleList('port:443,8443');
		expect(warnings).toHaveLength(1);
		expect(warnings[0]).toContain('широкий трафик');
		expect(rules[0]).toEqual({ port: [443, 8443] });
	});

	it('emits process local-process warning', () => {
		const { rules, errors, warnings } = parseInlineRuleList('process:curl');
		expect(errors).toHaveLength(0);
		expect(rules).toEqual([{ process_name: ['curl'] }]);
		expect(warnings).toHaveLength(1);
		expect(warnings[0]).toContain('локальным процессам');
	});

	it('emits process_path local-process warning', () => {
		const { rules, errors, warnings } = parseInlineRuleList('process_path:/usr/bin/curl');
		expect(errors).toHaveLength(0);
		expect(rules).toEqual([{ process_path: ['/usr/bin/curl'] }]);
		expect(warnings).toHaveLength(1);
		expect(warnings[0]).toContain('локальным процессам');
	});
});

// ── IPv6 ───────────────────────────────────────────────────────
describe('IPv6 support', () => {
	it('accepts bare IPv6 address without CIDR', () => {
		const { rules, errors } = parseInlineRuleList('2606:4700::1');
		expect(errors).toHaveLength(0);
		expect(rules).toEqual([{ ip_cidr: ['2606:4700::1'] }]);
	});

	it('accepts IPv6 CIDR notation', () => {
		const { rules, errors } = parseInlineRuleList('2606:4700::/32');
		expect(errors).toHaveLength(0);
		expect(rules).toEqual([{ ip_cidr: ['2606:4700::/32'] }]);
	});

	it('accepts ip: prefix with IPv6 address', () => {
		const { rules, errors } = parseInlineRuleList('ip:2606:4700::1');
		expect(errors).toHaveLength(0);
		expect(rules).toEqual([{ ip_cidr: ['2606:4700::1'] }]);
	});

	it('rejects IPv6 CIDR with out-of-range mask (129)', () => {
		const { rules, errors } = parseInlineRuleList('cidr:2606:4700::/129');
		expect(rules).toHaveLength(0);
		expect(errors).toHaveLength(1);
		expect(errors[0]).toContain('cidr');
	});
});

// ── port_range ─────────────────────────────────────────────────
describe('port_range (not yet supported)', () => {
	it('warns that port_range is unsupported, does not generate rule', () => {
		const { rules, errors, warnings } = parseInlineRuleList('port_range:1000-2000');
		expect(rules).toHaveLength(0);
		expect(errors).toHaveLength(0);
		expect(warnings).toHaveLength(1);
		expect(warnings[0]).toContain('port_range');
		expect(warnings[0]).toContain('не поддерживается');
	});
});
