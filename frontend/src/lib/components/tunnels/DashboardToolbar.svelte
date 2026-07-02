<script lang="ts">
	import type { Snippet } from 'svelte';
	import { SegmentedControl } from '$lib/components/ui';
	import LayoutViewToggle from '$lib/components/ui/LayoutViewToggle.svelte';
	import TunnelSearchInput from './TunnelSearchInput.svelte';
	import TunnelCreateMenu from './TunnelCreateMenu.svelte';
	import type { SingboxLayoutMode } from '$lib/constants/singboxLayout';
	import {
		TUNNEL_DASHBOARD_LAYOUT_LABELS,
		type TunnelDashboardLayout,
	} from '$lib/stores/tunnelDashboardMode';

	interface Props {
		searchQuery: string;
		onSearchChange: (value: string) => void;
		layout: TunnelDashboardLayout;
		onLayoutChange: (layout: TunnelDashboardLayout) => void;
		viewMode: SingboxLayoutMode;
		onViewModeChange: (mode: SingboxLayoutMode) => void;
		showViewToggle?: boolean;
		showListOption?: boolean;
		showSingboxCreate?: boolean;
		onCreateAwg: () => void;
		onCreateSingboxSingle?: () => void;
		onCreateSingboxGroup?: () => void;
		onCreateSingboxSubscription?: () => void;
		createIcon: Snippet;
		/** Дополнительные действия (экспорт, статус) перед меню создания. */
		actions?: Snippet;
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
		actions,
	}: Props = $props();

	const layoutOptions: Array<{ value: TunnelDashboardLayout; label: string }> = [
		{ value: 'flat', label: TUNNEL_DASHBOARD_LAYOUT_LABELS.flat },
		{ value: 'sections', label: TUNNEL_DASHBOARD_LAYOUT_LABELS.sections },
	];
</script>

<div class="dashboard-toolbar">
	<div class="tunnel-toolbar-search">
		<TunnelSearchInput value={searchQuery} onInput={onSearchChange} />
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

	{#if actions}
		<div class="dashboard-toolbar-actions">
			{@render actions()}
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

	.dashboard-toolbar-actions {
		flex: 0 0 auto;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
	}

	.dashboard-toolbar-create {
		flex: 0 0 auto;
	}

	/* Mobile: row 1 — layout + view 50/50; row 2 — actions (если есть) на всю ширину;
	   последняя строка — search then create */
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

		.dashboard-toolbar-actions {
			grid-row: 2;
			grid-column: 1 / -1;
			width: 100%;
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

		.dashboard-toolbar:has(.dashboard-toolbar-actions) .tunnel-toolbar-search {
			grid-row: 3;
		}

		.dashboard-toolbar:has(.dashboard-toolbar-actions) .dashboard-toolbar-create {
			grid-row: 3;
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

		.dashboard-toolbar-create :global(.dropdown-wrapper) {
			display: block;
			width: 100%;
		}

		.dashboard-toolbar-create :global(.dropdown-wrapper .btn) {
			width: 100%;
			justify-content: center;
			white-space: nowrap;
		}
	}
</style>
