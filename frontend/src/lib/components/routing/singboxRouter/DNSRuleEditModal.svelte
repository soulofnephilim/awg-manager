<script lang="ts">
	import {
		Button,
		Dropdown,
		ChipMultiSelect,
		SegmentedControl,
		type DropdownOption,
		type ChipOption,
		type SegmentedOption,
	} from '$lib/components/ui';
	import SingboxSettingsModal from './SingboxSettingsModal.svelte';
	import type { SingboxRouterDNSRule, SingboxRouterDNSServer, SingboxRouterRuleSet } from '$lib/types';
	import { dnsServerSubtitle } from '$lib/components/sb-router/dnsServerDetourDisplay';

	interface Props {
		rule?: SingboxRouterDNSRule;
		servers: SingboxRouterDNSServer[];
		availableRuleSets: SingboxRouterRuleSet[];
		/**
		 * Per-tag count of how many *other* DNS rules reference each rule_set.
		 * The currently edited rule must be excluded by the caller (use
		 * computeRuleSetUsage with excludeIndex=editIndex). Empty map is fine
		 * — all sets render as unused.
		 */
		ruleSetUsage?: Map<string, number>;
		onClose: () => void;
		onSave: (rule: SingboxRouterDNSRule) => Promise<void> | void;
	}
	let { rule, servers, availableRuleSets, ruleSetUsage, onClose, onSave }: Props = $props();

	function normalizeTags(tags: string[]): string[] {
		return [...new Set(tags.map((s) => s.trim()).filter(Boolean))];
	}

	const serverOptions = $derived<DropdownOption[]>([
		{ value: '', label: '— выберите —' },
		...servers.map((s) => ({
			value: s.tag,
			label: s.tag,
			description: dnsServerSubtitle(s),
		})),
	]);

	// svelte-ignore state_referenced_locally
	let ruleSetTags = $state<string[]>(rule?.rule_set ?? []);
	const ruleSetOptions = $derived<ChipOption[]>(
		availableRuleSets.map((rs) => ({
			value: rs.tag,
			label: rs.tag,
			usedCount: ruleSetUsage?.get(rs.tag) ?? 0,
		})),
	);
	// svelte-ignore state_referenced_locally
	let domainSuffixStr = $state((rule?.domain_suffix ?? []).join('\n'));
	// svelte-ignore state_referenced_locally
	let domainStr = $state((rule?.domain ?? []).join('\n'));
	// svelte-ignore state_referenced_locally
	let domainKeywordStr = $state((rule?.domain_keyword ?? []).join(', '));
	// svelte-ignore state_referenced_locally
	let domainRegexStr = $state((rule?.domain_regex ?? []).join('\n'));
	// svelte-ignore state_referenced_locally
	let queryTypeStr = $state((rule?.query_type ?? []).join(', '));

	function initAction(r?: SingboxRouterDNSRule): 'route' | 'block' {
		if (r?.action === 'reject' || r?.action === 'predefined') return 'block';
		return 'route';
	}
	// A rule with NO matchers matches every query = catch-all («всё остальное»).
	// We surface the simplified «catch-all» mode only for the route-to-server
	// shape; a matcher-less block is unusual and stays in the full editor.
	function isMatcherless(r?: SingboxRouterDNSRule): boolean {
		if (!r) return false;
		return !(
			(r.rule_set?.length ?? 0) > 0 ||
			(r.domain_suffix?.length ?? 0) > 0 ||
			(r.domain?.length ?? 0) > 0 ||
			(r.domain_keyword?.length ?? 0) > 0 ||
			(r.domain_regex?.length ?? 0) > 0 ||
			(r.query_type?.length ?? 0) > 0 ||
			// source_ip_cidr is a matcher too (backend dnsRuleHasMatcher counts it);
			// a source-scoped rule must not open in catch-all mode, which would drop
			// the field on save (bug #445 review).
			(r.source_ip_cidr?.length ?? 0) > 0
		);
	}
	function initMode(r?: SingboxRouterDNSRule): 'matchers' | 'catchall' {
		return r && isMatcherless(r) && initAction(r) === 'route' ? 'catchall' : 'matchers';
	}
	function initBlockMethod(r?: SingboxRouterDNSRule): 'nxdomain' | 'refused' | 'drop' {
		if (r?.action === 'predefined') return 'nxdomain';
		if (r?.action === 'reject' && r?.method === 'drop') return 'drop';
		return 'refused';
	}
	// svelte-ignore state_referenced_locally
	let mode = $state<'matchers' | 'catchall'>(initMode(rule));
	const catchAll = $derived(mode === 'catchall');
	// svelte-ignore state_referenced_locally
	let action = $state<'route' | 'block'>(initAction(rule));
	// svelte-ignore state_referenced_locally
	let blockMethod = $state<'nxdomain' | 'refused' | 'drop'>(initBlockMethod(rule));
	// svelte-ignore state_referenced_locally
	let server = $state(rule?.server ?? '');

	const blockMethodOptions = [
		{ value: 'nxdomain', label: 'NXDOMAIN (нет такого домена)' },
		{ value: 'refused', label: 'REFUSED' },
		{ value: 'drop', label: 'Drop (без ответа)' },
	];

	const actionOptions: SegmentedOption<'route' | 'block'>[] = [
		{ value: 'route', label: 'Резолвить' },
		{ value: 'block', label: 'Заблокировать' },
	];

	const modeOptions: SegmentedOption<'matchers' | 'catchall'>[] = [
		{ value: 'matchers', label: 'По условиям' },
		{ value: 'catchall', label: 'Catch-all (всё остальное)' },
	];

	let busy = $state(false);
	let error = $state('');

	// Snapshot initial state for isDirty detection
	let initialRuleSetTagsSnapshot = $state<string[]>([]);
	let initialDomainSuffixStr = $state('');
	let initialDomainStr = $state('');
	let initialDomainKeywordStr = $state('');
	let initialDomainRegexStr = $state('');
	let initialQueryTypeStr = $state('');
	let initialAction: 'route' | 'block' = $state('route');
	let initialBlockMethod: 'nxdomain' | 'refused' | 'drop' = $state('refused');
	let initialServer = $state('');
	let initialMode: 'matchers' | 'catchall' = $state('matchers');

	// Initialize snapshot when modal opens
	$effect(() => {
		if (rule) {
			initialRuleSetTagsSnapshot = [...(rule.rule_set ?? [])];
			initialDomainSuffixStr = (rule.domain_suffix ?? []).join('\n');
			initialDomainStr = (rule.domain ?? []).join('\n');
			initialDomainKeywordStr = (rule.domain_keyword ?? []).join(', ');
			initialDomainRegexStr = (rule.domain_regex ?? []).join('\n');
			initialQueryTypeStr = (rule.query_type ?? []).join(', ');
			initialAction = initAction(rule);
			initialBlockMethod = initBlockMethod(rule);
			initialServer = rule.server ?? '';
			initialMode = initMode(rule);
		} else {
			initialRuleSetTagsSnapshot = [];
			initialDomainSuffixStr = '';
			initialDomainStr = '';
			initialDomainKeywordStr = '';
			initialDomainRegexStr = '';
			initialQueryTypeStr = '';
			initialAction = 'route';
			initialBlockMethod = 'refused';
			initialServer = '';
			initialMode = 'matchers';
		}
	});

	const isDirty = $derived.by(() => {
		return (
			normalizeTags(ruleSetTags).join(',') !== normalizeTags(initialRuleSetTagsSnapshot).join(',') ||
			domainSuffixStr !== initialDomainSuffixStr ||
			domainStr !== initialDomainStr ||
			domainKeywordStr !== initialDomainKeywordStr ||
			domainRegexStr !== initialDomainRegexStr ||
			queryTypeStr !== initialQueryTypeStr ||
			action !== initialAction ||
			blockMethod !== initialBlockMethod ||
			server !== initialServer ||
			mode !== initialMode
		);
	});

	async function save(): Promise<void> {
		busy = true;
		error = '';
		try {
			const rule_set = normalizeTags(ruleSetTags);
			const domain_suffix = domainSuffixStr.split('\n').map((s) => s.trim()).filter(Boolean);
			const domain = domainStr.split('\n').map((s) => s.trim()).filter(Boolean);
			const domain_keyword = domainKeywordStr.split(',').map((s) => s.trim()).filter(Boolean);
			const domain_regex = domainRegexStr.split('\n').map((s) => s.trim()).filter(Boolean);
			const query_type = queryTypeStr.split(',').map((s) => s.trim().toUpperCase()).filter(Boolean);

			const hasMatcher =
				rule_set.length > 0 ||
				domain_suffix.length > 0 ||
				domain.length > 0 ||
				domain_keyword.length > 0 ||
				domain_regex.length > 0 ||
				query_type.length > 0;
			// Catch-all mode intentionally ships ZERO matchers (matches everything);
			// otherwise at least one matcher is required.
			if (!catchAll && !hasMatcher) {
				error = 'Нужен хотя бы один matcher';
				busy = false;
				return;
			}

			// In catch-all mode the matcher inputs are hidden — never serialize them,
			// even if the user had typed something before switching mode.
			const built: SingboxRouterDNSRule = catchAll
				? {}
				: {
						rule_set: rule_set.length ? rule_set : undefined,
						domain_suffix: domain_suffix.length ? domain_suffix : undefined,
						domain: domain.length ? domain : undefined,
						domain_keyword: domain_keyword.length ? domain_keyword : undefined,
						domain_regex: domain_regex.length ? domain_regex : undefined,
						query_type: query_type.length ? query_type : undefined,
					};

			if (catchAll || action === 'route') {
				if (!server) { error = 'Выберите DNS сервер'; busy = false; return; }
				built.action = 'route';
				built.server = server;
			} else if (blockMethod === 'nxdomain') {
				built.action = 'predefined';
				built.rcode = 'NXDOMAIN';
			} else if (blockMethod === 'drop') {
				built.action = 'reject';
				built.method = 'drop';
			} else {
				built.action = 'reject';
				built.method = 'default';
			}

			await onSave(built);
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy = false;
		}
	}
</script>

<SingboxSettingsModal
	title={rule ? 'Редактировать DNS правило' : 'Новое DNS правило'}
	onClose={onClose}
	size="lg"
	hasUnsavedChanges={() => isDirty}
>
	<div class="form">
		<div class="section-label">Тип правила</div>
		<SegmentedControl
			value={mode}
			options={modeOptions}
			ariaLabel="Тип DNS правила"
			onchange={(next) => (mode = next)}
		/>

		{#if catchAll}
			<div class="warn">
				Правило без условий совпадает со <b>всеми</b> запросами. Любые правила
				<b>ниже</b> него в списке игнорируются — держите его последним.
			</div>
			<label class="field">
				<div class="lbl">DNS сервер (для всех запросов)</div>
				<Dropdown bind:value={server} options={serverOptions} fullWidth />
			</label>
			<div class="hint">
				Для простого запасного сервера обычно достаточно «Final-сервера» в «DNS по умолчанию».
				Если задать и его, и catch-all правило — правило важнее (проверяется раньше final).
			</div>
		{:else}
			<div class="section-label">Matchers (минимум один)</div>

			<!-- div, не label: клик по любой не-интерактивной части label активирует
			     его первый labelable-элемент — крестик ПЕРВОГО чипа, т.е. клик по
			     названию любого rule-set удалял первый (bug #446). -->
			<div class="field">
				<div class="lbl">Rule sets</div>
				<ChipMultiSelect
					values={ruleSetTags}
					options={ruleSetOptions}
					onchange={(next) => (ruleSetTags = next)}
					placeholder="не выбрано"
					allowOrphans
				/>
			</div>

			<label class="field">
				<div class="lbl">Domain suffix</div>
				<textarea bind:value={domainSuffixStr} rows="3" placeholder="по одному на строке: .youtube.com"></textarea>
			</label>

			<label class="field">
				<div class="lbl">Domain (точное совпадение)</div>
				<textarea bind:value={domainStr} rows="2" placeholder="example.com"></textarea>
			</label>

			<label class="field">
				<div class="lbl">Domain keyword (через запятую)</div>
				<input bind:value={domainKeywordStr} placeholder="tracker, analytics" />
			</label>

			<label class="field">
				<div class="lbl">Domain regex (по строке)</div>
				<textarea bind:value={domainRegexStr} rows="2" placeholder={"^ads?\\d+\\."}></textarea>
			</label>

			<label class="field">
				<div class="lbl">Query type (через запятую)</div>
				<input bind:value={queryTypeStr} placeholder="A, AAAA, HTTPS" />
			</label>

			<div class="action-section">
				<div class="section-label">Действие</div>
				<SegmentedControl
					value={action}
					options={actionOptions}
					ariaLabel="Действие DNS правила"
					onchange={(next) => (action = next)}
				/>

				{#if action === 'route'}
					<label class="field">
						<div class="lbl">DNS сервер</div>
						<Dropdown bind:value={server} options={serverOptions} fullWidth />
					</label>
				{:else}
					<label class="field">
						<div class="lbl">Метод блокировки</div>
						<Dropdown bind:value={blockMethod} options={blockMethodOptions} fullWidth />
					</label>
				{/if}
			</div>
		{/if}

		{#if error}<div class="error">{error}</div>{/if}
	</div>

	{#snippet actions()}
		<Button variant="ghost" size="md" onclick={onClose} type="button">Отмена</Button>
		<Button variant="primary" size="md" onclick={save} disabled={busy} loading={busy} type="button">
			Сохранить
		</Button>
	{/snippet}
</SingboxSettingsModal>
