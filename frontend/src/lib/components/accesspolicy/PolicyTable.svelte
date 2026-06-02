<script lang="ts">
	import type { AccessPolicy } from '$lib/types';
	import { Badge } from '$lib/components/ui';
	import { isHydraRouteAccessPolicy } from '$lib/utils/accessPolicy';

	interface Props {
		policies: AccessPolicy[];
		onedit: (name: string) => void;
		ondelete: (name: string) => void;
		selectable?: boolean;
		selectedNames?: Set<string>;
		onselect?: (name: string) => void;
	}

	let { policies, onedit, ondelete, selectable, selectedNames, onselect }: Props = $props();
</script>

<div class="policy-grid">
	{#each policies as policy}
		{@const isHrPolicy = isHydraRouteAccessPolicy(policy)}
		<div class="policy-card" class:policy-card-hr={isHrPolicy}>
			{#if selectable}
				<div class="select-cell">
					<input
						type="checkbox"
						class="select-check"
						checked={selectedNames?.has(policy.name)}
						disabled={isHrPolicy}
						onchange={() => onselect?.(policy.name)}
					/>
				</div>
			{/if}
			<div class="policy-body">
				<div class="policy-meta">
					{policy.deviceCount} устройств
				</div>
				<div class="policy-title-row">
					<span class="policy-name">{policy.description || policy.name}</span>
					<div class="routing-badges">
						{#if isHrPolicy}
							<Badge variant="warning" uppercase size="xs" pill>HydraRoute</Badge>
						{/if}
						{#if policy.standalone}
							<Badge variant="accent" uppercase size="xs" pill>standalone</Badge>
						{/if}
					</div>
				</div>
				{#if policy.interfaces?.length}
					<div class="card-route">
						<span class="route-arrow">&rarr;</span>
						{#each [...policy.interfaces].sort((a, b) => a.order - b.order) as iface}
							<Badge variant="muted" mono size="xs" title={iface.name}>{iface.label || iface.name}</Badge>
						{/each}
					</div>
				{/if}
			</div>
			<div class="policy-actions">
				<button
					type="button"
					class="route-action-btn"
					title={isHrPolicy
						? `Открыть HydraRoute-политику «${policy.description || policy.name}»`
						: `Изменить политику «${policy.description || policy.name}»`}
					onclick={() => onedit(policy.name)}
				>
					<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
						<path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
					</svg>
				</button>
				{#if !isHrPolicy}
				<button
					type="button"
					class="route-action-btn danger"
					title={`Удалить политику «${policy.description || policy.name}»`}
					onclick={() => ondelete(policy.name)}
				>
					<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<polyline points="3 6 5 6 21 6"/>
						<path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
					</svg>
				</button>
				{/if}
			</div>
		</div>
	{/each}
</div>

<style>
	.policy-grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 12px;
	}

	.policy-card {
		background: var(--bg-secondary);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 14px 16px;
		display: flex;
		align-items: flex-start;
		gap: 12px;
		transition: border-color 0.15s;
	}

	.policy-card:hover {
		border-color: var(--border-hover);
	}

	.policy-body {
		flex: 1;
		min-width: 0;
	}

	.policy-meta {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--text-muted);
		margin-bottom: 2px;
	}

	.policy-title-row {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 8px;
		margin-bottom: 6px;
	}

	.policy-name {
		font-weight: 500;
		font-size: 0.9375rem;
		color: var(--text-primary);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.policy-card-hr {
		border-color: rgba(245, 158, 11, 0.35);
	}

	.policy-actions {
		display: flex;
		gap: 4px;
		align-items: center;
		flex-shrink: 0;
		align-self: center;
	}

	.select-cell {
		width: 2rem;
		padding: 0.5rem;
		display: flex;
		align-items: center;
		flex-shrink: 0;
	}

	.select-check {
		accent-color: var(--accent);
		width: 1rem;
		height: 1rem;
		cursor: pointer;
	}

	@media (max-width: 1024px) {
		.policy-grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}

	@media (max-width: 768px) {
		.policy-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
