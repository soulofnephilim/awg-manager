<script lang="ts">
	import type { Snippet } from 'svelte';
	import { SegmentedControl } from '$lib/components/ui';
	import LayoutViewToggle from '$lib/components/ui/LayoutViewToggle.svelte';
	import type { LayoutViewMode } from '$lib/components/ui/layoutViewToggle';
	import TunnelTableSortControls from '$lib/components/tunnels/TunnelTableSortControls.svelte';
	import TunnelCreateMenu from './TunnelCreateMenu.svelte';
	import {
		TUNNEL_DASHBOARD_LAYOUT_LABELS,
		type TunnelDashboardLayout,
	} from '$lib/stores/tunnelDashboardMode';

	interface Props {
		searchQuery: string;
		onSearchChange: (value: string) => void;
		layout: TunnelDashboardLayout;
		onLayoutChange: (layout: TunnelDashboardLayout) => void;
		viewMode: LayoutViewMode;
		onViewModeChange: (mode: LayoutViewMode) => void;
		showViewToggle?: boolean;
		showListOption?: boolean;
		showSingboxCreate?: boolean;
		onCreateAwg: () => void;
		onCreateSingboxSingle?: () => void;
		onCreateSingboxGroup?: () => void;
		onCreateSingboxSubscription?: () => void;
		createIcon: Snippet;
	}

	let {
		searchQuery,
		onSearchChange,
		layout,
		onLayoutChange,
		viewMode,
		onViewModeChange,
		showViewToggle = true,
		showListOption = true,
		showSingboxCreate = true,
		onCreateAwg,
		onCreateSingboxSingle,
		onCreateSingboxGroup,
		onCreateSingboxSubscription,
		createIcon,
	}: Props = $props();

	const layoutOptions: Array<{ value: TunnelDashboardLayout; label: string }> = [
		{ value: 'flat', label: TUNNEL_DASHBOARD_LAYOUT_LABELS.flat },
		{ value: 'sections', label: TUNNEL_DASHBOARD_LAYOUT_LABELS.sections },
	];
</script>

<div class="dashboard-toolbar">
	<div class="tunnel-toolbar-search">
		<TunnelTableSortControls
			{searchQuery}
			sortKey={null}
			sortAsc={true}
			options={[]}
			showSearch={true}
			showSort={false}
			{onSearchChange}
			onSortChange={() => {}}
			onToggleDir={() => {}}
		/>
	</div>

	<div class="dashboard-toolbar-layout">
		<SegmentedControl
			value={layout}
			options={layoutOptions}
			ariaLabel="Расположение туннелей на дашборде"
			onchange={(next) => onLayoutChange(next)}
		/>
	</div>

	{#if showViewToggle}
		<div class="dashboard-toolbar-view">
			<LayoutViewToggle
				value={viewMode}
				denseValue="dense"
				{showListOption}
				ariaLabel="Вид туннелей"
				onchange={(mode) => onViewModeChange(mode)}
			/>
		</div>
	{/if}

	<div class="dashboard-toolbar-create">
		<TunnelCreateMenu
			onAwg={onCreateAwg}
			onSingboxSingle={onCreateSingboxSingle}
			onSingboxGroup={onCreateSingboxGroup}
			onSingboxSubscription={onCreateSingboxSubscription}
			showSingbox={showSingboxCreate}
			triggerIcon={createIcon}
		/>
	</div>
</div>

<style>
	.dashboard-toolbar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
		flex: 1 1 auto;
	}

	.tunnel-toolbar-search {
		flex: 1 1 160px;
		min-width: 120px;
		max-width: 220px;
	}

	.tunnel-toolbar-search :global(.tunnel-sort-controls),
	.tunnel-toolbar-search :global(.tunnel-search) {
		width: 100%;
	}

	.dashboard-toolbar-layout {
		flex: 0 1 auto;
		min-width: 0;
	}

	.dashboard-toolbar-layout :global(.segmented-control) {
		height: 32px;
	}

	.dashboard-toolbar-view {
		flex: 0 0 auto;
	}

	.dashboard-toolbar-create {
		flex: 0 0 auto;
	}

	/* Mobile: row 1 — layout + view 50/50; row 2 — search then create */
	@media (max-width: 760px) {
		.dashboard-toolbar {
			display: grid;
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
			gap: 0.5rem;
			width: 100%;
		}

		.dashboard-toolbar-layout {
			grid-row: 1;
			grid-column: 1;
			width: 100%;
		}

		.dashboard-toolbar-view {
			grid-row: 1;
			grid-column: 2;
			width: 100%;
		}

		.dashboard-toolbar:not(:has(.dashboard-toolbar-view)) .dashboard-toolbar-layout {
			grid-column: 1 / -1;
		}

		.tunnel-toolbar-search {
			grid-row: 2;
			grid-column: 1;
			min-width: 0;
			max-width: none;
			width: 100%;
		}

		.dashboard-toolbar-create {
			grid-row: 2;
			grid-column: 2;
			justify-self: stretch;
			align-self: center;
			min-width: 0;
		}

		.dashboard-toolbar-layout :global(.segmented-control),
		.dashboard-toolbar-view :global(.segmented-control) {
			width: 100%;
			min-width: 0;
			justify-content: stretch;
		}

		.dashboard-toolbar-view :global(.segmented-control--icon .segmented-control-btn) {
			flex: 1 1 28px;
			min-width: 28px;
		}

		.tunnel-toolbar-search :global(.tunnel-sort-controls) {
			display: flex;
			width: 100%;
		}

		.dashboard-toolbar-create :global(.tunnel-create-menu) {
			display: block;
			width: 100%;
		}

		.dashboard-toolbar-create :global(.tunnel-create-menu .btn) {
			width: 100%;
			justify-content: center;
			white-space: nowrap;
		}
	}
</style>
