<script lang="ts">
	import type { Snippet } from 'svelte';
	import { ChevronRight } from 'lucide-svelte';

	interface Props {
		id: string;
		/** Заголовок группы над строкой — только у первой строки группы. */
		group?: string;
		label: string;
		/** CLI-флаг(и) параметра, например «-peer» или «-mode / -transport». */
		flag: string;
		summary: string;
		dirty?: boolean;
		expanded: boolean;
		ontoggle: (id: string) => void;
		children: Snippet;
	}

	let { id, group, label, flag, summary, dirty = false, expanded, ontoggle, children }: Props =
		$props();
</script>

{#if group}
	<div class="ft-group-label">{group}</div>
{/if}
<div class="ft-row" class:expanded>
	<button type="button" class="ft-row-head" aria-expanded={expanded} onclick={() => ontoggle(id)}>
		<span class="ft-row-label">{label} <span class="ft-row-flag">{flag}</span></span>
		<span class="ft-row-summary" class:dirty>{summary}{dirty ? ' · изменено' : ''}</span>
		<span class="ft-row-chevron" class:open={expanded}><ChevronRight size={13} /></span>
	</button>
	{#if expanded}
		<div class="ft-row-body">{@render children()}</div>
	{/if}
</div>

<style>
	.ft-group-label {
		padding: 0.625rem 1rem 0.25rem;
		font-size: 0.6875rem;
		font-weight: 600;
		letter-spacing: 0.05em;
		text-transform: uppercase;
		color: var(--color-text-secondary);
		border-top: 1px solid var(--color-border);
	}

	.ft-group-label:first-child {
		border-top: none;
	}

	/* В спеке разделитель rgba(255,255,255,.04) — заменён на color-mix от
	   --color-border, чтобы работала светлая тема. */
	.ft-row {
		border-top: 1px solid color-mix(in srgb, var(--color-border) 45%, transparent);
	}

	.ft-row.expanded {
		background: var(--color-bg-tertiary);
	}

	.ft-row-head {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		width: 100%;
		padding: 0.5625rem 1rem;
		background: none;
		border: none;
		cursor: pointer;
		font: inherit;
		text-align: left;
		color: inherit;
	}

	.ft-row-head:hover {
		background: color-mix(in srgb, var(--color-bg-hover) 40%, transparent);
	}

	.ft-row-label {
		flex: 1;
		min-width: 0;
		font-size: 0.8125rem;
		color: var(--color-text-secondary);
	}

	.ft-row-flag {
		font-size: 0.6875rem;
		color: var(--color-text-secondary);
		opacity: 0.7;
	}

	.ft-row-summary {
		font-size: 0.8125rem;
		font-family: var(--font-mono);
		color: var(--color-text-primary);
		white-space: nowrap;
		flex: none;
	}

	.ft-row-summary.dirty {
		color: var(--color-warning);
	}

	.ft-row-chevron {
		display: inline-flex;
		color: var(--color-text-secondary);
		transition: transform 0.15s ease;
		flex: none;
	}

	.ft-row-chevron.open {
		transform: rotate(90deg);
	}

	.ft-row-body {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.75rem;
		padding: 0.125rem 1rem 0.875rem;
	}

	@media (max-width: 640px) {
		.ft-row-body {
			grid-template-columns: 1fr;
		}

		.ft-row-summary {
			white-space: normal;
			word-break: break-word;
		}
	}
</style>
