<script lang="ts">
	import { singboxWizard } from '$lib/stores/singboxWizard';
	import { singboxRouter } from '$lib/stores/singboxRouter';
	import { api } from '$lib/api/client';
	import type { WizardStep, SingboxRouterDNSType, SingboxRouterDNSStrategy } from '$lib/types';
	import { runWizard, WizardError } from './wizardOrchestrator';
	import type { OrchestratorApi } from './wizardOrchestrator';
	import StepPresets from './StepPresets.svelte';
	import StepTunnel from './StepTunnel.svelte';
	import StepPolicy from './StepPolicy.svelte';
	import StepDevices from './StepDevices.svelte';
	import StepSummary from './StepSummary.svelte';
	import StepApplying from './StepApplying.svelte';
	import StepSuccess from './StepSuccess.svelte';
	import StepError from './StepError.svelte';

	const wizardOpen = singboxWizard.open;
	const wizardState = singboxWizard.state;
	const presetsStore = singboxRouter.presets;

	const STEPS: WizardStep[] = ['presets', 'tunnel', 'policy', 'devices', 'summary'];

	const stepIdx = $derived(STEPS.indexOf($wizardState.step));
	const presets = $derived($presetsStore);

	function nextStep(): void {
		const idx = STEPS.indexOf($wizardState.step);
		if (idx >= 0 && idx < STEPS.length - 1) {
			singboxWizard.setStep(STEPS[idx + 1]);
		}
	}

	function prevStep(): void {
		const idx = STEPS.indexOf($wizardState.step);
		if (idx > 0) {
			singboxWizard.setStep(STEPS[idx - 1]);
		}
	}

	function canAdvance(): boolean {
		const s = $wizardState;
		switch (s.step) {
			case 'presets':
				return s.presetIds.length > 0;
			case 'tunnel':
				return s.tunnelTag !== null;
			case 'policy':
				if (s.policyMode === 'create') return s.policyName.trim().length > 0;
				return s.existingPolicyName !== null;
			case 'devices':
				// For 'existing' mode, empty selection is a valid "remove all from policy"
				// intent. For 'create' mode, require at least 1 device (StepDevices defaults to []).
				if (s.policyMode === 'existing') return true;
				return s.deviceMacs.length > 0;
			case 'summary':
				return true;
			default:
				return false;
		}
	}

	const orchestratorApi: OrchestratorApi = {
		singboxRouterListPolicies: () => api.singboxRouterListPolicies(),
		singboxRouterCreatePolicy: (description: string) =>
			api.singboxRouterCreatePolicy(description),
		singboxRouterGetSettings: () => api.singboxRouterGetSettings(),
		singboxRouterPutSettings: (settings) =>
			api.singboxRouterPutSettings({
				enabled: settings.enabled ?? false,
				policyName: settings.policyName,
				refreshMode: settings.refreshMode as 'interval' | 'daily' | undefined,
				refreshIntervalHours: settings.refreshIntervalHours,
				refreshDailyTime: settings.refreshDailyTime,
			}),
		assignDeviceToPolicy: (mac: string, policy: string) =>
			api.assignDeviceToPolicy(mac, policy),
		singboxRouterListDNSServers: () => api.singboxRouterListDNSServers(),
		singboxRouterAddDNSServer: (server) =>
			api.singboxRouterAddDNSServer({
				...server,
				type: server.type as SingboxRouterDNSType,
				domain_strategy: server.domain_strategy as SingboxRouterDNSStrategy | undefined,
			}),
		singboxRouterApplyPreset: (id: string, outbound: string) =>
			api.singboxRouterApplyPreset(id, outbound),
		singboxRouterListDNSRules: () => api.singboxRouterListDNSRules(),
		singboxRouterAddDNSRule: (rule) => api.singboxRouterAddDNSRule(rule),
		singboxRouterUpdateDNSRule: (index, rule) => api.singboxRouterUpdateDNSRule(index, rule),
		singboxRouterEnable: () => api.singboxRouterEnable(),
		singboxDaemonStatus: async () => {
			const s = await api.singboxGetStatus();
			return { running: s.running };
		},
	};

	async function apply(): Promise<void> {
		singboxWizard.clearLog();
		singboxWizard.setStep('applying');
		singboxWizard.clearError();
		try {
			await runWizard($wizardState, {
				api: orchestratorApi,
				presets,
				onProgress: (label, status) => {
					if (status === 'running') {
						singboxWizard.pushLog({ label, status });
					} else {
						singboxWizard.updateLastLog({ status });
					}
				},
			});
			singboxWizard.setStep('success');
			await singboxRouter.loadAll();
		} catch (e) {
			const phase = e instanceof WizardError ? e.phase : 'unknown';
			const msg = e instanceof Error ? e.message : String(e);
			singboxWizard.setError(phase, msg);
		}
	}

	function close(): void {
		singboxWizard.close();
	}
</script>

{#if $wizardOpen}
	<div class="overlay" role="dialog" aria-modal="true" aria-label="Мастер быстрой настройки">
		<div class="modal">
			<header class="head">
				<span class="hl"><b>Мастер быстрой настройки</b>
					{#if stepIdx >= 0}&middot; Шаг {stepIdx + 1} из {STEPS.length}{/if}
				</span>
				<div class="prog">
					{#each STEPS as _step, i (i)}
						<div
							class="seg"
							class:done={i < stepIdx}
							class:active={i === stepIdx && $wizardState.step !== 'error'}
							class:err={i === stepIdx && $wizardState.step === 'error'}
						></div>
					{/each}
				</div>
				<button
					type="button"
					class="x"
					onclick={close}
					disabled={$wizardState.step === 'applying'}
					aria-label="Закрыть"
					title={$wizardState.step === 'applying' ? 'Дождитесь завершения' : 'Закрыть'}
				>[x]</button>
			</header>
			<div class="body">
				{#if $wizardState.step === 'presets'}
					<StepPresets {presets} />
				{:else if $wizardState.step === 'tunnel'}
					<StepTunnel onAdvance={nextStep} />
				{:else if $wizardState.step === 'policy'}
					<StepPolicy />
				{:else if $wizardState.step === 'devices'}
					<StepDevices />
				{:else if $wizardState.step === 'summary'}
					<StepSummary {presets} />
				{:else if $wizardState.step === 'applying'}
					<StepApplying />
				{:else if $wizardState.step === 'success'}
					<StepSuccess />
				{:else if $wizardState.step === 'error'}
					<StepError onRetry={apply} />
				{/if}
			</div>
			{#if $wizardState.step === 'presets' || $wizardState.step === 'tunnel' || $wizardState.step === 'policy' || $wizardState.step === 'devices' || $wizardState.step === 'summary'}
				<footer class="foot">
					{#if $wizardState.step === 'presets'}
						<button type="button" class="btn ghost" onclick={close}>Отмена</button>
					{:else}
						<button type="button" class="btn ghost" onclick={prevStep}>Назад</button>
					{/if}
					<div></div>
					{#if $wizardState.step === 'summary'}
						<button type="button" class="btn primary" onclick={apply}>Применить</button>
					{:else}
						<button
							type="button"
							class="btn primary"
							disabled={!canAdvance()}
							onclick={nextStep}
						>Дальше</button>
					{/if}
				</footer>
			{/if}
		</div>
	</div>
{/if}

<style>
	.overlay {
		position: fixed; inset: 0;
		background: rgba(0,0,0,0.6);
		display: flex; align-items: center; justify-content: center;
		z-index: 1000;
	}
	.modal {
		width: min(720px, 96vw);
		max-height: 90vh;
		display: flex; flex-direction: column;
		background: var(--color-bg-primary);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		overflow: hidden;
	}
	.head {
		background: var(--color-bg-secondary);
		border-bottom: 1px solid var(--color-border);
		padding: 0.75rem 1rem;
		display: flex; align-items: center; justify-content: space-between;
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}
	.hl b { color: var(--color-text-primary); font-weight: 600; }
	.prog { display: flex; gap: 4px; }
	.seg { width: 28px; height: 4px; border-radius: 2px; background: var(--color-bg-tertiary); }
	.seg.done { background: #3fb950; }
	.seg.active { background: var(--color-accent); }
	.seg.err { background: #f85149; }
	.x { background: transparent; border: 0; color: var(--color-text-muted); cursor: pointer; font-family: monospace; font-size: 0.85rem; }
	.x:disabled { opacity: 0.4; cursor: not-allowed; }
	.body {
		padding: 1.5rem;
		overflow-y: auto;
		flex: 1;
		min-height: 280px;
	}
	.foot {
		background: var(--color-bg-secondary);
		border-top: 1px solid var(--color-border);
		padding: 0.75rem 1rem;
		display: flex;
		justify-content: space-between;
		align-items: center;
	}
	.btn { padding: 0.4rem 1rem; border-radius: 6px; font: inherit; font-size: 0.85rem; cursor: pointer; border: 1px solid transparent; }
	.btn:disabled { opacity: 0.4; cursor: not-allowed; }
	.ghost { color: var(--color-text-muted); background: transparent; }
	.primary { color: white; background: #238636; border-color: #2ea043; }
</style>
