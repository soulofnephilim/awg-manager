<script lang="ts">
  import { page } from '$app/stores';
  import { singboxRouter as singboxRouterStore } from '$lib/stores/singboxRouter';
  import { DeviceProxySubTab } from '$lib/components/singbox-routing';
  import SettingsDrawer from './SettingsDrawer.svelte';
  import { ConnectionsSubTab } from '$lib/components/routing/singboxRouter';
  import {
    PageShell,
    RulesPanel,
    FlowGraph,
    TracePanel,
    traceOpen,
    AddWizardPanel,
    addWizardOpen,
    EmptyState,
    ExpertPanel,
    mode as sbMode,
    type EngineStatus,
  } from '$lib/components/sb-router';
  import { isMockDevMode } from '$lib/env';

  let activeSingboxSub = $derived($page.url.searchParams.get('sub'));
  const singboxRouterStatus = singboxRouterStore.status;
  const singboxRulesStore = singboxRouterStore.rules;
  let singboxRulesCount = $derived($singboxRulesStore.length);

  let sbEngineStatus: EngineStatus = $derived.by(() => {
    const s = $singboxRouterStatus;
    if (!s || !s.installed) return 'unknown';
    return s.enabled ? 'ok' : 'down';
  });
  const previewReadOnly = !isMockDevMode();
</script>

{#if previewReadOnly}
  <div class="preview-banner">
    Новый дизайн — alpha-preview. На реальном роутере изменение конфигурации отключено. Для рабочих изменений используйте Рабочий интерфейс.
  </div>
{:else}
  <div class="preview-banner dev">Mock/dev: действия разрешены.</div>
{/if}

<PageShell engineStatus={sbEngineStatus}>
  {#if activeSingboxSub === 'deviceproxy'}
    <DeviceProxySubTab />
  {:else if activeSingboxSub === 'connections'}
    <ConnectionsSubTab />
  {:else if $sbMode === 'beginner'}
    {#if $addWizardOpen}
      <AddWizardPanel readOnly={previewReadOnly} />
    {:else if $traceOpen}
      <TracePanel />
    {:else if !($singboxRouterStatus?.enabled ?? false) || singboxRulesCount === 0}
      <EmptyState readOnly={previewReadOnly} />
    {:else}
      <FlowGraph />
      <RulesPanel readOnly={previewReadOnly} />
    {/if}
  {:else}
    <ExpertPanel readOnly={previewReadOnly} />
  {/if}
</PageShell>

<SettingsDrawer />

<style>
  .preview-banner {
    margin-bottom: 0.5rem;
    border: 1px solid var(--color-warning-border, var(--border));
    background: color-mix(in srgb, var(--color-warning, #f59e0b) 12%, transparent);
    color: var(--text-primary);
    border-radius: var(--radius-sm);
    padding: 0.5rem 0.75rem;
    font-size: 12px;
  }
  .preview-banner.dev {
    border-color: var(--border);
    background: color-mix(in srgb, var(--color-success, #22c55e) 10%, transparent);
  }
</style>
