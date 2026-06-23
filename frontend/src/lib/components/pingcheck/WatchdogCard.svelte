<!-- frontend/src/lib/components/pingcheck/WatchdogCard.svelte -->
<script lang="ts">
	import { Card, StatusDot, VersionBadge, Badge, Button, type StatusDotVariant, type BadgeVariant } from '$lib/components/ui';
	import { TunnelDelaySparkBars } from '$lib/components/tunnels';
	import { Settings, Power, Info, Check, X, RotateCcw } from 'lucide-svelte';
	import { formatTime } from '$lib/utils/format';
	import type { CardStats } from '$lib/utils/pingStats';

	interface Props {
		name: string;
		backend: 'kernel' | 'nativewg';
		awgVersion?: string;
		statusKind: 'alive' | 'recovering' | 'disabled' | 'stopped';
		hasPingcheck: boolean;
		isWatchdog: boolean;
		configLine: string;
		stats: CardStats | null;
		onConfigure: () => void;
		onCheckNow: () => void;
	}

	let { name, backend, awgVersion, statusKind, hasPingcheck, isWatchdog, configLine, stats, onConfigure, onCheckNow }: Props =
		$props();

	const STATUS: Record<Props['statusKind'], { dot: StatusDotVariant; pulse: boolean; label: string; badge: BadgeVariant }> = {
		alive: { dot: 'success', pulse: false, label: 'активен', badge: 'success' },
		recovering: { dot: 'warning', pulse: true, label: 'восстановление', badge: 'warning' },
		disabled: { dot: 'muted', pulse: false, label: 'выключен', badge: 'muted' },
		stopped: { dot: 'muted', pulse: false, label: 'остановлен', badge: 'muted' },
	};
	const st = $derived(STATUS[statusKind]);
	const fmt = (v: number | null) => (v === null ? '—' : `${v}ms`);
</script>

<Card>
	<div class="wd-head">
		<StatusDot variant={st.dot} pulse={st.pulse} size="sm" />
		<span class="wd-name">{name}</span>
		<VersionBadge kind="backend" value={backend} />
		{#if awgVersion}<VersionBadge kind="awg" value={awgVersion} />{/if}
		{#if isWatchdog}<Badge variant="accent" mono>watchdog</Badge>{/if}
		<span class="wd-spacer"></span>
		<Badge variant={st.badge}>{st.label}</Badge>
	</div>

	{#if hasPingcheck && stats}
		<div class="wd-config">{configLine}</div>

		<div class="wd-stats">
			<div class="wd-stat"><span class="v">{fmt(stats.avgMs)}</span><span class="k">avg</span></div>
			<div class="wd-stat"><span class="v">{stats.failsLabel}</span><span class="k">сбои</span></div>
			<div class="wd-stat"><span class="v"><RotateCcw size={13} /> {stats.restarts}</span><span class="k">рестарты</span></div>
			<div class="wd-stat"><span class="v" class:loss={stats.lossPct > 0}>{stats.lossPct}%</span><span class="k">loss</span></div>
		</div>

		<div class="wd-checks">
			<div class="wd-checks-head">
				<span>Последние проверки</span>
				<span class="wd-minmax" title="Окно ~2ч / последние {stats.history.length} проверок">
					min {fmt(stats.minMs)} · max {fmt(stats.maxMs)}
				</span>
			</div>
			<div class="wd-bars">
				<TunnelDelaySparkBars history={stats.history} state="ok" maxBars={12} colorPerBar title="Проверить сейчас" onclick={onCheckNow} />
			</div>
			<div class="wd-log">
				{#each stats.recent as e (e.timestamp + e.tunnelId)}
					<div class="wd-log-row">
						<span class="ts">{formatTime(e.timestamp)}</span>
						<span class="ico" class:ok={e.success} class:bad={!e.success}>
							{#if e.success}<Check size={13} />{:else}<X size={13} />{/if}
						</span>
						<span class="lat">{e.success ? `${e.latency}ms` : '—'}</span>
						<span class="note">{e.error}</span>
					</div>
				{/each}
			</div>
		</div>

		<div class="wd-foot">
			<Button variant="ghost" onclick={onConfigure}>
				{#snippet iconBefore()}<Settings size={14} />{/snippet}
				Настроить
			</Button>
		</div>
	{:else}
		<div class="wd-note">
			<span class="wd-note-text"><Info size={14} /> Pingcheck/watchdog не настроен для этого туннеля.</span>
			<div class="wd-foot">
				<Button variant="ghost" onclick={onConfigure}>
					{#snippet iconBefore()}<Power size={14} />{/snippet}
					Включить
				</Button>
			</div>
		</div>
	{/if}
</Card>

<style>
	.wd-head { display: flex; align-items: center; gap: 8px; }
	.wd-name { font-weight: 600; font-size: 14px; color: var(--color-text); }
	.wd-spacer { flex: 1; }
	.wd-config { font-family: var(--font-mono); font-size: 11px; color: var(--color-text-muted); margin-top: 8px; }
	.wd-stats { display: grid; grid-template-columns: repeat(4, 1fr); gap: 1px; margin-top: 10px; }
	.wd-stat { display: flex; flex-direction: column; align-items: center; gap: 2px; padding: 8px 4px; }
	.wd-stat .v { display: inline-flex; align-items: center; gap: 4px; font-family: var(--font-mono); font-size: 15px; font-weight: 600; color: var(--color-text); }
	.wd-stat .v.loss { color: var(--color-error); }
	.wd-stat .k { font-size: 10px; text-transform: uppercase; letter-spacing: 0.04em; color: var(--color-text-muted); }
	.wd-checks { margin-top: 10px; }
	.wd-checks-head { display: flex; align-items: baseline; justify-content: space-between; margin-bottom: 6px; }
	.wd-checks-head span:first-child { font-size: 10px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: var(--color-text-muted); }
	.wd-minmax { font-family: var(--font-mono); font-size: 11px; color: var(--color-text-muted); }
	.wd-bars { height: 36px; display: flex; align-items: flex-end; }
	.wd-bars :global(.tunnel-delay-spark) { height: 36px; flex: 1; }
	.wd-log { display: flex; flex-direction: column; margin-top: 10px; }
	.wd-log-row { display: flex; align-items: center; gap: 10px; padding: 3px 0; font-family: var(--font-mono); font-size: 11px; }
	.wd-log-row .ts { color: var(--color-text-muted); min-width: 56px; }
	.wd-log-row .ico { display: inline-flex; align-items: center; }
	.wd-log-row .ico.ok { color: var(--color-success); }
	.wd-log-row .ico.bad { color: var(--color-error); }
	.wd-log-row .lat { color: var(--color-text); min-width: 52px; }
	.wd-log-row .note { color: var(--color-text-muted); font-style: italic; }
	.wd-foot { display: flex; justify-content: flex-end; margin-top: 10px; }
	.wd-note { display: flex; flex-direction: column; gap: 12px; color: var(--color-text-muted); font-size: 12px; margin-top: 8px; }
	.wd-note-text { display: inline-flex; align-items: center; gap: 6px; }
</style>
