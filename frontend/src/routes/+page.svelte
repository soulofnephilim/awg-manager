<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { tunnels } from '$lib/stores/tunnels';
	import { systemInfo as systemInfoStore } from '$lib/stores/system';
	import { notifications } from '$lib/stores/notifications';
	import { api } from '$lib/api/client';
	import {
		TunnelCard,
		ExternalTunnelCard,
		AdoptTunnelDialog,
		SystemTunnelCard,
		TunnelReferencedModal,
		ConnectivitySettingsModal,
		TunnelListTrafficCell,
		TunnelPingButton,
		TunnelTitleRow,
		TunnelMetaText,
		TunnelToolbarViewRow,
		DefaultRouteBadge,
	} from '$lib/components/tunnels';
	import { TunnelListActions } from '$lib/components/ui';
	import TunnelDiagnosticsModal from '$lib/components/testing/TunnelDiagnosticsModal.svelte';
	import { PageContainer, PageHeader, LoadingSpinner, EmptyState, WelcomeBanner } from '$lib/components/layout';
	import {
		Modal,
		StoreStatusBadge,
		TrafficChartModal,
		TrafficSparkline,
		Button,
		Badge,
		Tabs,
		Toggle,
		StatusDot,
		Stat,
		StatStrip,
		LayoutViewToggle,
		TableSortHeader,
	} from '$lib/components/ui';
	import { singboxDelayHistory, singboxStatus, singboxTraffic, singboxTunnels } from '$lib/stores/singbox';
	import { SingboxInstallBanner, SingboxTunnelCard } from '$lib/components/singbox';
	import { feedTraffic, getTrafficRates, getTrafficSparklineSeries, subscribeTraffic } from '$lib/stores/traffic';
	import { usageLevel } from '$lib/stores/settings';
	import { isSectionVisible, isTunnelDashboardAvailable } from '$lib/types/usageLevel';
	import { subscriptionsStore } from '$lib/stores/subscriptions';
	import SubscriptionCard from '$lib/components/subscriptions/SubscriptionCard.svelte';
	import AddTunnelWizard from '$lib/components/subscriptions/AddTunnelWizard.svelte';
	import SubscriptionActiveCard from '$lib/components/subscriptions/SubscriptionActiveCard.svelte';
	import SubscriptionGroupsSection from '$lib/components/subscriptions/SubscriptionGroupsSection.svelte';
	import SubscriptionsTabSection from '$lib/components/subscriptions/SubscriptionsTabSection.svelte';
	import SingboxTunnelsTabSection from '$lib/components/singbox/SingboxTunnelsTabSection.svelte';
	import AwgTunnelsTabSection from '$lib/components/tunnels/AwgTunnelsTabSection.svelte';
	import DashboardFlatSection from '$lib/components/tunnels/DashboardFlatSection.svelte';
	import type { DashboardFlatContext } from '$lib/components/tunnels/dashboardFlatContext';
	import type { AwgTabContext } from '$lib/components/tunnels/awgTabContext';
	import type { ExternalTunnel, Subscription, SubscriptionMember, SystemTunnel, TunnelListItem } from '$lib/types';
	import { formatBitRate, formatBytes, formatDuration, formatRelativeTime, secondsSince } from '$lib/utils/format';
	import { showOutboundReferencedError } from '$lib/utils/outboundReferenced';
	import {
		awgConnectivityDown,
		awgListShowsPingButton,
		awgPingStatusNote,
		awgRecoveringVisual,
		awgShowConnectivityRow,
		awgToggleTint,
	} from '$lib/utils/awgPingStatus';
	import { awgManagedStatusDot } from '$lib/utils/statusDot';
	import { resolveSubscriptionMemberTag } from '$lib/utils/subscriptionMember';
	import { nativewgUnavailableHint } from '$lib/utils/backendAvailability';
	import {
		SINGBOX_LAYOUT_STORAGE_KEY,
		parseSingboxLayoutMode,
		readTunnelMobileLayout,
		subscribeTunnelMobileLayout,
		type SingboxLayoutMode,
		type TunnelRenderMode,
	} from '$lib/constants/singboxLayout';
	import { isMockDevMode as getIsMockDevMode } from '$lib/env';
	import { Download, Eye, EyeOff, GripVertical, Server, Upload, LayoutGrid, Link, Globe, TriangleAlert } from 'lucide-svelte';
	import CreateIcon from '$lib/components/ui/icons/CreateIcon.svelte';
	import { formatRunningSub, pluralForm, SUBSCRIPTION_WORDS, TUNNEL_WORDS } from '$lib/utils/pluralize';
	import DashboardToolbar from '$lib/components/tunnels/DashboardToolbar.svelte';
	import DashboardSummary from '$lib/components/tunnels/DashboardSummary.svelte';
	import TunnelTagChips from '$lib/components/tunnels/TunnelTagChips.svelte';
	import TunnelSectionHeader from '$lib/components/tunnels/TunnelSectionHeader.svelte';
	import {
		tunnelDashboardLayout,
		tunnelDashboardMode,
		tunnelDashboardView,
	} from '$lib/stores/tunnelDashboardMode';
	import {
		tunnelDashboardGroupMode,
		tunnelDashboardManualOrder,
		tunnelDashboardOrderMode,
		tunnelDashboardTags,
	} from '$lib/stores/tunnelDashboardPrefs';
	import { applyManualOrder, mergeManualOrder, reorder } from '$lib/utils/tunnelDashboardOrder';
	import { filterItemsByTag, getItemTags, groupFlatItemsByTag } from '$lib/utils/tunnelDashboardTags';
	import { createReorderDrag } from '$lib/components/sb-router/reorderDrag.svelte';
	import { buildFlatDashboardItems, type TunnelDashboardFlatItem } from '$lib/utils/tunnelDashboardFlat';
	import {
		awgTunnelTableSort,
		singboxSubscriptionTableSort,
		singboxTunnelTableSort,
		type AwgTunnelSortKey,
		type SingboxTunnelSortKey,
		type SubscriptionSortKey,
	} from '$lib/stores/tunnelTableSort';
	import {
		applyDirection,
		ariaSort,
		compareBool,
		compareDelayLike,
		compareNullableNumber,
		compareString,
	} from '$lib/utils/tunnelTableSort';

	type TunnelTab = 'awg' | 'singbox' | 'subscriptions';
	type AwgTunnelViewMode = 'cards' | 'compact' | 'list';
	type TunnelSurfaceLayout = SingboxLayoutMode | 'cards';

	function resolveTunnelRenderMode(mobile: boolean, layout: TunnelSurfaceLayout): TunnelRenderMode {
		if (layout === 'list') return mobile ? 'list-card' : 'table';
		if (layout === 'dense' || layout === 'cards') return 'dense';
		return 'compact';
	}
	type ConnectivityCell = { connected: boolean; latency: number | null } | undefined;
	type EndpointScope = 'managed' | 'system' | 'external';
	type TunnelSortOption = { value: string; label: string };

	const AWG_TUNNEL_VIEW_STORAGE_KEY = 'awg_tunnel_view_mode';
	const SINGBOX_TUNNELS_LAYOUT_STORAGE_KEY = 'singbox_tunnels_layout_mode';
	const SINGBOX_SUBSCRIPTIONS_LAYOUT_STORAGE_KEY = 'singbox_subscriptions_layout_mode';
	const isMockDevMode = getIsMockDevMode();

	// Polling-store subscription: first subscriber triggers the fetch,
	// the last unsubscribe stops polling. `$tunnels` yields a
	// PollingState<TunnelsSnapshot> — unwrap below.
	let unsubTunnels: (() => void) | undefined;
	onMount(() => { unsubTunnels = tunnels.subscribe(() => {}); });
	onDestroy(() => unsubTunnels?.());

	let trafficTick = $state(0);
	let unsubTraffic: (() => void) | undefined;
	onMount(() => {
		unsubTraffic = subscribeTraffic(() => {
			trafficTick += 1;
		});
	});
	onDestroy(() => unsubTraffic?.());

	let sysInfo = $derived($systemInfoStore.data);
	let tunnelSnap = $derived($tunnels);
	let awgList = $derived(tunnelSnap.data?.tunnels ?? []);
	let externalList = $derived(tunnelSnap.data?.external ?? []);
	let systemList = $derived(tunnelSnap.data?.system ?? []);
	const awgConnectivityStore = tunnels.connectivityMap;
	let awgConnectivityMap = $derived($awgConnectivityStore);
	// Wait for both system info AND the first tunnels snapshot before leaving
	// the loading state — otherwise sysInfo arrives first and the empty-state
	// flashes until /api/tunnels/all lands.
	let loading = $derived(
		!sysInfo ||
		tunnelSnap.status === 'idle' ||
		tunnelSnap.status === 'loading',
	);

	// System tunnels don't emit tunnel:traffic stream events (no awg-manager
	// peer entry tracks them) — feed the traffic store from the polled
	// snapshot so the per-system-tunnel rate chart stays alive. Runs on
	// every snapshot refresh (~5s).
	$effect(() => {
		// Skip system tunnels that are ALSO tracked as managed — they receive
		// tunnel:traffic stream events via +layout. Double-feeding doubles
		// the rate sample and produces a spurious chart spike.
		for (const st of systemList) {
			const isManaged = awgList.some((m) =>
				(m.ndmsName && m.ndmsName === st.id) || (m.interfaceName && m.interfaceName === st.id)
			);
			if (isManaged) continue;
			if (st.status === 'up' && st.peer) {
				feedTraffic(st.id, st.peer.rxBytes, st.peer.txBytes);
			}
		}
	});

	const goArch = $derived(sysInfo?.goArch ?? '');
	let singboxStatusState = $derived($singboxStatus);
	const singboxInstalled = $derived($singboxStatus.data?.installed ?? false);
	const singboxStatusLoading = $derived(
		singboxStatusState.lastFetchedAt === 0 &&
		(singboxStatusState.status === 'idle' || singboxStatusState.status === 'loading'),
	);

	let showUnsupportedBlock = $derived(
		sysInfo !== null &&
		!sysInfo.kernelModuleExists &&
		!sysInfo.kernelModuleLoaded &&
		!sysInfo.backendAvailability?.nativewg
	);

	let toggleLoading = $state<Record<string, boolean>>({});
	let pingChecking = $state<Record<string, boolean>>({});
	let connectivitySettingsOpen = $state(false);
	let connectivitySettingsTunnel = $state<TunnelListItem | null>(null);
	let deleteLoading = $state<Record<string, boolean>>({});
	let deleteConfirmId = $state<string | null>(null);
	let referencedDetails = $state<import('$lib/types').TunnelReferencedError | null>(null);
	let referencedTunnelName = $state<string>('');

	let detailId = $state<string | null>(null);
	let singboxDetailTag = $state<string | null>(null);
	let awgDiagnosticsTarget = $state<{ id: string; name: string; kind: 'awg' | 'system' } | null>(null);
	let endpointVisibility = $state<Record<string, boolean>>({});
	let awgListSearchQuery = $state('');
	let singboxTunnelsSearchQuery = $state('');
	let singboxSubscriptionsSearchQuery = $state('');
	let dashboardSearchQuery = $state('');

	function endpointVisibilityKey(scope: EndpointScope, id: string): string {
		return `${scope}:${id}`;
	}

	function endpointVisible(scope: EndpointScope, id: string): boolean {
		return endpointVisibility[endpointVisibilityKey(scope, id)] ?? false;
	}

	function toggleEndpointVisible(scope: EndpointScope, id: string): void {
		const key = endpointVisibilityKey(scope, id);
		endpointVisibility = {
			...endpointVisibility,
			[key]: !endpointVisibility[key],
		};
	}

	function endpointHost(endpoint?: string | null): string {
		const value = endpoint ?? '';
		const match = value.match(/^(?:\[([^\]]+)\]|([^:]+)):(\d+)$/);
		if (match) return match[1] || match[2] || value;
		return value;
	}

	function endpointPort(endpoint?: string | null): string {
		const value = endpoint ?? '';
		const match = value.match(/:(\d+)$/);
		return match ? match[1] : '';
	}

	function openDetail(id: string) {
		detailId = id;
		singboxDetailTag = null;
		const url = new URL(window.location.href);
		url.searchParams.set('detail', id);
		url.searchParams.delete('sbDetail');
		history.replaceState(history.state, '', url);
	}

	function closeDetail() {
		detailId = null;
		const url = new URL(window.location.href);
		url.searchParams.delete('detail');
		history.replaceState(history.state, '', url);
	}

	function openAwgDiagnostics(id: string, name: string, kind: 'awg' | 'system' = 'awg'): void {
		awgDiagnosticsTarget = { id, name, kind };
	}

	function closeAwgDiagnostics(): void {
		awgDiagnosticsTarget = null;
	}

	function openSingboxDetail(tag: string) {
		singboxDetailTag = tag;
		detailId = null;
		const url = new URL(window.location.href);
		url.searchParams.set('sbDetail', tag);
		url.searchParams.delete('detail');
		history.replaceState(history.state, '', url);
	}

	function closeSingboxDetail() {
		singboxDetailTag = null;
		const url = new URL(window.location.href);
		url.searchParams.delete('sbDetail');
		history.replaceState(history.state, '', url);
	}

	// Sync from URL on mount + whenever the page store changes (back/forward).
	$effect(() => {
		const awgQ = $page.url.searchParams.get('detail');
		const sbQ = $page.url.searchParams.get('sbDetail');
		detailId = awgQ && awgQ.length > 0 ? awgQ : null;
		singboxDetailTag = sbQ && sbQ.length > 0 ? sbQ : null;
	});

	async function markAsServer(id: string) {
		try {
			await api.markServerInterface(id);
			// markServerInterface returns fresh ServersSnapshot; the tunnels
			// list also changes (the system card disappears) — invalidate.
			tunnels.invalidate();
			notifications.success(`Туннель ${id} перенесён в серверы.`);
		} catch (e) {
			notifications.error(e instanceof Error ? e.message : 'Ошибка переноса в серверы');
		}
	}

	async function checkPing(id: string) {
		if (pingChecking[id]) return;
		pingChecking[id] = true;
		try {
			const result = await api.checkConnectivity(id);
			tunnels.updateConnectivity(id, result.connected, result.latency ?? null);
		} catch {
			tunnels.updateConnectivity(id, false, null);
		} finally {
			pingChecking[id] = false;
		}
	}

	function openConnectivitySettings(tunnel: TunnelListItem): void {
		connectivitySettingsTunnel = tunnel;
		connectivitySettingsOpen = true;
	}

	function closeConnectivitySettings(): void {
		connectivitySettingsOpen = false;
		connectivitySettingsTunnel = null;
	}

	async function handleToggleOnOff(id: string) {
		const tunnel = awgList.find(t => t.id === id);
		if (!tunnel) return;
		// needs_start is NOT "on" — it means "intent up but not actually running",
		// so the toggle should show OFF and the click should fire Start, not Stop.
		const isOn = ['running', 'starting', 'broken'].includes(tunnel.status);
		toggleLoading = { ...toggleLoading, [id]: true };
		try {
			if (isOn) {
				await tunnels.stop(id);
				notifications.success('Туннель остановлен');
			} else {
				await tunnels.start(id);
				notifications.success('Туннель запущен');
			}
		} catch (e) {
			notifications.error(e instanceof Error ? e.message : 'Ошибка');
		} finally {
			const { [id]: _, ...rest } = toggleLoading;
			toggleLoading = rest;
		}
	}

	function requestDelete(id: string) {
		deleteConfirmId = id;
	}

	async function handleDelete(id: string) {
		deleteConfirmId = null;
		deleteLoading = { ...deleteLoading, [id]: true };
		try {
			const result = await tunnels.remove(id);
			if (result.success && result.verified) {
				notifications.success('Туннель удалён');
			} else {
				notifications.error('Не удалось верифицировать удаление');
			}
		} catch (e) {
			if (e instanceof Error && e.message === 'tunnel_referenced') {
				const refErr = e as Error & {
					details: import('$lib/types').TunnelReferencedError;
				};
				referencedDetails = refErr.details;
				referencedTunnelName = awgList.find((t) => t.id === id)?.name ?? id;
			} else {
				notifications.error(e instanceof Error ? e.message : 'Не удалось удалить туннель');
			}
		} finally {
			const { [id]: _, ...rest } = deleteLoading;
			deleteLoading = rest;
		}
	}

	// Polling-store subscriptions for sing-box status + tunnels list.
	// First subscribe triggers fetch; last unsubscribe stops polling.
	let unsubSingboxStatus: (() => void) | undefined;
	let unsubSingboxTunnels: (() => void) | undefined;
	onMount(() => {
		unsubSingboxStatus = singboxStatus.subscribe(() => {});
		unsubSingboxTunnels = singboxTunnels.subscribe(() => {});
	});
	onDestroy(() => {
		unsubSingboxStatus?.();
		unsubSingboxTunnels?.();
	});

	let singboxTunnelsList = $derived($singboxTunnels.data ?? []);
	let singboxTunnelsInitialLoading = $derived(
		$singboxTunnels.data === null &&
		($singboxTunnels.status === 'idle' || $singboxTunnels.status === 'loading'),
	);

	const singboxTunnelListStats = $derived.by(() => {
		void trafficTick;
		const list = singboxTunnelsList;
		let running = 0;
		let down = 0;
		let up = 0;
		let delaySum = 0;
		let delayN = 0;
		let leaderBytes = 0;
		let leaderName = '—';
		const trMap = $singboxTraffic;
		const histMap = $singboxDelayHistory;
		for (const t of list) {
			if (t.running === true) running++;
			const tr = trMap.get(t.tag);
			if (tr) {
				const tunnelDown = tr.download ?? 0;
				const tunnelUp = tr.upload ?? 0;
				const total = tunnelDown + tunnelUp;
				down += tunnelDown;
				up += tunnelUp;
				if (total > leaderBytes) {
					leaderBytes = total;
					leaderName = t.tag;
				}
			}
			const h = histMap.get(t.tag) ?? [];
			const last = h.length > 0 ? h[h.length - 1] : 0;
			if (typeof last === 'number' && last > 0) {
				delaySum += last;
				delayN++;
			}
		}
		return {
			count: list.length,
			running,
			stopped: list.length - running,
			down,
			up,
			avgDelayMs: delayN > 0 ? Math.round(delaySum / delayN) : null,
			leaderBytes,
			leaderName,
		};
	});

	let subscriptionsState = $derived($subscriptionsStore);
	let subscriptionsList = $derived(subscriptionsState.data ?? []);
	let subscriptionsInitialLoading = $derived(
		subscriptionsState.data === null &&
		(subscriptionsState.status === 'idle' || subscriptionsState.status === 'loading'),
	);
	let subscriptionsFetchFailed = $derived(
		subscriptionsState.data === null && subscriptionsState.status === 'error',
	);
	let createModalOpen = $state(false);
	let wizardPreselect = $state<'choose' | 'single' | 'inline' | 'url'>('choose');

	function openWizard(preselect: 'choose' | 'single' | 'inline' | 'url'): void {
		wizardPreselect = preselect;
		createModalOpen = true;
	}

	let pendingSubscriptionDelete = $state<string | null>(null);
	let deletingSubscription = $state(false);

	function requestSubscriptionDelete(id: string): void {
		pendingSubscriptionDelete = id;
	}
	async function confirmSubscriptionDelete(): Promise<void> {
		if (!pendingSubscriptionDelete || deletingSubscription) return;
		const id = pendingSubscriptionDelete;
		deletingSubscription = true;
		try {
			await api.deleteSubscription(id);
			pendingSubscriptionDelete = null;
			await subscriptionsStore.refetch();
		} catch (e) {
			const name = pendingSubscriptionLabel || id;
			if (showOutboundReferencedError(e, name, 'Подписка')) {
				pendingSubscriptionDelete = null;
			} else {
				notifications.error(e instanceof Error ? e.message : 'Не удалось удалить подписку');
			}
		} finally {
			deletingSubscription = false;
		}
	}
	const pendingSubscriptionLabel = $derived.by(() => {
		const id = pendingSubscriptionDelete;
		if (!id) return '';
		const s = subscriptionsList.find((x) => x.id === id);
		return s ? s.label || s.url : id;
	});

	// Same as detail page — poll Clash for live "now" pointer this often.
	const URLTEST_POLL_MS = 5000;

	let liveActives = $state<Record<string, string>>({});

	$effect(() => {
		const urltestSubs = ($subscriptionsStore.data ?? []).filter(
			(s) => s.enabled && s.mode === 'urltest',
		);
		if (urltestSubs.length === 0) {
			liveActives = {};
			return;
		}
		let cancelled = false;
		const tick = async (): Promise<void> => {
			try {
				const results = await Promise.all(
					urltestSubs.map((s) =>
						api
							.getSubscriptionActiveNow(s.id)
							.then((r) => [s.id, r.now] as const)
							.catch(() => [s.id, ''] as const),
					),
				);
				if (cancelled) return;
				const next: Record<string, string> = {};
				for (const [id, now] of results) {
					if (now) next[id] = now;
				}
				liveActives = next;
			} catch {
				/* ignore — keep last known */
			}
		};
		void tick();
		const handle = setInterval(() => void tick(), URLTEST_POLL_MS);
		return () => {
			cancelled = true;
			clearInterval(handle);
		};
	});

	const subscriptionsActiveCards = $derived(
		($subscriptionsStore.data ?? [])
			// Selector-mode subs ship with activeMember="" — resolve first member instead of hiding the card.
			.filter((s) => s.enabled && (s.members?.length ?? 0) > 0)
			.map((s) => {
				const tag = resolveSubscriptionMemberTag(s, liveActives[s.id] || null);
				let m = s.members?.find((mm) => mm.tag === tag);
				if (!m && isMockDevMode && s.members?.length) {
					const first = s.members[0];
					m = tag
						? { ...first, tag, label: first.label || tag }
						: first;
				}
				return m ? { subscription: s, activeMember: m } : null;
			})
			.filter((x): x is { subscription: Subscription; activeMember: SubscriptionMember } => x !== null),
	);

	const subscriptionActiveIds = $derived(
		new Set(subscriptionsActiveCards.map((card) => card.subscription.id)),
	);

	const subscriptionsListRows = $derived(
		subscriptionsList.filter((subscription) => !subscriptionActiveIds.has(subscription.id)),
	);

	const singboxSubscriptionsTrafficStats = $derived.by(() => {
		void trafficTick;
		let down = 0;
		let up = 0;
		let delaySum = 0;
		let delaySamples = 0;
		let leaderBytes = 0;
		let leaderName = '—';
		const map = $singboxTraffic;
		const delayMap = $singboxDelayHistory;

		function ingestMember(tag: string, label: string, sampleDelay = false): void {
			const tr = map.get(tag);
			const memberDown = tr?.download ?? 0;
			const memberUp = tr?.upload ?? 0;
			const memberTotal = memberDown + memberUp;
			down += memberDown;
			up += memberUp;
			if (memberTotal > leaderBytes) {
				leaderBytes = memberTotal;
				leaderName = label || tag;
			}

			if (sampleDelay) {
				const delayHistory = delayMap.get(tag) ?? [];
				const lastDelay = delayHistory.length > 0 ? delayHistory[delayHistory.length - 1] : 0;
				if (typeof lastDelay === 'number' && lastDelay > 0) {
					delaySum += lastDelay;
					delaySamples += 1;
				}
			}
		}

		for (const card of subscriptionsActiveCards) {
			ingestMember(
				card.activeMember.tag,
				card.subscription.label || card.activeMember.label || card.activeMember.tag,
				true,
			);
		}
		for (const sub of subscriptionsListRows) {
			const tag = resolveSubscriptionMemberTag(sub, liveActives[sub.id] || null);
			if (!tag) continue;
			ingestMember(tag, sub.label || tag);
		}
		const totalTraffic = down + up;
		return {
			count: subscriptionsList.length,
			activeCount: subscriptionsActiveCards.length,
			inactiveCount: subscriptionsListRows.length,
			down,
			up,
			avgDelayMs: delaySamples > 0 ? Math.round(delaySum / delaySamples) : null,
			delaySamples,
			leaderBytes,
			leaderName,
			leaderSharePct: totalTraffic > 0 ? Math.round((leaderBytes / totalTraffic) * 100) : 0,
		};
	});

	// Tabs
	let activeTab = $state<TunnelTab>('awg');
	let awgViewMode = $state<AwgTunnelViewMode>('compact');
	let awgViewModeReady = false;
	let isAwgMobile = $state(readTunnelMobileLayout());
	let showAwgViewModeSwitch = $derived($usageLevel !== 'basic');
	let singboxTunnelsLayoutMode = $state<SingboxLayoutMode>('compact');
	let singboxSubscriptionsLayoutMode = $state<SingboxLayoutMode>('compact');
	let singboxTunnelsLayoutReady = false;
	let singboxSubscriptionsLayoutReady = false;
	let showSingboxListOption = $derived($usageLevel !== 'basic');
	let singboxTunnelsEffectiveLayout = $derived<SingboxLayoutMode>(
		!showSingboxListOption && singboxTunnelsLayoutMode === 'list'
			? 'compact'
			: singboxTunnelsLayoutMode,
	);
	let singboxSubscriptionsEffectiveLayout = $derived<SingboxLayoutMode>(
		!showSingboxListOption && singboxSubscriptionsLayoutMode === 'list'
			? 'compact'
			: singboxSubscriptionsLayoutMode,
	);
	let showSingboxGridListToggle = $derived(showSingboxListOption);
	let awgEffectiveViewMode = $derived(!showAwgViewModeSwitch ? 'compact' : awgViewMode);
	let awgRenderMode = $derived(resolveTunnelRenderMode(isAwgMobile, awgEffectiveViewMode));
	let singboxTunnelsRenderMode = $derived(
		resolveTunnelRenderMode(isAwgMobile, singboxTunnelsEffectiveLayout),
	);
	let singboxSubscriptionsRenderMode = $derived(
		resolveTunnelRenderMode(isAwgMobile, singboxSubscriptionsEffectiveLayout),
	);
	let awgCardViewMode = $derived<'cards' | 'compact'>(
		awgEffectiveViewMode === 'cards' ? 'cards' : 'compact',
	);

	// Dashboard mode is only reachable while its settings toggle is available
	// («advanced»+): on «basic» a persisted flag must fall back to tabs
	// instead of locking the user into a hidden mode.
	let dashboardOn = $derived($tunnelDashboardMode && isTunnelDashboardAvailable($usageLevel));
	const showSingboxSections = $derived(isSectionVisible($usageLevel, 'singboxTunnels'));
	// Sing-box data is admitted into the dashboard only while its sections are
	// visible at this usage level AND sing-box is installed (or still probing).
	const dashboardSingboxVisible = $derived(
		showSingboxSections && (singboxStatusLoading || singboxInstalled),
	);
	let dashboardEffectiveView = $derived(
		!showSingboxListOption && $tunnelDashboardView === 'list' ? 'compact' : $tunnelDashboardView,
	);
	let dashboardAwgViewMode = $derived<AwgTunnelViewMode>(
		dashboardEffectiveView === 'dense' ? 'cards' : dashboardEffectiveView === 'compact' ? 'compact' : 'list',
	);
	let dashboardSingboxLayoutMode = $derived<SingboxLayoutMode>(
		dashboardEffectiveView === 'dense'
			? 'dense'
			: dashboardEffectiveView === 'list'
				? 'list'
				: 'compact',
	);
	let dashboardFlatLayout = $derived($tunnelDashboardLayout === 'flat');
	let dashboardSectionsLayout = $derived(dashboardOn && $tunnelDashboardLayout === 'sections');
	let dashboardFlatCardMode = $derived(dashboardOn && dashboardFlatLayout);
	// Sections layout splits by kind ('type') or by user tags ('tags').
	let dashboardGroupByTags = $derived(
		dashboardSectionsLayout && $tunnelDashboardGroupMode === 'tags',
	);
	let dashboardTypeSections = $derived(
		dashboardSectionsLayout && $tunnelDashboardGroupMode === 'type',
	);
	// Merged card surfaces (flat list / tag groups) can't host per-kind tables:
	// «список» renders the merged list-card list (same as mobile) there. Card
	// densities are honest in every layout: dense → full cards with graphs,
	// compact → compact cards.
	let dashboardCardSurface = $derived(dashboardFlatLayout || dashboardGroupByTags);
	let dashboardAwgRenderMode = $derived<TunnelRenderMode>(
		dashboardCardSurface && dashboardAwgViewMode === 'list'
			? 'list-card'
			: resolveTunnelRenderMode(isAwgMobile, dashboardAwgViewMode),
	);
	let dashboardSingboxRenderMode = $derived<TunnelRenderMode>(
		dashboardCardSurface && dashboardSingboxLayoutMode === 'list'
			? 'list-card'
			: resolveTunnelRenderMode(isAwgMobile, dashboardSingboxLayoutMode),
	);
	let dashboardCardViewMode = $derived<'cards' | 'compact'>(
		dashboardAwgViewMode === 'cards' ? 'cards' : 'compact',
	);
	let effectiveAwgRenderMode = $derived(dashboardOn ? dashboardAwgRenderMode : awgRenderMode);
	let effectiveAwgEffectiveViewMode = $derived(
		dashboardOn ? dashboardAwgViewMode : awgEffectiveViewMode,
	);
	let effectiveAwgCardViewMode = $derived(
		dashboardOn ? dashboardCardViewMode : awgCardViewMode,
	);
	let effectiveSingboxTunnelsRenderMode = $derived(
		dashboardOn ? dashboardSingboxRenderMode : singboxTunnelsRenderMode,
	);
	let effectiveSingboxTunnelsEffectiveLayout = $derived(
		dashboardOn ? dashboardSingboxLayoutMode : singboxTunnelsEffectiveLayout,
	);
	let effectiveSingboxSubscriptionsRenderMode = $derived(
		dashboardOn ? dashboardSingboxRenderMode : singboxSubscriptionsRenderMode,
	);
	let effectiveSingboxSubscriptionsEffectiveLayout = $derived(
		dashboardOn ? dashboardSingboxLayoutMode : singboxSubscriptionsEffectiveLayout,
	);
	let effectiveAwgSearchQuery = $derived(dashboardOn ? dashboardSearchQuery : awgListSearchQuery);
	let effectiveSingboxTunnelsSearchQuery = $derived(
		dashboardOn ? dashboardSearchQuery : singboxTunnelsSearchQuery,
	);
	let effectiveSubscriptionsSearchQuery = $derived(
		dashboardOn ? dashboardSearchQuery : singboxSubscriptionsSearchQuery,
	);

	function isAwgTunnelViewMode(value: string | null): value is AwgTunnelViewMode {
		return value === 'cards' || value === 'compact' || value === 'list';
	}

	const tunnelTabs = $derived(
		[
			{ id: 'awg', label: 'AWG', badge: awgList.length + systemList.length },
			isSectionVisible($usageLevel, 'singboxTunnels')
				? { id: 'singbox', label: 'Sing-box туннели', badge: singboxTunnelsList.length }
				: null,
			isSectionVisible($usageLevel, 'singboxTunnels')
				? { id: 'subscriptions', label: 'Sing-box подписки', badge: subscriptionsList.length }
				: null,
		].filter((t): t is { id: string; label: string; badge: number } => t !== null),
	);

	// Auto-switch off sing-box tab if it becomes hidden (basic mode).
	$effect(() => {
		if (!tunnelTabs.find((t) => t.id === activeTab)) {
			activeTab = 'awg';
		}
	});

	onMount(() => {
		const stored = localStorage.getItem(AWG_TUNNEL_VIEW_STORAGE_KEY);
		if (isAwgTunnelViewMode(stored)) {
			awgViewMode = stored;
		}
		awgViewModeReady = true;

		// Backward compatible migration:
		// if per-tab keys are missing, fall back to the old shared sing-box layout key.
		const legacyShared = localStorage.getItem(SINGBOX_LAYOUT_STORAGE_KEY);

		const sbTunnels = localStorage.getItem(SINGBOX_TUNNELS_LAYOUT_STORAGE_KEY) ?? legacyShared;
		const parsedTunnels = parseSingboxLayoutMode(sbTunnels);
		if (parsedTunnels) singboxTunnelsLayoutMode = parsedTunnels;
		singboxTunnelsLayoutReady = true;

		const sbSubscriptions =
			localStorage.getItem(SINGBOX_SUBSCRIPTIONS_LAYOUT_STORAGE_KEY) ?? legacyShared;
		const parsedSubscriptions = parseSingboxLayoutMode(sbSubscriptions);
		if (parsedSubscriptions) singboxSubscriptionsLayoutMode = parsedSubscriptions;
		singboxSubscriptionsLayoutReady = true;
	});

	onMount(() => subscribeTunnelMobileLayout((mobile) => {
		isAwgMobile = mobile;
	}));

	$effect(() => {
		if (!awgViewModeReady) return;
		localStorage.setItem(AWG_TUNNEL_VIEW_STORAGE_KEY, awgViewMode);
	});

	$effect(() => {
		if (!singboxTunnelsLayoutReady) return;
		localStorage.setItem(SINGBOX_TUNNELS_LAYOUT_STORAGE_KEY, singboxTunnelsLayoutMode);
	});

	$effect(() => {
		if (!singboxSubscriptionsLayoutReady) return;
		localStorage.setItem(
			SINGBOX_SUBSCRIPTIONS_LAYOUT_STORAGE_KEY,
			singboxSubscriptionsLayoutMode,
		);
	});

	let awgAutoConnectivityNonce = $state(0);
	let singboxAutoDelayCheckNonce = $state(0);
	let lastAutoCheckKey = '';
	// Separate dedupe key for the dashboard: there both the AWG connectivity
	// effect and the sing-box delay effect are live at once, so sharing
	// lastAutoCheckKey would ping-pong and re-trigger checks on every poll.
	let lastDashboardDelayKey = '';
	let currentTunnelSurface = '';
	let tunnelSurfaceEntryNonce = $state(0);

	function activeAwgConnectivityIds(): string {
		return awgList
			.filter((t) =>
				t.enabled &&
				(t.status === 'running' || t.status === 'broken') &&
				(t.connectivityCheck?.method ?? 'http') !== 'disabled'
			)
			.map((t) => t.id)
			.sort()
			.join(',');
	}

	function activeSingboxDelayTags(): string {
		return singboxTunnelsList
			.filter((t) => t.running === true)
			.map((t) => t.tag)
			.sort()
			.join(',');
	}

	function activeSubscriptionDelayTags(): string {
		return subscriptionsActiveCards
			.map((card) => card.activeMember.tag)
			.filter(Boolean)
			.sort()
			.join(',');
	}

	$effect(() => {
		const surface = $page.url.pathname === '/' ? activeTab : 'outside';
		if (surface === currentTunnelSurface) return;
		currentTunnelSurface = surface;
		tunnelSurfaceEntryNonce += 1;
	});

	$effect(() => {
		const path = $page.url.pathname;
		const tab = activeTab;
		const entry = tunnelSurfaceEntryNonce;
		if (path !== '/' || tab !== 'awg' || loading) return;

		const ids = activeAwgConnectivityIds();
		if (!ids) return;

		const key = `awg:${entry}:${ids}`;
		if (key === lastAutoCheckKey) return;
		lastAutoCheckKey = key;
		awgAutoConnectivityNonce += 1;
	});

	// Только в табличном рендере не рендерятся TunnelCard — там срабатывает autoConnectivity.
	// Иначе connectivityMap не заполняется и подстрока статуса залипает на «Проверка…».
	// В list-card (мобильный «список» и сплошной дашборд) карточки сами
	// проверяются по тому же nonce — дублировать запросы со страницы нельзя.
	$effect(() => {
		const mode = effectiveAwgRenderMode;
		const nonce = awgAutoConnectivityNonce;
		if (mode !== 'table' || loading || nonce <= 0) return;

		const targets = untrack(() =>
			awgList.filter(
				(t) =>
					t.enabled &&
					(t.status === 'running' || t.status === 'broken') &&
					(t.connectivityCheck?.method ?? 'http') !== 'disabled',
			),
		);
		if (targets.length === 0) return;

		const timers: ReturnType<typeof setTimeout>[] = [];
		for (let i = 0; i < targets.length; i++) {
			const id = targets[i].id;
			timers.push(
				setTimeout(() => {
					void api
						.checkConnectivity(id)
						.then((result) => {
							tunnels.updateConnectivity(id, result.connected, result.latency ?? null);
						})
						.catch(() => {
							tunnels.updateConnectivity(id, false, null);
						});
				}, i * 180),
			);
		}
		return () => {
			for (const t of timers) clearTimeout(t);
		};
	});

	$effect(() => {
		const path = $page.url.pathname;
		const tab = activeTab;
		const entry = tunnelSurfaceEntryNonce;
		if (path !== '/') return;

		// Dashboard shows sing-box tunnels and subscriptions at once — run the
		// auto delay checks for both regardless of the (hidden) active tab.
		if (dashboardOn) {
			const sbTags = dashboardSingboxTunnels
				.filter((t) => t.running === true)
				.map((t) => t.tag)
				.sort()
				.join(',');
			const subTags = dashboardSubscriptionsActive
				.map((card) => card.activeMember.tag)
				.filter(Boolean)
				.sort()
				.join(',');
			if (!sbTags && !subTags) return;

			// Отдельные префиксы групп: набор тегов туннелей и подписок не
			// должен схлопываться в один ключ при перестановке между группами.
			const key = `dashboard:${entry}:sb:${sbTags}|sub:${subTags}`;
			if (key === lastDashboardDelayKey) return;
			lastDashboardDelayKey = key;
			singboxAutoDelayCheckNonce += 1;
			return;
		}

		if (tab !== 'singbox' && tab !== 'subscriptions') return;

		const tags = tab === 'singbox'
			? activeSingboxDelayTags()
			: activeSubscriptionDelayTags();
		if (!tags) return;

		const key = `${tab}:${entry}:${tags}`;
		if (key === lastAutoCheckKey) return;
		lastAutoCheckKey = key;
		singboxAutoDelayCheckNonce += 1;
	});

	// External tunnels
	let adoptDialogOpen = $state(false);
	let adoptingInterface = $state('');
	let adoptError = $state('');
	let adoptLoading = $state(false);

	function handleAdoptClick(interfaceName: string): void {
		adoptingInterface = interfaceName;
		adoptDialogOpen = true;
	}

	async function handleAdopt(data: { content: string; name: string }): Promise<void> {
		adoptLoading = true;
		adoptError = '';
		try {
			const adopted = await tunnels.adoptExternal(adoptingInterface, data.content, data.name);
			if (adopted.warnings?.length) {
				adopted.warnings.forEach(w => notifications.warning(w));
			}
			notifications.success('Туннель успешно импортирован');
			adoptDialogOpen = false;
		} catch (e) {
			adoptError = e instanceof Error ? e.message : 'Не удалось импортировать туннель';
		} finally {
			adoptLoading = false;
		}
	}

	// Empty state: inline drag-and-drop import
	let dragOver = $state(false);
	let importing = $state(false);

	let exporting = $state(false);

	async function handleExportAll() {
		exporting = true;
		try {
			const blob = await api.exportAllTunnels();
			const { downloadBlob } = await import('$lib/utils/download');
			downloadBlob(blob, 'awg-tunnels.zip');
		} catch (e) {
			notifications.error('Не удалось экспортировать конфиги');
		} finally {
			exporting = false;
		}
	}

	function handleDrop(event: DragEvent) {
		event.preventDefault();
		dragOver = false;
		if (event.dataTransfer?.files?.[0]) {
			readAndImport(event.dataTransfer.files[0]);
		}
	}

	function handleDragOver(event: DragEvent) {
		event.preventDefault();
		dragOver = true;
	}

	function handleDragLeave() {
		dragOver = false;
	}

	let selectedBackend = $state<'nativewg' | 'kernel'>('nativewg');

	let nativewgHint = $derived(
		sysInfo !== null && !sysInfo.backendAvailability?.nativewg
			? nativewgUnavailableHint(sysInfo.nativewgReason)
			: ''
	);

	// Auto-select backend based on availability
	$effect(() => {
		if (sysInfo?.backendAvailability && !sysInfo.backendAvailability.nativewg && sysInfo.backendAvailability.kernel) {
			selectedBackend = 'kernel';
		}
	});

	let fileInput = $state<HTMLInputElement>();

	function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		if (input.files?.[0]) {
			readAndImport(input.files[0]);
		}
	}

	function readAndImport(file: File) {
		const reader = new FileReader();
		reader.onload = async (e) => {
			const content = e.target?.result as string;
			if (!content?.trim()) return;
			importing = true;
			try {
				const name = file.name.replace(/\.conf$/i, '');
				const tunnel = await tunnels.importConfig(content, name, selectedBackend);
				if (tunnel.warnings?.length) {
					tunnel.warnings.forEach(w => notifications.warning(w));
				}
				notifications.success('Туннель импортирован');
				goto(`/tunnels/${tunnel.id}`);
			} catch (err) {
				notifications.error(err instanceof Error ? err.message : 'Ошибка импорта');
			} finally {
				importing = false;
			}
		};
		reader.readAsText(file);
	}

	// Terminal status line
	let statusLine = $derived.by(() => {
		if (!sysInfo) return '';
		const count = awgList.length;
		const word = count === 0 ? 'туннелей' : count === 1 ? 'туннель' : count < 5 ? 'туннеля' : 'туннелей';
		return `${sysInfo.version}  ·  ${sysInfo.goArch}  ·  ${count} ${word}`;
	});

	let visibleSystemList = $derived(
		systemList.filter((st) =>
			!awgList.some((mt) =>
				(mt.ndmsName && mt.ndmsName === st.id) ||
				(mt.interfaceName && mt.interfaceName === st.id)
			)
		),
	);

	function tunnelStatusBucket(status: string): 'running' | 'broken' | 'starting' | 'stopped' | 'disabled' | 'other' {
		switch (status) {
			case 'running':
				return 'running';
			case 'broken':
				return 'broken';
			case 'starting':
			case 'needs_stop':
			case 'stopping':
				return 'starting';
			case 'needs_start':
			case 'stopped':
			case 'not_created':
				return 'stopped';
			case 'disabled':
				return 'disabled';
			default:
				return 'other';
		}
	}

	function isManagedTunnelOn(tunnel: TunnelListItem): boolean {
		return ['running', 'starting', 'broken'].includes(tunnel.status);
	}

	function showManagedPing(
		tunnel: TunnelListItem,
		connectivity: { connected: boolean; latency: number | null } | undefined,
	): boolean {
		return awgListShowsPingButton(tunnel, connectivity);
	}

	function managedRouteMeta(tunnel: TunnelListItem): string {
		const iface = tunnel.resolvedIspInterface || tunnel.ispInterface || '';
		const label = tunnel.resolvedIspInterfaceLabel || tunnel.ispInterfaceLabel || '';
		if (label && iface) return label === iface ? label : `${label} (${iface})`;
		if (label) return label;
		if (iface) return iface;
		return 'Маршрут не установлен';
	}

	function systemStatusVariant(tunnel: SystemTunnel): 'success' | 'muted' {
		return tunnel.status === 'up' ? 'success' : 'muted';
	}

	function systemStatusLabel(tunnel: SystemTunnel): string {
		if (tunnel.status !== 'up') return 'Выключен';
		return tunnel.peer?.online ? 'Активен' : 'Без handshake';
	}

	function externalStatusVariant(tunnel: ExternalTunnel): 'success' | 'muted' {
		return tunnel.lastHandshake ? 'success' : 'muted';
	}

	function externalStatusLabel(tunnel: ExternalTunnel): string {
		return tunnel.lastHandshake ? 'Подключён' : 'Неактивен';
	}

	function latestRate(id: string): { rx: number; tx: number } {
		void trafficTick;
		const rates = getTrafficRates(id);
		return {
			rx: rates.rx.length > 0 ? rates.rx[rates.rx.length - 1] : 0,
			tx: rates.tx.length > 0 ? rates.tx[rates.tx.length - 1] : 0,
		};
	}

	function sparklineSeries(id: string): { rx: number[]; tx: number[] } {
		void trafficTick;
		return getTrafficSparklineSeries(id, 28);
	}

	let awgSummaryTotal = $derived(awgList.length + visibleSystemList.length + externalList.length);
	let awgSummaryActive = $derived(
		awgList.filter((t) => isManagedTunnelOn(t)).length +
		visibleSystemList.filter((t) => t.status === 'up').length +
		externalList.filter((t) => !!t.lastHandshake).length,
	);

	let awgSummaryPeak = $derived.by(() => {
		let rate = 0;
		let name = '—';

		for (const tunnel of awgList) {
			if (!isManagedTunnelOn(tunnel)) continue;
			const latest = latestRate(tunnel.id);
			const combined = latest.rx + latest.tx;
			if (combined > rate) {
				rate = combined;
				name = tunnel.name;
			}
		}

		for (const tunnel of visibleSystemList) {
			if (tunnel.status !== 'up') continue;
			const latest = latestRate(tunnel.id);
			const combined = latest.rx + latest.tx;
			if (combined > rate) {
				rate = combined;
				name = tunnel.description || tunnel.interfaceName;
			}
		}

		return { rate, name };
	});

	let awgSummaryRx = $derived(
		awgList.reduce((sum, tunnel) => sum + (tunnel.rxBytes ?? 0), 0) +
		visibleSystemList.reduce((sum, tunnel) => sum + (tunnel.peer?.rxBytes ?? 0), 0) +
		externalList.reduce((sum, tunnel) => sum + tunnel.rxBytes, 0),
	);

	let awgSummaryTx = $derived(
		awgList.reduce((sum, tunnel) => sum + (tunnel.txBytes ?? 0), 0) +
		visibleSystemList.reduce((sum, tunnel) => sum + (tunnel.peer?.txBytes ?? 0), 0) +
		externalList.reduce((sum, tunnel) => sum + tunnel.txBytes, 0),
	);

	let awgTrafficLeader = $derived.by(() => {
		let bytes = 0;
		let name = '—';

		for (const tunnel of awgList) {
			const total = (tunnel.rxBytes ?? 0) + (tunnel.txBytes ?? 0);
			if (total > bytes) {
				bytes = total;
				name = tunnel.name;
			}
		}

		for (const tunnel of visibleSystemList) {
			const total = (tunnel.peer?.rxBytes ?? 0) + (tunnel.peer?.txBytes ?? 0);
			if (total > bytes) {
				bytes = total;
				name = tunnel.description || tunnel.interfaceName;
			}
		}

		for (const tunnel of externalList) {
			const total = tunnel.rxBytes + tunnel.txBytes;
			if (total > bytes) {
				bytes = total;
				name = tunnel.interfaceName;
			}
		}

		return { bytes, name };
	});

	const awgSortOptions: TunnelSortOption[] = [
		{ value: 'name', label: 'По имени' },
		{ value: 'status', label: 'По статусу' },
		{ value: 'endpoint', label: 'По endpoint' },
		{ value: 'traffic', label: 'По трафику' },
		{ value: 'handshake', label: 'По handshake' },
	];

	const singboxTunnelSortOptions: TunnelSortOption[] = [
		{ value: 'delay', label: 'По delay' },
		{ value: 'name', label: 'По имени' },
		{ value: 'protocol', label: 'По протоколу' },
		{ value: 'server', label: 'По серверу' },
		{ value: 'running', label: 'По процессу' },
		{ value: 'traffic', label: 'По трафику' },
		{ value: 'ping', label: 'По ping' },
	];

	const subscriptionSortOptions: TunnelSortOption[] = [
		{ value: 'delay', label: 'По delay' },
		{ value: 'label', label: 'По имени' },
		{ value: 'mode', label: 'По режиму' },
		{ value: 'active', label: 'По активному серверу' },
		{ value: 'traffic', label: 'По трафику' },
		{ value: 'updated', label: 'По обновлению' },
		{ value: 'ping', label: 'По ping' },
	];

	function handleAwgSortChange(key: AwgTunnelSortKey): void {
		awgTunnelTableSort.toggleSort(key);
	}

	function handleSingboxTunnelSortChange(key: SingboxTunnelSortKey): void {
		singboxTunnelTableSort.toggleSort(key);
	}

	function handleSubscriptionSortChange(key: SubscriptionSortKey): void {
		singboxSubscriptionTableSort.toggleSort(key);
	}

	function matchQuery(values: Array<string | null | undefined>, query: string): boolean {
		const q = query.trim().toLowerCase();
		if (!q) return true;
		return values.some((value) => String(value ?? '').toLowerCase().includes(q));
	}

	function awgStatusRank(tunnel: TunnelListItem): number {
		switch (tunnelStatusBucket(tunnel.status)) {
			case 'running':
				return 0;
			case 'starting':
				return 1;
			case 'broken':
				return 2;
			case 'stopped':
				return 3;
			case 'disabled':
				return 4;
			default:
				return 5;
		}
	}

	let sortedFilteredAwgList = $derived.by(() => {
		const query = effectiveAwgSearchQuery.trim().toLowerCase();
		const filtered = awgList.filter((tunnel) =>
			matchQuery(
				[
					tunnel.name,
					tunnel.interfaceName,
					tunnel.id,
					tunnel.ndmsName,
					tunnel.address,
					tunnel.endpoint,
					tunnel.backend,
					tunnel.awgVersion,
				],
				query,
			),
		);
		const sortBy = $awgTunnelTableSort.sortBy;
		if (!sortBy) return filtered;
		const asc = $awgTunnelTableSort.sortAsc;
		return [...filtered].sort((a, b) => {
			switch (sortBy) {
				case 'name':
					return applyDirection(compareString(a.name, b.name), asc);
				case 'status':
					return applyDirection(compareNullableNumber(awgStatusRank(a), awgStatusRank(b), false), asc);
				case 'endpoint':
					return applyDirection(compareString(a.endpoint, b.endpoint), asc);
				case 'traffic':
					return applyDirection(
						compareNullableNumber((a.rxBytes ?? 0) + (a.txBytes ?? 0), (b.rxBytes ?? 0) + (b.txBytes ?? 0), false),
						asc,
					);
				case 'handshake':
					return applyDirection(
						compareNullableNumber(
							a.lastHandshake ? new Date(a.lastHandshake).getTime() : null,
							b.lastHandshake ? new Date(b.lastHandshake).getTime() : null,
						),
						asc,
					);
			}
		});
	});

	let sortedFilteredSystemList = $derived.by(() => {
		const query = effectiveAwgSearchQuery.trim().toLowerCase();
		const filtered = visibleSystemList.filter((tunnel) =>
			matchQuery(
				[
					tunnel.description,
					tunnel.interfaceName,
					tunnel.id,
					tunnel.address,
					tunnel.peer?.endpoint,
					tunnel.peer?.via,
				],
				query,
			),
		);
		const sortBy = $awgTunnelTableSort.sortBy;
		if (!sortBy) return filtered;
		const asc = $awgTunnelTableSort.sortAsc;
		return [...filtered].sort((a, b) => {
			switch (sortBy) {
				case 'name':
					return applyDirection(compareString(a.description || a.id, b.description || b.id), asc);
				case 'status':
					return applyDirection(compareBool(a.status === 'up', b.status === 'up'), asc);
				case 'endpoint':
					return applyDirection(compareString(a.peer?.endpoint, b.peer?.endpoint), asc);
				case 'traffic':
					return applyDirection(
						compareNullableNumber(
							(a.peer?.rxBytes ?? 0) + (a.peer?.txBytes ?? 0),
							(b.peer?.rxBytes ?? 0) + (b.peer?.txBytes ?? 0),
							false,
						),
						asc,
					);
				case 'handshake':
					return applyDirection(
						compareNullableNumber(
							a.peer?.lastHandshake ? new Date(a.peer.lastHandshake).getTime() : null,
							b.peer?.lastHandshake ? new Date(b.peer.lastHandshake).getTime() : null,
						),
						asc,
					);
			}
		});
	});

	let sortedFilteredExternalList = $derived.by(() => {
		const query = effectiveAwgSearchQuery.trim().toLowerCase();
		const filtered = externalList.filter((tunnel) =>
			matchQuery([tunnel.interfaceName, tunnel.endpoint, tunnel.publicKey, tunnel.isAWG ? 'awg' : 'wg'], query),
		);
		const sortBy = $awgTunnelTableSort.sortBy;
		if (!sortBy) return filtered;
		const asc = $awgTunnelTableSort.sortAsc;
		return [...filtered].sort((a, b) => {
			switch (sortBy) {
				case 'name':
					return applyDirection(compareString(a.interfaceName, b.interfaceName), asc);
				case 'status':
					return applyDirection(compareBool(!!a.lastHandshake, !!b.lastHandshake), asc);
				case 'endpoint':
					return applyDirection(compareString(a.endpoint, b.endpoint), asc);
				case 'traffic':
					return applyDirection(compareNullableNumber(a.rxBytes + a.txBytes, b.rxBytes + b.txBytes, false), asc);
				case 'handshake':
					return applyDirection(
						compareNullableNumber(
							a.lastHandshake ? new Date(a.lastHandshake).getTime() : null,
							b.lastHandshake ? new Date(b.lastHandshake).getTime() : null,
						),
						asc,
					);
			}
		});
	});

	let singboxTunnelDelayValue = $derived.by(() => {
		const map = new Map<string, number | null>();
		for (const tunnel of singboxTunnelsList) {
			const history = $singboxDelayHistory.get(tunnel.tag) ?? [];
			const latest = history.length > 0 ? history[history.length - 1] : null;
			map.set(tunnel.tag, latest && latest > 0 ? latest : null);
		}
		return map;
	});

	let sortedFilteredSingboxTunnels = $derived.by(() => {
		const query = effectiveSingboxTunnelsSearchQuery.trim().toLowerCase();
		const filtered = singboxTunnelsList.filter((tunnel) =>
			matchQuery(
				[
					tunnel.tag,
					tunnel.protocol,
					tunnel.server,
					tunnel.proxyInterface,
					tunnel.kernelInterface,
					tunnel.transport,
					tunnel.security,
				],
				query,
			),
		);
		const sortBy = $singboxTunnelTableSort.sortBy;
		if (!sortBy) return filtered;
		const asc = $singboxTunnelTableSort.sortAsc;
		return [...filtered].sort((a, b) => {
			switch (sortBy) {
				case 'delay':
					return compareDelayLike(singboxTunnelDelayValue.get(a.tag), singboxTunnelDelayValue.get(b.tag), asc);
				case 'ping':
					return compareDelayLike(singboxTunnelDelayValue.get(a.tag), singboxTunnelDelayValue.get(b.tag), asc);
				case 'name':
					return applyDirection(compareString(a.tag, b.tag), asc);
				case 'protocol':
					return applyDirection(compareString(a.protocol, b.protocol), asc);
				case 'server':
					return applyDirection(compareString(`${a.server}:${a.port}`, `${b.server}:${b.port}`), asc);
				case 'running':
					return applyDirection(compareBool(a.running, b.running), asc);
				case 'traffic':
					return applyDirection(
						compareNullableNumber(
							($singboxTraffic.get(a.tag)?.download ?? 0) + ($singboxTraffic.get(a.tag)?.upload ?? 0),
							($singboxTraffic.get(b.tag)?.download ?? 0) + ($singboxTraffic.get(b.tag)?.upload ?? 0),
							false,
						),
						asc,
					);
			}
		});
	});

	function subscriptionTrafficBytes(subscription: Subscription, activeTag: string | null): number {
		if (!activeTag) return 0;
		const traffic = $singboxTraffic.get(activeTag);
		return (traffic?.download ?? 0) + (traffic?.upload ?? 0);
	}

	function subscriptionDelayValue(subscription: Subscription, activeTag: string | null): number | null {
		if (!activeTag) return null;
		const history = $singboxDelayHistory.get(activeTag) ?? [];
		const latest = history.length > 0 ? history[history.length - 1] : null;
		return latest && latest > 0 ? latest : null;
	}

	let sortedFilteredSubscriptionsActiveCards = $derived.by(() => {
		const query = effectiveSubscriptionsSearchQuery.trim().toLowerCase();
		const filtered = subscriptionsActiveCards.filter(({ subscription, activeMember }) =>
			matchQuery(
				[
					subscription.label,
					subscription.url,
					subscription.inboundTag,
					subscription.selectorTag,
					activeMember.tag,
					activeMember.label,
					activeMember.server,
					`Proxy${subscription.proxyIndex}`,
					`t2s${subscription.proxyIndex}`,
				],
				query,
			),
		);
		const sortBy = $singboxSubscriptionTableSort.sortBy;
		if (!sortBy) return filtered;
		const asc = $singboxSubscriptionTableSort.sortAsc;
		return [...filtered].sort((a, b) => {
			switch (sortBy) {
				case 'delay':
					return compareDelayLike(subscriptionDelayValue(a.subscription, a.activeMember.tag), subscriptionDelayValue(b.subscription, b.activeMember.tag), asc);
				case 'ping':
					return compareDelayLike(subscriptionDelayValue(a.subscription, a.activeMember.tag), subscriptionDelayValue(b.subscription, b.activeMember.tag), asc);
				case 'label':
					return applyDirection(compareString(a.subscription.label, b.subscription.label), asc);
				case 'mode':
					return applyDirection(compareString(a.subscription.mode, b.subscription.mode), asc);
				case 'active':
					return applyDirection(compareString(a.activeMember.label || a.activeMember.tag, b.activeMember.label || b.activeMember.tag), asc);
				case 'traffic':
					return applyDirection(compareNullableNumber(subscriptionTrafficBytes(a.subscription, a.activeMember.tag), subscriptionTrafficBytes(b.subscription, b.activeMember.tag), false), asc);
				case 'updated':
					return applyDirection(compareNullableNumber(
						a.subscription.lastFetched ? new Date(a.subscription.lastFetched).getTime() : null,
						b.subscription.lastFetched ? new Date(b.subscription.lastFetched).getTime() : null,
					), asc);
			}
		});
	});

	let sortedFilteredSubscriptionsListRows = $derived.by(() => {
		const query = effectiveSubscriptionsSearchQuery.trim().toLowerCase();
		const filtered = subscriptionsListRows.filter((subscription) => {
			const activeTag = liveActives[subscription.id] || null;
			const member = subscription.members?.find((m) => m.tag === activeTag) ?? null;
			return matchQuery(
				[
					subscription.label,
					subscription.url,
					subscription.inboundTag,
					subscription.selectorTag,
					member?.tag,
					member?.label,
					member?.server,
					`Proxy${subscription.proxyIndex}`,
					`t2s${subscription.proxyIndex}`,
				],
				query,
			);
		});
		const sortBy = $singboxSubscriptionTableSort.sortBy;
		if (!sortBy) return filtered;
		const asc = $singboxSubscriptionTableSort.sortAsc;
		return [...filtered].sort((a, b) => {
			const activeA = liveActives[a.id] || resolveSubscriptionMemberTag(a, null);
			const activeB = liveActives[b.id] || resolveSubscriptionMemberTag(b, null);
			const memberA = a.members?.find((m) => m.tag === activeA) ?? null;
			const memberB = b.members?.find((m) => m.tag === activeB) ?? null;
			switch (sortBy) {
				case 'delay':
					return compareDelayLike(subscriptionDelayValue(a, activeA), subscriptionDelayValue(b, activeB), asc);
				case 'ping':
					return compareDelayLike(subscriptionDelayValue(a, activeA), subscriptionDelayValue(b, activeB), asc);
				case 'label':
					return applyDirection(compareString(a.label, b.label), asc);
				case 'mode':
					return applyDirection(compareString(a.mode, b.mode), asc);
				case 'active':
					return applyDirection(compareString(memberA?.label || memberA?.tag, memberB?.label || memberB?.tag), asc);
				case 'traffic':
					return applyDirection(compareNullableNumber(subscriptionTrafficBytes(a, activeA), subscriptionTrafficBytes(b, activeB), false), asc);
				case 'updated':
					return applyDirection(compareNullableNumber(
						a.lastFetched ? new Date(a.lastFetched).getTime() : null,
						b.lastFetched ? new Date(b.lastFetched).getTime() : null,
					), asc);
			}
		});
	});

	let awgSourceRowCount = $derived(awgList.length + visibleSystemList.length + externalList.length);
	let singboxTunnelsSourceRowCount = $derived(singboxTunnelsList.length);
	let singboxSubscriptionsSourceRowCount = $derived(
		subscriptionsActiveCards.length + subscriptionsListRows.length,
	);
	let awgFilteredRowsCount = $derived(
		sortedFilteredAwgList.length + sortedFilteredSystemList.length + sortedFilteredExternalList.length,
	);
	let singboxTunnelsFilteredRowsCount = $derived(sortedFilteredSingboxTunnels.length);
	let singboxSubscriptionsFilteredRowsCount = $derived(
		sortedFilteredSubscriptionsActiveCards.length + sortedFilteredSubscriptionsListRows.length,
	);
	let awgSearchEmpty = $derived(
		effectiveAwgSearchQuery.trim() !== '' &&
			awgFilteredRowsCount === 0,
	);
	let singboxTunnelsSearchEmpty = $derived(
		effectiveSingboxTunnelsSearchQuery.trim() !== '' &&
			singboxTunnelsFilteredRowsCount === 0,
	);
	let singboxSubscriptionsSearchEmpty = $derived(
		effectiveSubscriptionsSearchQuery.trim() !== '' &&
			singboxSubscriptionsFilteredRowsCount === 0,
	);

	// Single source of gated sing-box data for the dashboard: empty unless the
	// sections are visible at this usage level AND sing-box is installed (or
	// its status is still probing). Hidden/unavailable sections must not leak
	// into the merged flat list, section headers or counts.
	let dashboardSingboxTunnels = $derived(
		dashboardSingboxVisible ? sortedFilteredSingboxTunnels : [],
	);
	let dashboardSubscriptionsActive = $derived(
		dashboardSingboxVisible ? sortedFilteredSubscriptionsActiveCards : [],
	);
	let dashboardSubscriptionsStopped = $derived(
		dashboardSingboxVisible ? sortedFilteredSubscriptionsListRows : [],
	);
	let dashboardSubscriptionsCount = $derived(
		dashboardSubscriptionsActive.length + dashboardSubscriptionsStopped.length,
	);
	// Локальный (не персистентный) фильтр по тегу; сбрасывается при выходе из
	// дашборда и в видах, где он не применяется (секции с группировкой «Тип») —
	// иначе чип в тулбаре остаётся, а секции показывают нефильтрованные списки.
	let dashboardTagFilter = $state<string | null>(null);
	$effect(() => {
		if (!dashboardOn || dashboardTypeSections) dashboardTagFilter = null;
	});
	// Merged item base is needed by BOTH card surfaces: the flat layout and the
	// tag-group view of the sections layout.
	let dashboardFlatBase = $derived.by(() => {
		if (!dashboardFlatCardMode && !dashboardGroupByTags) return [];
		return buildFlatDashboardItems({
			awg: sortedFilteredAwgList,
			system: sortedFilteredSystemList,
			external: sortedFilteredExternalList,
			singbox: dashboardSingboxTunnels,
			subscriptionsActive: dashboardSubscriptionsActive,
			subscriptionsStopped: dashboardSubscriptionsStopped,
		});
	});
	// Единственная точка применения тег-фильтра: и сплошной список, и теговые
	// группы потребляют один и тот же отфильтрованный набор.
	let dashboardVisibleItems = $derived(
		dashboardTagFilter !== null
			? filterItemsByTag(dashboardFlatBase, $tunnelDashboardTags, dashboardTagFilter)
			: dashboardFlatBase,
	);
	let dashboardFlatItems = $derived.by(() => {
		if (!dashboardFlatCardMode) return [];
		if ($tunnelDashboardOrderMode === 'manual') {
			return applyManualOrder(dashboardVisibleItems, $tunnelDashboardManualOrder);
		}
		return dashboardVisibleItems;
	});
	// Теговые группы сортируются всегда авто (kind → имя из buildFlatDashboardItems):
	// контрол «Авто/Вручную» здесь скрыт, ручной порядок — фича сплошного списка.
	// Элемент с N тегами рендерится N раз — auto-проверки (nonce) получает только
	// первое вхождение ключа, иначе дублируются API-вызовы и гоняются обновления.
	let dashboardTagGroups = $derived.by(() => {
		if (!dashboardGroupByTags) return [];
		const groups = groupFlatItemsByTag(dashboardVisibleItems, $tunnelDashboardTags);
		const seen = new Set<string>();
		return groups.map((group) => ({
			tag: group.tag,
			items: group.items.map((item) => {
				const autoCheck = !seen.has(item.key);
				seen.add(item.key);
				return { item, autoCheck };
			}),
		}));
	});
	// Admitted sing-box/subscription stores still on their first load — a
	// late-arriving list must not flash the onboarding screen (see
	// dashboardNothingAtAll) и не должен принимать drag-переупорядочивание:
	// коммит по неполному списку затёр бы позиции ещё не приехавших ключей.
	let dashboardSingboxDataPending = $derived(
		dashboardSingboxVisible &&
			(singboxStatusLoading || singboxTunnelsInitialLoading || subscriptionsInitialLoading),
	);
	// D7: ручной порядок в сплошном дашборде — общее pointer-drag ядро sb-router
	// (createReorderDrag). Активен только когда порядок реально редактируемый:
	// без поиска, без фильтра по тегу и когда данные уже доехали.
	let dashboardDndEnabled = $derived(
		dashboardFlatCardMode &&
			$tunnelDashboardOrderMode === 'manual' &&
			dashboardSearchQuery.trim() === '' &&
			dashboardTagFilter === null &&
			!dashboardSingboxDataPending,
	);
	let flatRowEls = $state<Array<HTMLElement | null>>([]);
	let flatGridEl = $state<HTMLElement | null>(null);
	// dashboardFlatItems живой (5s-поллинг), а движок оперирует индексами,
	// зафиксированными на pointerdown — на время drag грид рендерится из
	// снапшота (те же ключи), чтобы мутация посреди drag не закоммитила чужой
	// ключ и не подменила ghost. Обновления полла применяются после отпускания.
	let dragSnapshot = $state<TunnelDashboardFlatItem[] | null>(null);
	let dashboardRenderItems = $derived(dragSnapshot ?? dashboardFlatItems);
	const flatDrag = createReorderDrag({
		getRowElement: (i) => flatRowEls[i] ?? null,
		count: () => dashboardRenderItems.length,
		getPanelEl: () => flatGridEl,
		onCommit: async (from, to) => {
			// Движок отдаёт финальный splice-индекс `to` — переставляем ключи
			// снапшота и вливаем видимую подпоследовательность в сохранённый
			// порядок: позиции скрытых сейчас ключей (sing-box loading /
			// uninstalled / другой usage level) не затираются.
			const before = (dragSnapshot ?? dashboardFlatItems).map((item) => item.key);
			const after = reorder(before, from, to);
			tunnelDashboardManualOrder.set(mergeManualOrder($tunnelDashboardManualOrder, before, after));
			dragSnapshot = null;
		},
	});
	onDestroy(() => flatDrag.destroy());
	function handleGripPointerDown(index: number, event: PointerEvent): void {
		dragSnapshot = dashboardFlatItems;
		flatDrag.handlePointerDown(index, event);
		// Клик без движения не доходит ни до onCommit, ни до active=true —
		// одноразовый слушатель снимает снапшот после того, как движок
		// (зарегистрированный раньше) отработал свой pointerup.
		const clearIfIdle = () => {
			window.removeEventListener('pointerup', clearIfIdle);
			window.removeEventListener('pointercancel', clearIfIdle);
			if (!flatDrag.active && !flatDrag.busy) dragSnapshot = null;
		};
		window.addEventListener('pointerup', clearIfIdle);
		window.addEventListener('pointercancel', clearIfIdle);
	}
	// Страховка: любое завершение drag (включая Escape-отмену) снимает снапшот.
	let dragWasActive = false;
	$effect(() => {
		const active = flatDrag.active;
		if (dragWasActive && !active) dragSnapshot = null;
		dragWasActive = active;
	});
	// Перестановка с клавиатуры на грипе: ArrowUp/ArrowDown двигает элемент на
	// одну позицию — тот же reorder + mergeManualOrder, без pointer.
	function handleGripKeydown(index: number, event: KeyboardEvent): void {
		if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return;
		event.preventDefault();
		if (flatDrag.busy || flatDrag.active) return;
		const before = dashboardFlatItems.map((item) => item.key);
		const to = index + (event.key === 'ArrowUp' ? -1 : 1);
		if (to < 0 || to >= before.length) return;
		tunnelDashboardManualOrder.set(
			mergeManualOrder($tunnelDashboardManualOrder, before, reorder(before, index, to)),
		);
	}
	// Пустой результат поиска ИЛИ тег-фильтра: в карточных видах считаем по
	// видимым элементам, в секциях по типу — по счётчикам блоков (тег-фильтр
	// там не применяется и авто-сбрасывается).
	let dashboardVisibleCount = $derived(
		dashboardFlatCardMode
			? dashboardFlatItems.length
			: dashboardGroupByTags
				? dashboardTagGroups.reduce((n, group) => n + group.items.length, 0)
				: awgFilteredRowsCount + dashboardSingboxTunnels.length + dashboardSubscriptionsCount,
	);
	let dashboardFilterEmpty = $derived(
		dashboardOn &&
			(dashboardSearchQuery.trim() !== '' || dashboardTagFilter !== null) &&
			dashboardVisibleCount === 0,
	);
	let dashboardNothingAtAll = $derived(
		!dashboardSingboxDataPending &&
			awgList.length === 0 &&
			systemList.length === 0 &&
			externalList.length === 0 &&
			dashboardSingboxTunnels.length === 0 &&
			dashboardSubscriptionsCount === 0 &&
			dashboardSearchQuery.trim() === '',
	);
	// Named gates for the three per-kind template blocks. They legitimately
	// differ: AWG hosts the shared onboarding, subscriptions must surface its
	// loading spinner and fetch-error state even in dashboard mode.
	// Per-kind dashboard sections render only in sections layout with grouping
	// by kind ('type'); the tag-group view replaces them wholesale.
	let showAwgBlock = $derived(
		(!dashboardOn && activeTab === 'awg') ||
			(dashboardTypeSections && awgFilteredRowsCount > 0) ||
			(dashboardOn && dashboardNothingAtAll),
	);
	let showSingboxBlock = $derived(
		(!dashboardOn && activeTab === 'singbox') ||
			(dashboardTypeSections && dashboardSingboxTunnels.length > 0),
	);
	let showSubscriptionsBlock = $derived(
		(!dashboardOn && activeTab === 'subscriptions') ||
			(dashboardTypeSections && dashboardSubscriptionsCount > 0) ||
			(dashboardOn &&
				dashboardSingboxVisible &&
				(subscriptionsInitialLoading || subscriptionsFetchFailed)),
	);

	// Единый класс сетки для сплошного и тегового карточных видов — классы
	// плотности не могут разъехаться между двумя разметками.
	let dashboardGridClass = $derived(
		[
			'tunnel-grid',
			effectiveAwgRenderMode === 'list-card' ? 'tunnel-grid--list' : '',
			effectiveAwgRenderMode === 'dense' || effectiveSingboxTunnelsRenderMode === 'dense'
				? 'tunnel-grid--dense'
				: '',
			effectiveAwgRenderMode === 'compact' ? 'tunnel-grid--compact' : '',
		]
			.filter(Boolean)
			.join(' '),
	);

	// Бейдж вида для компактных рядов переупорядочивания и ghost-токена.
	const DASHBOARD_KIND_LABELS: Record<TunnelDashboardFlatItem['kind'], string> = {
		'awg-managed': 'AWG',
		'awg-system': 'system',
		'awg-external': 'external',
		singbox: 'sing-box',
		'sub-active': 'подписка',
		'sub-stopped': 'подписка',
	};

	// D6 (issue #353): комбинированная сводка дашборда по трём видам туннелей.
	// Sing-box и подписки учитываются только когда их данные допущены в дашборд.
	let dashboardSummaryStats = $derived.by(() => {
		if (!dashboardOn) return [];
		const sb = dashboardSingboxVisible ? singboxTunnelListStats : null;
		const subs = dashboardSingboxVisible ? singboxSubscriptionsTrafficStats : null;
		const totalAll = awgSummaryTotal + (sb?.count ?? 0) + (subs?.count ?? 0);
		const totalActive = awgSummaryActive + (sb?.running ?? 0) + (subs?.activeCount ?? 0);
		const kinds = [`AWG ${awgSummaryActive}/${awgSummaryTotal}`];
		if (sb) kinds.push(`Sing-box ${sb.running}/${sb.count}`);
		if (subs) kinds.push(`Подписки ${subs.activeCount}/${subs.count}`);
		const rx = awgSummaryRx + (sb?.down ?? 0) + (subs?.down ?? 0);
		const tx = awgSummaryTx + (sb?.up ?? 0) + (subs?.up ?? 0);
		const leaders = [
			{ bytes: awgTrafficLeader.bytes, name: awgTrafficLeader.name },
			{ bytes: sb?.leaderBytes ?? 0, name: sb?.leaderName ?? '—' },
			{ bytes: subs?.leaderBytes ?? 0, name: subs?.leaderName ?? '—' },
		];
		const leader = leaders.reduce((best, cur) => (cur.bytes > best.bytes ? cur : best));
		return [
			{
				value: `${totalActive}/${totalAll}`,
				label: 'Туннели',
				sub: kinds.join(' · '),
			},
			{
				value: awgSummaryPeak.rate > 0 ? formatBitRate(awgSummaryPeak.rate) : '—',
				label: 'Пиковая скорость',
				// Метрика считается только по AWG-туннелям внутри кросс-видовой
				// сводки — область видимости заявлена явно.
				sub: awgSummaryPeak.rate > 0 ? `AWG · ${awgSummaryPeak.name}` : '—',
			},
			{
				value: formatBytes(rx + tx),
				label: 'Трафик всего',
				sub: `↓ ${formatBytes(rx)} · ↑ ${formatBytes(tx)}`,
			},
			{
				value: leader.bytes > 0 ? leader.name : '—',
				label: 'Лидер трафика',
				sub: leader.bytes > 0 ? formatBytes(leader.bytes) : '—',
			},
		];
	});

	// Live-контекст AWG-вкладки: геттеры замыкают $state/$derived страницы,
	// сеттеры мутируют её состояние (см. awgTabContext.ts).
	const awgTabCtx: AwgTabContext = {
		get awgList() { return awgList; },
		get systemList() { return systemList; },
		get visibleSystemList() { return visibleSystemList; },
		get externalList() { return externalList; },
		get sortedFilteredAwgList() { return sortedFilteredAwgList; },
		get sortedFilteredSystemList() { return sortedFilteredSystemList; },
		get sortedFilteredExternalList() { return sortedFilteredExternalList; },
		get awgConnectivityMap() { return awgConnectivityMap; },
		get statusLine() { return statusLine; },
		get sysInfo() { return sysInfo; },
		get awgSummaryActive() { return awgSummaryActive; },
		get awgSummaryPeak() { return awgSummaryPeak; },
		get awgSummaryRx() { return awgSummaryRx; },
		get awgSummaryTx() { return awgSummaryTx; },
		get awgSummaryTotal() { return awgSummaryTotal; },
		get awgTrafficLeader() { return awgTrafficLeader; },
		get nativewgHint() { return nativewgHint; },
		get dashboardOn() { return dashboardOn; },
		get dashboardSectionsLayout() { return dashboardSectionsLayout; },
		get dashboardNothingAtAll() { return dashboardNothingAtAll; },
		get awgSearchEmpty() { return awgSearchEmpty; },
		get awgSourceRowCount() { return awgSourceRowCount; },
		get showAwgViewModeSwitch() { return showAwgViewModeSwitch; },
		get effectiveAwgCardViewMode() { return effectiveAwgCardViewMode; },
		get effectiveAwgEffectiveViewMode() { return effectiveAwgEffectiveViewMode; },
		get effectiveAwgRenderMode() { return effectiveAwgRenderMode; },
		get awgAutoConnectivityNonce() { return awgAutoConnectivityNonce; },
		get deleteLoading() { return deleteLoading; },
		get dragOver() { return dragOver; },
		get exporting() { return exporting; },
		get importing() { return importing; },
		get pingChecking() { return pingChecking; },
		get toggleLoading() { return toggleLoading; },
		get adoptDialogOpen() { return adoptDialogOpen; },
		set adoptDialogOpen(v) { adoptDialogOpen = v; },
		get adoptingInterface() { return adoptingInterface; },
		set adoptingInterface(v) { adoptingInterface = v; },
		get awgListSearchQuery() { return awgListSearchQuery; },
		set awgListSearchQuery(v) { awgListSearchQuery = v; },
		get awgViewMode() { return awgViewMode; },
		set awgViewMode(v) { awgViewMode = v; },
		get selectedBackend() { return selectedBackend; },
		set selectedBackend(v) { selectedBackend = v; },
		get fileInput() { return fileInput; },
		set fileInput(v) { fileInput = v; },
		endpointHost, endpointPort, endpointVisible, toggleEndpointVisible, externalStatusLabel, externalStatusVariant, systemStatusLabel, systemStatusVariant, isManagedTunnelOn, managedRouteMeta, showManagedPing, latestRate, sparklineSeries, handleAdoptClick, handleAwgSortChange, handleDragLeave, handleDragOver, handleDrop, handleFileSelect, openAwgDiagnostics, openConnectivitySettings, openDetail, requestDelete, markAsServer, handleToggleOnOff, checkPing, handleExportAll,
	};

	// Live-контекст flat-дашборда (см. dashboardFlatContext.ts).
	const dashboardFlatCtx: DashboardFlatContext = {
		get DASHBOARD_KIND_LABELS() { return DASHBOARD_KIND_LABELS; },
		get dashboardOn() { return dashboardOn; },
		get dashboardDndEnabled() { return dashboardDndEnabled; },
		get dashboardFilterEmpty() { return dashboardFilterEmpty; },
		get dashboardFlatCardMode() { return dashboardFlatCardMode; },
		get dashboardFlatLayout() { return dashboardFlatLayout; },
		get dashboardGridClass() { return dashboardGridClass; },
		get dashboardGroupByTags() { return dashboardGroupByTags; },
		get dashboardRenderItems() { return dashboardRenderItems; },
		get dashboardSummaryStats() { return dashboardSummaryStats; },
		get dashboardTagGroups() { return dashboardTagGroups; },
		get effectiveAwgCardViewMode() { return effectiveAwgCardViewMode; },
		get effectiveAwgRenderMode() { return effectiveAwgRenderMode; },
		get effectiveSingboxTunnelsEffectiveLayout() { return effectiveSingboxTunnelsEffectiveLayout; },
		get effectiveSingboxTunnelsRenderMode() { return effectiveSingboxTunnelsRenderMode; },
		get effectiveSingboxSubscriptionsEffectiveLayout() { return effectiveSingboxSubscriptionsEffectiveLayout; },
		get effectiveSingboxSubscriptionsRenderMode() { return effectiveSingboxSubscriptionsRenderMode; },
		get showSingboxListOption() { return showSingboxListOption; },
		get showSingboxSections() { return showSingboxSections; },
		get loading() { return loading; },
		get exporting() { return exporting; },
		get adoptingInterface() { return adoptingInterface; },
		get awgAutoConnectivityNonce() { return awgAutoConnectivityNonce; },
		get singboxAutoDelayCheckNonce() { return singboxAutoDelayCheckNonce; },
		get deleteLoading() { return deleteLoading; },
		get toggleLoading() { return toggleLoading; },
		get liveActives() { return liveActives; },
		get flatDrag() { return flatDrag; },
		get flatRowEls() { return flatRowEls; },
		get adoptDialogOpen() { return adoptDialogOpen; },
		set adoptDialogOpen(v) { adoptDialogOpen = v; },
		get adoptError() { return adoptError; },
		set adoptError(v) { adoptError = v; },
		get adoptLoading() { return adoptLoading; },
		set adoptLoading(v) { adoptLoading = v; },
		get dashboardSearchQuery() { return dashboardSearchQuery; },
		set dashboardSearchQuery(v) { dashboardSearchQuery = v; },
		get dashboardTagFilter() { return dashboardTagFilter; },
		set dashboardTagFilter(v) { dashboardTagFilter = v; },
		get flatGridEl() { return flatGridEl; },
		set flatGridEl(v) { flatGridEl = v; },
		handleAdopt, handleAdoptClick, handleExportAll, handleGripKeydown, handleGripPointerDown, handleToggleOnOff, markAsServer, openAwgDiagnostics, openDetail, openSingboxDetail, openWizard, requestDelete, requestSubscriptionDelete,
	};
</script>

{#snippet createIcon()}
	<CreateIcon />
{/snippet}

<svelte:head>
	<title>Туннели - AWG Manager</title>
</svelte:head>

<PageContainer width="full">
	<PageHeader title="Туннели" />
	<WelcomeBanner />
	{#if loading}
		<div class="py-12">
			<LoadingSpinner size="lg" message="Загрузка туннелей..." />
		</div>
	{:else if tunnelSnap.status === 'error' && !tunnelSnap.data}
		<EmptyState
			title="Ошибка загрузки"
			description={tunnelSnap.error ?? 'Не удалось получить список туннелей'}
		/>
	{:else}
		{#if dashboardOn}
			<DashboardFlatSection ctx={dashboardFlatCtx} />
		{:else}
			<Tabs
				tabs={tunnelTabs}
				active={activeTab}
				onchange={(id) => (activeTab = id as TunnelTab)}
				urlParam="tab"
				defaultTab="awg"
			/>
		{/if}

		{#if dashboardTypeSections && awgFilteredRowsCount > 0}
			<TunnelSectionHeader
				title="Amnezia WireGuard"
				count={awgFilteredRowsCount}
				countLabel={pluralForm(awgFilteredRowsCount, TUNNEL_WORDS)}
			/>
		{/if}
		{#if showAwgBlock}
			<AwgTunnelsTabSection ctx={awgTabCtx} />
		{/if}

		{#if dashboardTypeSections && dashboardSingboxTunnels.length > 0}
			<TunnelSectionHeader
				title="Sing-box туннели"
				count={dashboardSingboxTunnels.length}
				countLabel={pluralForm(dashboardSingboxTunnels.length, TUNNEL_WORDS)}
			/>
		{/if}
		{#if showSingboxBlock}
			<SingboxTunnelsTabSection
				{dashboardOn}
				{dashboardSingboxTunnels}
				{singboxTunnelsList}
				{sortedFilteredSingboxTunnels}
				{singboxTunnelListStats}
				{singboxTunnelsSourceRowCount}
				{singboxTunnelsSearchEmpty}
				{singboxAutoDelayCheckNonce}
				{showSingboxGridListToggle}
				{effectiveSingboxTunnelsEffectiveLayout}
				{effectiveSingboxTunnelsRenderMode}
				{subscriptionsActiveCards}
				bind:singboxTunnelsSearchQuery
				bind:singboxTunnelsLayoutMode
				{handleSingboxTunnelSortChange}
				{openSingboxDetail}
				{openWizard}
			/>
		{/if}

		{#if dashboardTypeSections && dashboardSubscriptionsCount > 0}
			<TunnelSectionHeader
				title="Sing-box подписки"
				count={dashboardSubscriptionsCount}
				countLabel={pluralForm(dashboardSubscriptionsCount, SUBSCRIPTION_WORDS)}
			/>
		{/if}
		{#if showSubscriptionsBlock}
			<SubscriptionsTabSection
				{loading}
				{dashboardOn}
				{dashboardSectionsLayout}
				{subscriptionsInitialLoading}
				{subscriptionsFetchFailed}
				subscriptionsError={subscriptionsState.error ?? null}
				{subscriptionsList}
				{subscriptionsActiveCards}
				{sortedFilteredSubscriptionsActiveCards}
				{sortedFilteredSubscriptionsListRows}
				{singboxSubscriptionsTrafficStats}
				{singboxSubscriptionsSourceRowCount}
				{singboxSubscriptionsSearchEmpty}
				{singboxInstalled}
				{singboxStatusLoading}
				{singboxAutoDelayCheckNonce}
				{showSingboxGridListToggle}
				{effectiveSingboxSubscriptionsEffectiveLayout}
				{effectiveSingboxSubscriptionsRenderMode}
				{liveActives}
				bind:singboxSubscriptionsSearchQuery
				bind:singboxSubscriptionsLayoutMode
				{handleSubscriptionSortChange}
				{openSingboxDetail}
				{openWizard}
				{requestSubscriptionDelete}
			/>
		{/if}
	{/if}
</PageContainer>

{#if flatDrag.ghostVisible && flatDrag.ghostFromIndex !== null && dashboardRenderItems[flatDrag.ghostFromIndex]}
	{@const ghostItem = dashboardRenderItems[flatDrag.ghostFromIndex]}
	<!-- Ghost — компактный токен, а не полная карточка: полный рендер монтировал
	     графики и стрелял loadHistory/subscribeTraffic на каждый захват. -->
	<div
		class="dashboard-drag-ghost"
		style={`top:${flatDrag.ghostTop}px;left:${flatDrag.ghostLeft}px;width:${flatDrag.ghostWidth}px;`}
	>
		<div class="drag-ghost-token">
			<span class="dashboard-kind-badge">{DASHBOARD_KIND_LABELS[ghostItem.kind]}</span>
			<span class="dashboard-reorder-name">{ghostItem.name}</span>
		</div>
	</div>
{/if}

<AdoptTunnelDialog
	interfaceName={adoptingInterface}
	bind:open={adoptDialogOpen}
	bind:error={adoptError}
	bind:loading={adoptLoading}
	onclose={() => adoptDialogOpen = false}
	onadopt={handleAdopt}
/>

{#if deleteConfirmId}
	{@const tunnelName = awgList.find(t => t.id === deleteConfirmId)?.name ?? deleteConfirmId}
	<Modal
		open={true}
		title="Удалить туннель"
		size="sm"
		onclose={() => deleteConfirmId = null}
	>
		<p class="confirm-text">Удалить туннель <strong>{tunnelName}</strong>?</p>
		{#snippet actions()}
			<Button variant="secondary" size="md" onclick={() => deleteConfirmId = null}>Отмена</Button>
			<Button variant="danger" size="md" onclick={() => handleDelete(deleteConfirmId!)}>Удалить</Button>
		{/snippet}
	</Modal>
{/if}

<TunnelReferencedModal
	open={referencedDetails !== null}
	details={referencedDetails}
	tunnelName={referencedTunnelName}
	onclose={() => { referencedDetails = null; referencedTunnelName = ''; }}
/>

<AddTunnelWizard bind:open={createModalOpen} preselect={wizardPreselect} />

<Modal
	open={pendingSubscriptionDelete !== null}
	title="Удалить подписку?"
	size="md"
	onclose={() => {
		if (deletingSubscription) return;
		pendingSubscriptionDelete = null;
	}}
>
	<p>
		Подписка <strong>{pendingSubscriptionLabel}</strong> будет удалена
		вместе с её sing-box outbound'ами и NDMS Proxy-интерфейсом.
	</p>
	{#snippet actions()}
		<Button
			variant="ghost"
			disabled={deletingSubscription}
			onclick={() => (pendingSubscriptionDelete = null)}
		>
			Отмена
		</Button>
		<Button
			variant="danger"
			disabled={deletingSubscription}
			loading={deletingSubscription}
			onclick={confirmSubscriptionDelete}
		>
			{deletingSubscription ? 'Удаляем...' : 'Удалить'}
		</Button>
	{/snippet}
</Modal>

{#if detailId}
	{@const managed = awgList.find((x) => x.id === detailId)}
	{@const sys = systemList.find((x) => x.id === detailId)}
	{#if managed || sys}
		<TrafficChartModal
			open={true}
			tunnelId={detailId}
			tunnelName={managed?.name ?? sys?.description ?? detailId}
			ifaceName={managed?.interfaceName ?? sys?.interfaceName ?? ''}
			onclose={closeDetail}
		/>
	{/if}
{/if}

{#if singboxDetailTag}
	{@const sb = singboxTunnelsList.find((x) => x.tag === singboxDetailTag)}
	{@const subActiveCard = subscriptionsActiveCards.find((c) => c.activeMember.tag === singboxDetailTag)}
	{@const subListRow = subscriptionsListRows.find(
		(s) => resolveSubscriptionMemberTag(s, liveActives[s.id] || null) === singboxDetailTag,
	)}
	{@const detailName =
		subActiveCard?.subscription.label
		?? subListRow?.label
		?? sb?.tag
		?? singboxDetailTag}
	{@const detailIface =
		subActiveCard
			? (subActiveCard.subscription.proxyIndex >= 0 ? `Proxy${subActiveCard.subscription.proxyIndex}` : '')
			: (subListRow
				? (subListRow.proxyIndex >= 0 ? `Proxy${subListRow.proxyIndex}` : '')
				: (sb?.proxyInterface ?? ''))}
	<TrafficChartModal
		open={true}
		tunnelId={singboxDetailTag}
		tunnelName={detailName}
		ifaceName={detailIface}
		onclose={closeSingboxDetail}
	/>
{/if}

{#if awgDiagnosticsTarget}
	<TunnelDiagnosticsModal
		open={true}
		kind={awgDiagnosticsTarget.kind}
		targetId={awgDiagnosticsTarget.id}
		displayName={awgDiagnosticsTarget.name}
		subjectLabel="туннель"
		onclose={closeAwgDiagnostics}
	/>
{/if}

{#if connectivitySettingsTunnel}
	<ConnectivitySettingsModal
		bind:open={connectivitySettingsOpen}
		tunnelId={connectivitySettingsTunnel.id}
		tunnelAddress={connectivitySettingsTunnel.address}
		onclose={closeConnectivitySettings}
		onSaved={closeConnectivitySettings}
	/>
{/if}

{#snippet exportIcon()}
	<Download size={14} strokeWidth={2} aria-hidden="true" />
{/snippet}

{#if showUnsupportedBlock}
	<div class="unsupported-overlay">
		<div class="unsupported-card">
			<div class="unsupported-icon">
				<TriangleAlert size={48} strokeWidth={1.5} aria-hidden="true" />
			</div>
			<h2 class="unsupported-title">Модуль ядра недоступен</h2>
			<p class="unsupported-text">
				Модель роутера <strong>{sysInfo?.kernelModuleModel || '(неизвестна)'}</strong> не имеет скомпилированный модуль ядра в настоящий момент.
			</p>
			<div class="unsupported-actions">
				<a href="https://t.me/awgmanager" target="_blank" rel="noopener" class="unsupported-link unsupported-link-primary">
					Написать в @awgmanager
				</a>
				<a href="https://gitlab.com/AmneziaVPN/amneziawg/amneziawg-linux-kernel-module" target="_blank" rel="noopener" class="unsupported-link">
					Установить вручную
				</a>
			</div>
		</div>
	</div>
{/if}

<style>

	/* Ghost-токен drag-переупорядочивания: бейдж вида + имя. */
	.drag-ghost-token {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		box-sizing: border-box;
		height: var(--reorder-row-height, 40px);
		padding: 0 0.75rem;
		min-width: 0;
		background: var(--color-bg-secondary);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-sm);
	}

	.dashboard-kind-badge {
		flex: 0 0 auto;
		font-family: var(--font-mono);
		font-size: 0.6875rem;
		line-height: 1.3;
		color: var(--color-text-muted);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-sm);
		padding: 1px 6px;
		white-space: nowrap;
	}

	.dashboard-reorder-name {
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		min-width: 0;
	}

	.dashboard-drag-ghost {
		position: fixed;
		z-index: 10000;
		pointer-events: none;
		opacity: 0.96;
		filter: drop-shadow(0 14px 24px rgba(0, 0, 0, 0.35));
	}

	:global(body.reorder-dragging) {
		user-select: none;
		cursor: grabbing;
	}

	/* "Kernel module unavailable" full-screen overlay — page-specific */
	.unsupported-overlay {
		position: fixed;
		inset: 0;
		z-index: var(--z-full-overlay);
		background: rgba(0, 0, 0, 0.85);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1rem;
	}

	.unsupported-card {
		background: var(--color-bg-primary);
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		padding: 2rem;
		max-width: 420px;
		width: 100%;
		text-align: center;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 1rem;
	}

	.unsupported-icon {
		color: var(--color-warning);
	}

	.unsupported-title {
		font-size: 1.25rem;
		font-weight: 600;
		margin: 0;
	}

	.unsupported-text {
		font-size: 0.875rem;
		color: var(--color-text-secondary);
		line-height: 1.6;
		margin: 0;
	}

	.unsupported-text strong {
		color: var(--color-text-primary);
	}

	.unsupported-actions {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		width: 100%;
		margin-top: 0.5rem;
	}

	.unsupported-link {
		display: block;
		padding: 0.625rem 1rem;
		border-radius: var(--radius-sm);
		font-size: 0.875rem;
		font-weight: 500;
		text-decoration: none;
		text-align: center;
		transition: opacity var(--t-fast) ease;
		border: 1px solid var(--color-border);
		color: var(--color-text-secondary);
		background: var(--color-bg-secondary);
	}

	.unsupported-link:hover {
		opacity: 0.85;
	}

	.unsupported-link-primary {
		background: var(--color-accent);
		color: #fff;
		border-color: var(--color-accent);
	}

	@media (max-width: 760px) {
	}
</style>
