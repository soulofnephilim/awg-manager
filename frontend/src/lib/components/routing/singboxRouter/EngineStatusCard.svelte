<script lang="ts">
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores/notifications';
	import type {
		RouterPolicy,
		SingboxRouterStatus,
		SingboxRouterSettings,
		SingboxRouterWANInterface,
	} from '$lib/types';
	import Toggle from '$lib/components/ui/Toggle.svelte';
	import Dropdown from '$lib/components/ui/Dropdown.svelte';
	import type { DropdownOption } from '$lib/components/ui/Dropdown.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';

	interface Props {
		status: SingboxRouterStatus | null;
		settings: SingboxRouterSettings | null;
		onChange: () => Promise<void> | void;
		onOpenRefreshSettings: () => void;
	}
	let { status, settings, onChange, onOpenRefreshSettings }: Props = $props();

	let busy = $state(false);
	let policies = $state<RouterPolicy[]>([]);
	let wanInterfaces = $state<SingboxRouterWANInterface[]>([]);
	let creatingPolicy = $state(false);
	let createModalOpen = $state(false);
	let createDescription = $state('awgm-router');

	async function loadPolicies(): Promise<void> {
		try {
			policies = await api.singboxRouterListPolicies();
		} catch (e) {
			notifications.error((e as Error).message);
			policies = [];
		}
	}

	async function loadWANInterfaces(): Promise<void> {
		try {
			wanInterfaces = await api.singboxRouterListWANInterfaces();
		} catch (e) {
			notifications.error((e as Error).message);
			wanInterfaces = [];
		}
	}

	$effect(() => {
		// Load policies on mount and whenever the user might have created
		// one externally (Access Policies tab). Cheap call; one per mount.
		loadPolicies();
		loadWANInterfaces();
	});

	async function selectPolicy(name: string): Promise<void> {
		if (!settings) return;
		busy = true;
		try {
			await api.singboxRouterPutSettings({ ...settings, policyName: name });
			await onChange();
		} catch (e) {
			notifications.error((e as Error).message);
		} finally {
			busy = false;
		}
	}

	// Toggle wanAutoDetect. Switching to pinned mode requires also
	// supplying a non-empty wanInterface — backend validator rejects
	// {auto:false, interface:""}. We pick the first WAN as a default
	// kernel name so the toggle never leaves the user with an invalid
	// pending state.
	async function setWANAutoDetect(next: boolean): Promise<void> {
		if (!settings) return;
		busy = true;
		try {
			if (next) {
				await api.singboxRouterPutSettings({
					...settings,
					wanAutoDetect: true,
					wanInterface: '',
				});
			} else {
				const first = wanInterfaces[0]?.name ?? '';
				if (!first) {
					notifications.error(
						'Не удалось получить список WAN-интерфейсов с роутера',
					);
					return;
				}
				await api.singboxRouterPutSettings({
					...settings,
					wanAutoDetect: false,
					wanInterface: first,
				});
			}
			await onChange();
		} catch (e) {
			notifications.error((e as Error).message);
		} finally {
			busy = false;
		}
	}

	async function selectWAN(kernelName: string): Promise<void> {
		if (!settings || !kernelName) return;
		busy = true;
		try {
			await api.singboxRouterPutSettings({
				...settings,
				wanAutoDetect: false,
				wanInterface: kernelName,
			});
			await onChange();
		} catch (e) {
			notifications.error((e as Error).message);
		} finally {
			busy = false;
		}
	}

	function openCreateModal(): void {
		createDescription = 'awgm-router';
		createModalOpen = true;
	}

	async function confirmCreatePolicy(): Promise<void> {
		if (creatingPolicy || !settings) return;
		const trimmed = createDescription.trim();
		if (!trimmed) return;
		creatingPolicy = true;
		try {
			const created = await api.singboxRouterCreatePolicy(trimmed);
			await loadPolicies();
			await api.singboxRouterPutSettings({ ...settings, policyName: created.name });
			await onChange();
			createModalOpen = false;
			notifications.success(
				'Политика создана. Отметьте устройства которые должны идти через VPN.',
			);
		} catch (e) {
			notifications.error((e as Error).message);
		} finally {
			creatingPolicy = false;
		}
	}

	async function toggleEnabled(next: boolean): Promise<void> {
		if (busy || !status) return;
		busy = true;
		try {
			if (next) await api.singboxRouterEnable();
			else await api.singboxRouterDisable();
			await onChange();
		} catch (e) {
			notifications.error((e as Error).message);
		} finally {
			busy = false;
		}
	}

	const canEnable = $derived(
		!!status?.netfilterAvailable &&
			!!status?.tproxyTargetAvailable &&
			!!settings?.policyName,
	);

	const enableTooltip = $derived.by(() => {
		if (!status?.netfilterAvailable) return 'Netfilter недоступен';
		if (!status?.tproxyTargetAvailable) return 'TPROXY target недоступен';
		if (!settings?.policyName) return 'Сначала выберите или создайте политику';
		return '';
	});

	// True when settings reference a policy that no longer exists in NDMS
	// (manual deletion, NDMS reset, etc.). Distinct from "never configured":
	// here we have a stale name that needs an explicit recovery action.
	const policyMissing = $derived(
		!!settings?.policyName && status?.policyExists === false,
	);

	const policyOptions = $derived<DropdownOption[]>(
		policies.map((p) => {
			const meta = p.mark
				? `${p.mark} · ${p.deviceCount} устр.`
				: `${p.deviceCount} устр.`;
			return {
				value: p.name,
				label: p.description || p.name,
				description: meta,
			};
		}),
	);

	// WAN dropdown options. Up/down is info-only (label suffix); doesn't
	// gate selection — user picks freely. We surface kernel name as the
	// secondary text so the user understands they're picking a stable
	// kernel identifier, not the NDMS ID.
	const wanOptions = $derived<DropdownOption[]>(
		wanInterfaces.map((iface) => ({
			value: iface.name,
			label: iface.label || iface.id || iface.name,
			description: `${iface.id || iface.name} · ${iface.name} · ${iface.up ? 'up' : 'down'}`,
		})),
	);
</script>

{#if status}
	<div class="card">
		<div class="top-row">
			<div class="toggle-group" title={enableTooltip}>
				<Toggle
					checked={status.enabled}
					onchange={(v) => toggleEnabled(v)}
					disabled={busy || !canEnable}
					size="md"
				/>
				<span class="label">Движок {status.enabled ? 'включён' : 'выключен'}</span>
			</div>
			<div class="stats">
				{status.ruleCount} правил · {status.ruleSetCount} lists · {status.outboundCompositeCount}
				outbound
			</div>
			<button
				class="gear"
				onclick={onOpenRefreshSettings}
				aria-label="Настройки обновления"
				title="Настройки автообновления"
			>
				<svg
					width="18"
					height="18"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
				>
					<circle cx="12" cy="12" r="3" />
					<path
						d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"
					/>
				</svg>
			</button>
		</div>

		{#if status.netfilterAvailable && !status.tproxyTargetAvailable}
			<div class="dep-warning">
				<span
					>TPROXY target недоступен в iptables. Убедитесь что модуль <code
						>xt_TPROXY</code
					> загружен.</span
				>
			</div>
		{/if}

		{#if policyMissing}
			<div class="dep-warning issue-error policy-missing-row">
				<span>
					Policy <strong>«{settings?.policyName}»</strong> не найдена в NDMS —
					возможно, удалена вручную. Маршрутизация не запустится без неё.
				</span>
				<Button
					variant="primary"
					size="sm"
					onclick={openCreateModal}
					disabled={creatingPolicy || busy}
				>
					Создать «awgm-router»
				</Button>
			</div>
		{/if}

		{#if settings}
			<div class="policy-block">
				<div class="control-label">NDMS Access Policy</div>
				<div class="policy-row">
					<div class="policy-dropdown">
						<Dropdown
							value={settings.policyName}
							options={policyOptions}
							placeholder="— выберите политику —"
							disabled={busy || creatingPolicy}
							onchange={(v) => selectPolicy(v)}
							fullWidth
						/>
					</div>
					<Button
						variant="ghost"
						size="md"
						onclick={openCreateModal}
						disabled={creatingPolicy || busy}
					>
						+ Создать новую
					</Button>
				</div>

				{#if status.policyMark}
					<div class="mark-info">Текущий mark: <code>{status.policyMark}</code></div>
				{/if}

				{#if settings.policyName}
					<div class="policy-link-row">
						<span class="setting-description">
							Привязка устройств к политике
							<strong>«{settings.policyName}»</strong>
							настраивается на отдельной странице.
						</span>
						<Button variant="ghost" size="sm" href="/routing?tab=policy">
							Управление устройствами →
						</Button>
					</div>
				{/if}
			</div>

			<div class="wan-block">
				<div class="control-label">WAN-интерфейс для outbound трафика</div>
				<div class="wan-row">
					<Toggle
						checked={settings.wanAutoDetect}
						onchange={(v) => setWANAutoDetect(v)}
						disabled={busy}
						size="sm"
					/>
					<span class="wan-toggle-label">
						{settings.wanAutoDetect
							? 'Авто-определение (sing-box выбирает сам)'
							: 'Использовать конкретный WAN'}
					</span>
				</div>
				{#if !settings.wanAutoDetect}
					<div class="wan-dropdown">
						<Dropdown
							value={settings.wanInterface ?? ''}
							options={wanOptions}
							placeholder={wanInterfaces.length === 0
								? '— нет доступных WAN —'
								: '— выберите WAN —'}
							disabled={busy || wanInterfaces.length === 0}
							onchange={(v) => selectWAN(v)}
							fullWidth
						/>
					</div>
					<div class="wan-hint">
						В sing-box передаётся kernel-имя (например, <code>ppp0</code>) —
						оно стабильно при пересоздании интерфейсов в NDMS.
					</div>
				{/if}
			</div>
		{/if}

		{#if status.issues && status.issues.length > 0}
			<div class="issues">
				{#each status.issues as issue}
					{#if issue.kind !== 'policy-missing'}
						<div class="issue issue-{issue.severity}">{issue.message}</div>
					{/if}
				{/each}
			</div>
		{/if}
	</div>
{/if}

<Modal
	open={createModalOpen}
	title="Создать новую access policy"
	size="sm"
	onclose={() => (createModalOpen = false)}
>
	<div class="create-modal-body">
		<Input
			label="Имя политики"
			bind:value={createDescription}
			placeholder="awgm-router"
			disabled={creatingPolicy}
		/>
		<p class="create-hint">
			Политика будет создана в NDMS. К ней будет автоматически привязан текущий
			WAN-интерфейс — это нужно чтобы NDMS начал маркировать трафик связанных
			устройств.
		</p>
	</div>
	{#snippet actions()}
		<Button
			variant="ghost"
			size="md"
			onclick={() => (createModalOpen = false)}
			disabled={creatingPolicy}
		>
			Отмена
		</Button>
		<Button
			variant="primary"
			size="md"
			onclick={confirmCreatePolicy}
			disabled={creatingPolicy || !createDescription.trim()}
		>
			{creatingPolicy ? 'Создание…' : 'Создать'}
		</Button>
	{/snippet}
</Modal>

<style>
	.card {
		background: var(--surface-bg);
		padding: 1rem 1.25rem;
		border-radius: 8px;
		margin-bottom: 1rem;
	}
	.top-row {
		display: flex;
		align-items: center;
		gap: 1rem;
	}
	.toggle-group {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.label {
		font-weight: 600;
	}
	.stats {
		margin-left: auto;
		font-size: 0.85rem;
		color: var(--muted-text);
	}
	.gear {
		background: transparent;
		border: none;
		color: var(--muted-text);
		cursor: pointer;
		padding: 0.25rem;
		display: inline-flex;
	}
	.gear:hover {
		color: var(--text);
	}
	.dep-warning {
		margin-top: 0.75rem;
		padding: 0.5rem 0.75rem;
		background: rgba(224, 175, 104, 0.12);
		border-left: 3px solid var(--warning);
		border-radius: 4px;
		font-size: 0.85rem;
		color: var(--text);
	}
	.dep-warning.issue-error {
		background: rgba(247, 118, 142, 0.12);
		border-left-color: var(--error, var(--danger));
	}
	.dep-warning code {
		background: var(--bg);
		padding: 1px 6px;
		border-radius: 3px;
		font-size: 0.8rem;
	}
	.policy-missing-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		flex-wrap: wrap;
	}
	.policy-missing-row > span {
		flex: 1 1 auto;
		min-width: 0;
	}
	.policy-block {
		margin-top: 1rem;
	}
	.control-label {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.5px;
		color: var(--muted-text);
		margin-bottom: 0.375rem;
	}
	.policy-row {
		display: flex;
		gap: 0.5rem;
		align-items: flex-start;
	}
	.policy-dropdown {
		flex: 1;
		min-width: 0;
	}
	.create-modal-body {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.create-hint {
		margin: 0;
		font-size: 0.8rem;
		color: var(--muted-text);
		line-height: 1.4;
	}
	.mark-info {
		margin-top: 0.375rem;
		font-size: 0.75rem;
		color: var(--muted-text);
	}
	.mark-info code {
		font-family: var(--font-mono, ui-monospace, monospace);
		background: var(--bg);
		padding: 1px 6px;
		border-radius: 3px;
	}
	.policy-link-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 0.5rem;
		flex-wrap: wrap;
	}
	.wan-block {
		margin-top: 1rem;
		padding-top: 1rem;
		border-top: 1px solid var(--border, rgba(255, 255, 255, 0.08));
	}
	.wan-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.wan-toggle-label {
		font-size: 0.85rem;
		color: var(--text);
	}
	.wan-dropdown {
		margin-top: 0.5rem;
	}
	.wan-hint {
		margin-top: 0.375rem;
		font-size: 0.75rem;
		color: var(--muted-text);
		line-height: 1.4;
	}
	.wan-hint code {
		font-family: var(--font-mono, ui-monospace, monospace);
		background: var(--bg);
		padding: 1px 6px;
		border-radius: 3px;
	}
	.issues {
		margin-top: 1rem;
		display: grid;
		gap: 0.25rem;
	}
	.issue {
		padding: 0.4rem 0.6rem;
		border-radius: 4px;
		font-size: 0.85rem;
	}
	.issue-warning {
		background: rgba(224, 175, 104, 0.12);
		border-left: 3px solid var(--warning);
	}
	.issue-error {
		background: rgba(247, 118, 142, 0.12);
		border-left: 3px solid var(--error, var(--danger));
	}
</style>
