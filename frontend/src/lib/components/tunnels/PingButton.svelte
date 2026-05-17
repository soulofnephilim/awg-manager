<script lang="ts">
	type ConnState = 'idle' | 'connected' | 'disconnected' | 'checking';

	interface Props {
		connectivity: ConnState;
		latencyMs?: number | null;
		recovering?: boolean;
		checking?: boolean;
		size?: 'sm' | 'md';
		onclick?: () => void;
	}

	let {
		connectivity,
		latencyMs = null,
		recovering = false,
		checking = false,
		size = 'md',
		onclick,
	}: Props = $props();

	function tier(ms: number): string {
		if (ms < 80) return 'good';
		if (ms < 130) return 'warn';
		if (ms < 200) return 'high';
		return 'bad';
	}

	let label = $derived.by(() => {
		if (checking || connectivity === 'checking') return '...';
		if (connectivity === 'connected' && latencyMs !== null) return `${latencyMs}ms`;
		if (connectivity === 'connected') return 'OK';
		if (connectivity === 'disconnected') return '—';
		return '...';
	});

	let tierClass = $derived.by(() => {
		if (recovering) return '';
		if (checking || connectivity === 'checking') return '';
		if (connectivity === 'disconnected') return 'tier-bad';
		if (connectivity === 'connected' && latencyMs !== null) return `tier-${tier(latencyMs)}`;
		return '';
	});

	let isSpinning = $derived(checking || connectivity === 'checking');
</script>

<button
	type="button"
	class="ping-btn {size} {tierClass}"
	class:spinning={isSpinning}
	{onclick}
	disabled={isSpinning}
	title={connectivity === 'disconnected'
		? 'Нет связи. Нажать для проверки'
		: 'Проверить связь'}
>
	{label}
	<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
		<path d="M23 4v6h-6M1 20v-6h6"/>
		<path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
	</svg>
</button>

<style>
	.ping-btn {
		background: none;
		border: 1px solid transparent;
		color: var(--color-text-muted);
		font-family: var(--font-mono, monospace);
		font-size: 12px;
		font-weight: 500;
		padding: 2px 6px;
		border-radius: var(--radius-sm, 4px);
		cursor: pointer;
		display: inline-flex;
		width: auto;
		max-width: 100%;
		align-items: center;
		gap: 4px;
		flex: 0 0 auto;
		font-variant-numeric: tabular-nums;
		transition: background 0.15s ease, border-color 0.15s ease, color 0.4s ease;
		white-space: nowrap;
	}

	.ping-btn:hover:not(:disabled) {
		background: var(--color-bg-hover, rgba(255,255,255,0.05));
		border-color: var(--color-border);
	}

	.ping-btn:disabled {
		cursor: default;
	}

	/* size variant */
	.ping-btn.sm {
		font-size: 9px;
		padding: 1px 4px;
		gap: 3px;
	}

	/* latency tier colours */
	.ping-btn.tier-good { color: var(--color-success); }
	.ping-btn.tier-warn  { color: var(--color-warning); }
	.ping-btn.tier-high  { color: var(--color-broken); }
	.ping-btn.tier-bad   { color: var(--color-error); }

	/* refresh icon */
	.ping-btn svg {
		flex-shrink: 0;
		width: 11px;
		height: 11px;
		opacity: 0.45;
		transition: opacity 0.15s ease;
	}

	.ping-btn.sm svg {
		width: 9px;
		height: 9px;
	}

	.ping-btn:hover:not(:disabled) svg {
		opacity: 1;
	}

	.ping-btn.spinning svg {
		opacity: 0.7;
		animation: ping-spin 0.9s linear infinite;
	}

	@keyframes ping-spin {
		to { transform: rotate(360deg); }
	}
</style>
