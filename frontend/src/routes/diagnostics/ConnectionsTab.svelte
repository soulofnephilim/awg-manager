<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import type { ConnectionsResponse } from '$lib/types';
	import type { DropdownOption } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores/notifications';
	import { ConnectionsTable, ConnectionsTotalsBar, ConnectionsFilterPanel } from '$lib/components/connections';
	let data = $state<ConnectionsResponse | null>(null);
	let loading = $state(false);
	const AUTO_REFRESH_MS = 30_000;
	let nowTs = $state(Date.now());
	let lastFetchedAtTs = $state(0);

	let fTunnel = $state('all');
	let fProto = $state('all');
	let fState = $state('all');
	let search = $state('');
	let offset = $state(0);
	let sortBy = $state<'' | 'proto' | 'src' | 'dst' | 'iface' | 'state' | 'bytes'>('');
	let sortDir = $state<'asc' | 'desc'>('asc');
	let autoRefreshTimer: ReturnType<typeof setInterval> | null = null;
	let progressTimer: ReturnType<typeof setInterval> | null = null;
	let requestSeq = 0;
	const refreshProgress = $derived.by(() => {
		if (lastFetchedAtTs <= 0) return 0;
		const elapsed = Math.max(0, nowTs - lastFetchedAtTs);
		return Math.min(1, elapsed / AUTO_REFRESH_MS);
	});
	const totalOut = $derived((data?.byTunnel ?? []).reduce((s, b) => s + b.bytesOut, 0));
	const totalIn = $derived((data?.byTunnel ?? []).reduce((s, b) => s + b.bytesIn, 0));

	const tunnelOptions = $derived.by((): DropdownOption[] => {
		const opts: DropdownOption[] = [{ value: 'all', label: 'Все' }, { value: 'direct', label: 'Напрямую' }];
		for (const [id, info] of Object.entries(data?.tunnels ?? {})) {
			if (id === '@direct') continue;
			if (id === '@singbox') opts.push({ value: 'singbox', label: 'sing-box' });
			else if (id === '@local') opts.push({ value: 'local', label: 'Локально' });
			else opts.push({ value: id, label: info.name });
		}
		return opts;
	});

	async function fetchData() {
		const seq = ++requestSeq;
		loading = true;
		try {
			const nextData = await api.getConnections({
				tunnel: fTunnel,
				protocol: fProto,
				state: fState,
				search,
				offset,
				limit: 200,
				sortBy: sortBy || undefined,
				sortDir,
			});
			if (seq !== requestSeq) return;
			data = nextData;
			lastFetchedAtTs = Date.now();
		} catch (e) {
			if (seq !== requestSeq) return;
			notifications.error('Не удалось загрузить соединения');
			data = null;
		} finally {
			if (seq === requestSeq) {
				loading = false;
			}
		}
	}

	function setFilter(patch: Partial<{ fTunnel: string; fProto: string; fState: string }>): void {
		if (patch.fTunnel !== undefined) fTunnel = patch.fTunnel;
		if (patch.fProto !== undefined) fProto = patch.fProto;
		if (patch.fState !== undefined) fState = patch.fState;
		offset = 0;
		fetchData();
	}

	function handleSortChange(column: 'proto' | 'src' | 'dst' | 'iface' | 'state' | 'bytes') {
		if (sortBy === column) {
			sortDir = sortDir === 'asc' ? 'desc' : 'asc';
		} else {
			sortBy = column;
			sortDir = 'asc';
		}
		offset = 0;
		fetchData();
	}

	let searchTimeout: ReturnType<typeof setTimeout> | null = null;
	function handleSearchInput(value: string) {
		search = value;
		if (searchTimeout) clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => {
			offset = 0;
			fetchData();
		}, 300);
	}

	onMount(() => {
		fetchData();
		autoRefreshTimer = setInterval(fetchData, AUTO_REFRESH_MS);
		progressTimer = setInterval(() => {
			nowTs = Date.now();
		}, 200);
	});

	onDestroy(() => {
		if (searchTimeout) clearTimeout(searchTimeout);
		if (autoRefreshTimer) clearInterval(autoRefreshTimer);
		if (progressTimer) clearInterval(progressTimer);
	});

	function handlePageChange(newOffset: number) {
		offset = newOffset;
		fetchData();
	}
</script>

{#if data || loading}
	<ConnectionsTotalsBar
		stats={data?.stats ?? null}
		bytesOut={totalOut}
		bytesIn={totalIn}
		fetchedAt={data?.fetchedAt ?? ''}
		{loading}
		progress={refreshProgress}
		onRefresh={fetchData}
	/>

	<ConnectionsFilterPanel
		{search}
		{fTunnel}
		{fProto}
		{fState}
		{tunnelOptions}
		onSearchInput={handleSearchInput}
		onTunnel={(v) => setFilter({ fTunnel: v })}
		onProto={(v) => setFilter({ fProto: v })}
		onState={(v) => setFilter({ fState: v })}
	/>

	<ConnectionsTable
		connections={data?.connections ?? []}
		pagination={data?.pagination ?? { total: 0, offset: 0, limit: 200, returned: 0 }}
		showSkeleton={loading && !data}
		{sortBy}
		{sortDir}
		onSortChange={handleSortChange}
		onPageChange={handlePageChange}
	/>
{/if}
