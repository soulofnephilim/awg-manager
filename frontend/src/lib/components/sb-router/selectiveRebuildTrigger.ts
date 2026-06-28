/**
 * Helper for triggering a selective-bypass ipset rebuild after rule mutations.
 * Called from AddWizardPanel, RulesPanel and any other place that modifies
 * router rules or rule sets while selective mode may be active.
 *
 * The rebuild runs in the background: progress arrives via SSE
 * (singbox-router:selective-progress) → selectiveBypass store → the
 * SelectiveRebuildModal in StatusDrawer opens automatically.
 */
import { get } from 'svelte/store';
import { api } from '$lib/api/client';
import { singboxRouter } from '$lib/stores/singboxRouter';
import { selectiveBypass } from '$lib/stores/selectiveBypass';

/**
 * Trigger an ipset rebuild if the router settings have selectiveBypass enabled.
 * Errors are silently swallowed — a stale ipset is preferable to surfacing an
 * extra error notification after a successful rule save.
 */
export async function triggerSelectiveRebuildIfEnabled(): Promise<void> {
  const settings = get(singboxRouter.settings);
  if (!settings?.selectiveBypass) return;
  selectiveBypass.resetProgress();
  selectiveBypass.requestModal();
  try {
    const status = await api.singboxRouterSelectiveRebuild();
    selectiveBypass.applyStatus(status);
  } catch {
    // non-fatal — progress/error arrives via SSE anyway
  }
}
