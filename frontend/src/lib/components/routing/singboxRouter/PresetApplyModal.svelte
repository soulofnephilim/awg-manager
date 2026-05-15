<script lang="ts">
	import Modal from '$lib/components/ui/Modal.svelte';
	import { Dropdown, type DropdownOption } from '$lib/components/ui';
	import type { SingboxRouterPreset } from '$lib/types';
	import type { OutboundGroup } from './outboundOptions';

	interface DnsServerSummary { tag: string; type: string; server: string }

	interface Props {
		presets: SingboxRouterPreset[];
		outboundOptions: OutboundGroup[];
		dnsServers: DnsServerSummary[];
		onClose: () => void;
		onApply: (params: {
			presetIds: string[];
			outboundTag: string;
			createDnsRule: boolean;
			dnsServerTag: string | null;
		}) => Promise<void> | void;
		onCreateDnsServer?: () => void;
	}
	let { presets, outboundOptions, dnsServers, onClose, onApply, onCreateDnsServer }: Props = $props();

	const title = $derived(
		presets.length === 1
			? `Применить пресет: ${presets[0].name}`
			: `Применить ${presets.length} пресета`,
	);

	// Show outbound dropdown only when at least one preset routes to a tunnel.
	const needsOutbound = $derived(
		presets.some((p) => p.rules.some((r) => r.actionTarget === 'tunnel')),
	);

	// DNS section is meaningful only for tunnel-action presets — DNS-rule
	// for direct/reject is irrelevant (no domain resolution path involved).
	const showDnsSection = $derived(needsOutbound);

	const allReject = $derived(presets.every((p) => p.rules.every((r) => r.actionTarget === 'reject')));
	const allDirect = $derived(presets.every((p) => p.rules.every((r) => r.actionTarget === 'direct')));
	const specialHint = $derived.by(() => {
		if (allReject) return 'Совпадающий трафик будет заблокирован (action: reject). Выбор туннеля не требуется.';
		if (allDirect) return 'Совпадающий трафик пойдёт мимо VPN (direct). Выбор туннеля не требуется.';
		return '';
	});

	const outboundDropdownOptions = $derived<DropdownOption[]>([
		{ value: '', label: '— выберите —' },
		...outboundOptions.flatMap((g) =>
			g.items.map((i) => ({ value: i.value, label: i.label, group: g.group })),
		),
	]);

	const dnsDropdownOptions = $derived<DropdownOption[]>([
		{ value: '', label: '— выберите DNS-сервер —' },
		...dnsServers.map((d) => ({ value: d.tag, label: `${d.tag} (${d.server})` })),
	]);

	let selectedOutbound = $state('');
	let createDnsRule = $state(false);
	let selectedDnsServer = $state('');
	let busy = $state(false);
	let error = $state('');

	const applyDisabled = $derived(
		busy
		|| (needsOutbound && !selectedOutbound)
		|| (createDnsRule && !selectedDnsServer),
	);

	// Aggregate preview: unique rule_set tags + flat rules list across all selected.
	const previewRuleSets = $derived(
		Array.from(new Map(presets.flatMap((p) => p.ruleSets.map((rs) => [rs.tag, rs])).values()).values()),
	);
	const previewRules = $derived(
		presets.flatMap((p) => p.rules.map((r) => ({ ...r, presetName: p.name }))),
	);
	const previewNotices = $derived(
		presets.map((p) => p.notice).filter((n): n is string => !!n),
	);

	async function apply(): Promise<void> {
		busy = true;
		error = '';
		try {
			await onApply({
				presetIds: presets.map((p) => p.id),
				outboundTag: selectedOutbound,
				createDnsRule,
				dnsServerTag: createDnsRule ? selectedDnsServer : null,
			});
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy = false;
		}
	}
</script>

<Modal open onclose={onClose} {title}>
	<div class="form">
		<div class="preview">
			<div class="preview-head">Будет добавлено:</div>
			<ul>
				{#each previewRuleSets as rs (rs.tag)}
					<li>Rule set <code>{rs.tag}</code> (если ещё не добавлен)</li>
				{/each}
				{#each previewRules as r, i (i)}
					<li>
						Правило <code>rule_set: {r.ruleSetRef} → {r.actionTarget === 'tunnel' ? '«выбранный туннель»' : r.actionTarget}</code>
					</li>
				{/each}
			</ul>
		</div>

		{#each previewNotices as notice (notice)}
			<div class="notice">{notice}</div>
		{/each}

		{#if needsOutbound}
			<label class="field">
				<div class="lbl">Направить трафик в</div>
				<Dropdown bind:value={selectedOutbound} options={outboundDropdownOptions} fullWidth />
			</label>
		{:else if specialHint}
			<div class="special-hint">{specialHint}</div>
		{/if}

		{#if showDnsSection}
			<div class="dns-section">
				<label class="checkbox-row">
					<input type="checkbox" bind:checked={createDnsRule} aria-label="Также создать DNS-правило для доменов" />
					<span>Также создать DNS-правило для доменов</span>
				</label>
				{#if createDnsRule}
					<div class="field">
						<div class="lbl">DNS-сервер для правила</div>
						<div class="dns-row">
							<Dropdown bind:value={selectedDnsServer} options={dnsDropdownOptions} fullWidth />
							{#if onCreateDnsServer}
								<button type="button" class="btn-create-dns" onclick={onCreateDnsServer}>
									+ Создать DNS-сервер
								</button>
							{/if}
						</div>
					</div>
				{/if}
			</div>
		{/if}

		{#if error}<div class="error">{error}</div>{/if}

		<div class="actions">
			<button class="btn btn-secondary" onclick={onClose} type="button">Отмена</button>
			<button class="btn btn-primary" onclick={apply} disabled={applyDisabled} type="button">
				Применить
			</button>
		</div>
	</div>
</Modal>

<style>
	.form { display: grid; gap: 0.75rem; min-width: 0; }
	.preview { padding: 0.75rem; background: var(--bg); border-radius: 4px; font-size: 0.85rem; }
	.preview-head { color: var(--muted-text); margin-bottom: 0.35rem; }
	.preview ul { margin: 0; padding-left: 1.25rem; color: var(--text); }
	.preview li { margin: 0.15rem 0; }
	.preview code {
		font-family: ui-monospace, monospace; font-size: 0.8rem;
		color: var(--accent, #3b82f6);
	}
	.notice {
		padding: 0.5rem 0.75rem;
		background: rgba(224, 175, 104, 0.12);
		border-left: 3px solid var(--warning); border-radius: 4px;
		font-size: 0.8rem; line-height: 1.4; color: var(--text);
	}
	.special-hint {
		padding: 0.5rem 0.75rem; background: var(--bg);
		border-left: 2px solid var(--accent, #3b82f6); border-radius: 4px;
		font-size: 0.8rem; color: var(--muted-text);
	}
	.field { display: grid; gap: 0.25rem; }
	.lbl { font-size: 0.75rem; color: var(--muted-text); }
	.dns-section { display: grid; gap: 0.5rem; padding-top: 0.5rem; border-top: 1px solid var(--color-border); }
	.checkbox-row { display: inline-flex; align-items: center; gap: 0.4rem; font-size: 0.85rem; cursor: pointer; }
	.dns-row { display: flex; gap: 0.5rem; align-items: center; }
	.dns-row :global(.dropdown-wrap) { flex: 1; }
	.btn-create-dns {
		background: transparent; color: var(--accent, #6cb6ff);
		border: 1px solid currentColor; padding: 0.3rem 0.6rem;
		border-radius: 4px; font-size: 0.78rem; cursor: pointer; white-space: nowrap;
	}
	.btn-create-dns:hover { background: rgba(108, 182, 255, 0.08); }
	.error { color: var(--danger, #dc2626); font-size: 0.85rem; }
	.actions { display: flex; justify-content: flex-end; gap: 0.5rem; }
</style>
