<!-- frontend/src/lib/components/routing/singboxRouter/ConnectionsFilters.svelte -->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import type { ConnectionFilters, NetworkFilter } from '$lib/types/singboxConnections';
	import { Dropdown, type DropdownOption } from '$lib/components/ui';

	interface Props {
		filters: ConnectionFilters;
		outboundOptions: string[];
		ruleOptions: string[];
		onChange: (next: ConnectionFilters) => void;
	}

	let { filters, outboundOptions, ruleOptions, onChange }: Props = $props();

	// svelte-ignore state_referenced_locally
	let searchValue = $state(filters.search);

	$effect(() => {
		if (searchValue !== filters.search) {
			searchValue = filters.search;
		}
	});

	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

	onDestroy(() => {
		if (debounceTimer !== null) clearTimeout(debounceTimer);
	});

	function commitSearch(): void {
		onChange({ ...filters, search: searchValue });
	}

	function onSearchInput(e: Event): void {
		searchValue = (e.target as HTMLInputElement).value;
		if (debounceTimer !== null) clearTimeout(debounceTimer);
		debounceTimer = setTimeout(commitSearch, 200);
	}

	const outboundDropdown = $derived<DropdownOption[]>([
		{ value: '', label: 'Все' },
		...outboundOptions.map((o) => ({ value: o, label: o })),
	]);

	const networkDropdown: DropdownOption<NetworkFilter>[] = [
		{ value: 'all', label: 'Все' },
		{ value: 'tcp', label: 'TCP' },
		{ value: 'udp', label: 'UDP' },
	];

	const ruleDropdown = $derived<DropdownOption[]>([
		{ value: '', label: 'Все' },
		...ruleOptions.map((r) => ({ value: r, label: r })),
	]);
</script>

<div class="col">
	<input
		type="text"
		class="search"
		placeholder="Поиск host / IP / клиент"
		value={searchValue}
		oninput={onSearchInput}
	/>

	<div class="filter-row">
		<span class="filter-label">Outbound</span>
		<div class="filter-control">
			<Dropdown
				value={filters.outbound}
				options={outboundDropdown}
				onchange={(v) => onChange({ ...filters, outbound: v })}
				fullWidth
			/>
		</div>
	</div>

	<div class="filter-row">
		<span class="filter-label">Network</span>
		<div class="filter-control">
			<Dropdown
				value={filters.network}
				options={networkDropdown}
				onchange={(v) => onChange({ ...filters, network: v })}
				fullWidth
			/>
		</div>
	</div>

	<div class="filter-row">
		<span class="filter-label">Rule</span>
		<div class="filter-control">
			<Dropdown
				value={filters.rule}
				options={ruleDropdown}
				onchange={(v) => onChange({ ...filters, rule: v })}
				fullWidth
			/>
		</div>
	</div>
</div>

<style>
	.col {
		display: flex;
		flex-direction: column;
		gap: 10px;
		margin-bottom: 12px;
	}
	.search {
		padding: 6px 10px;
		font-size: 13px;
		background: var(--surface-1, #1f2425);
		border: 1px solid var(--border-1, #2c3134);
		border-radius: 6px;
		color: var(--text-primary, #e8e6e3);
	}
	.filter-row {
		display: flex;
		align-items: center;
		gap: 12px;
	}
	.filter-label {
		flex: 0 0 96px;
		font-size: 12px;
		color: var(--text-secondary, #b8b6b3);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.filter-control {
		flex: 1;
		min-width: 0;
	}
</style>
