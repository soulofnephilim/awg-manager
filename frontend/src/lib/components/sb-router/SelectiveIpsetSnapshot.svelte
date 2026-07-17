<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { SelectiveDomainMatcherRecord, SelectiveRebuildSnapshot } from '$lib/types';
	import SelectiveMatcherTable from './SelectiveMatcherTable.svelte';

	interface Props {
		snapshot: SelectiveRebuildSnapshot;
	}

	let { snapshot }: Props = $props();

	const PAGE = 100;

	let matchers = $state<SelectiveDomainMatcherRecord[]>([]);
	let total = $state(0);
	let loading = $state(false);
	let loadError = $state('');

	const matcherTotal = $derived(
		snapshot.domainMatcherCount ?? snapshot.domainResults?.length ?? 0,
	);

	onMount(() => {
		if (matcherTotal > 0) {
			void loadMatchers(0);
		}
	});

	async function loadMatchers(offset: number) {
		loading = true;
		loadError = '';
		try {
			const res = await api.singboxRouterSelectiveSnapshotMatchers(offset, PAGE);
			if (offset === 0) {
				matchers = res.matchers;
			} else {
				matchers = [...matchers, ...res.matchers];
			}
			total = res.total;
		} catch (e) {
			loadError = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	function loadMore() {
		if (!loading && matchers.length < total) {
			void loadMatchers(matchers.length);
		}
	}
</script>

<div class="snap">
	<div class="snap-meta">
		<span>Записей в ipset: <strong>{snapshot.entryCount}</strong></span>
		{#if snapshot.staticCidrCount != null && snapshot.staticCidrCount > 0}
			<span>· статических CIDR: {snapshot.staticCidrCount}</span>
		{/if}
		{#if matcherTotal > 0}
			<span>· доменных правил: {matcherTotal}</span>
		{/if}
		{#if snapshot.lastCDNRefresh}
			<span>· CDN refresh: {snapshot.lastCDNRefresh}</span>
		{/if}
	</div>

	{#if snapshot.staticCidrs?.length}
		<details class="snap-block">
			<summary>Статические CIDR ({snapshot.staticCidrs.length})</summary>
			<ul class="ip-list">
				{#each snapshot.staticCidrs as cidr (cidr)}
					<li class="mono">{cidr}</li>
				{/each}
			</ul>
		</details>
	{/if}

	{#if matcherTotal > 0}
		<details class="snap-block" open>
			<summary>Доменные правила ({matcherTotal})</summary>
			<SelectiveMatcherTable
				{matchers}
				{total}
				{loading}
				{loadError}
				showLoadMore
				onLoadMore={loadMore}
			/>
		</details>
	{/if}
</div>

<style>
	.snap {
		font-size: 12px;
	}
	.snap-meta {
		color: var(--text-muted, #888);
		margin-bottom: 8px;
		display: flex;
		flex-wrap: wrap;
		gap: 4px 8px;
	}
	.snap-block {
		margin-top: 8px;
	}
	.snap-block summary {
		cursor: pointer;
		font-weight: 500;
		margin-bottom: 4px;
	}
	.mono {
		font-family: var(--font-mono, monospace);
	}
	.ip-list {
		margin: 4px 0 0;
		padding-left: 1.2em;
		max-height: 120px;
		overflow-y: auto;
	}
</style>
