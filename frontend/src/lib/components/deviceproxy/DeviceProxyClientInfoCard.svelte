<script lang="ts">
	import { notifications } from '$lib/stores/notifications';
	import { Button } from '$lib/components/ui';
	import { copyToClipboard } from '$lib/utils/clipboard';
	import type { DeviceProxyConfig, DeviceProxyInstanceIPCheckResult } from '$lib/types';

	interface Props {
		config: DeviceProxyConfig;
		resolvedListenIP: string;
		bridgeLabel: string;
		externalIP: DeviceProxyInstanceIPCheckResult | null;
		externalIPError: string;
		externalIPLoading: boolean;
		externalIPCheckedAt: number | null;
		onRefreshExternalIP: () => Promise<void>;
		onOpenSettings: () => void;
	}

	let {
		config,
		resolvedListenIP,
		bridgeLabel,
		externalIP,
		externalIPError,
		externalIPLoading,
		externalIPCheckedAt,
		onRefreshExternalIP,
		onOpenSettings
	}: Props = $props();

	let revealExternalIP = $state(false);

	const socksHostPort = $derived(`${resolvedListenIP}:${config.port}`);

	const socksUrl = $derived.by(() => {
		const auth = config.auth.enabled
			? `${encodeURIComponent(config.auth.username)}:${encodeURIComponent(config.auth.password)}@`
			: '';
		return `socks5://${auth}${socksHostPort}`;
	});

	const listenLabel = $derived(
		config.listenAll ? 'Все интерфейсы' : (bridgeLabel || config.listenInterface),
	);

	async function copyUrl() {
		if (await copyToClipboard(socksUrl)) {
			notifications.success('Скопировано');
		} else {
			notifications.error('Не удалось скопировать');
		}
	}

	function toggleExternalIPReveal(event: MouseEvent) {
		event.preventDefault();
		event.stopPropagation();
		revealExternalIP = !revealExternalIP;
	}

	function maskIP(ip: string): string {
		const trimmed = ip.trim();
		if (!trimmed) return '';
		if (trimmed.includes(':')) {
			const parts = trimmed.split(':').filter((x) => x.length > 0);
			if (parts.length <= 2) return '••••:••••';
			return `${parts[0]}:••••:••••:${parts[parts.length - 1]}`;
		}
		const parts = trimmed.split('.');
		if (parts.length !== 4) return '•••';
		return `${parts[0]}.${parts[1]}.•••.•••`;
	}

	function maskHostPort(value: string): string {
		const trimmed = value.trim();
		if (!trimmed) return '';

		const ipv6 = trimmed.match(/^(\[[0-9a-fA-F:]+\])(:\d+)$/);
		if (ipv6) {
			return `[••••]${ipv6[2]}`;
		}

		const hostPort = trimmed.match(/^(.+?)(:\d+)$/);
		if (!hostPort) return '•••';

		return `${maskIP(hostPort[1])}${hostPort[2]}`;
	}

	function formatCheckedAt(ts: number | null): string {
		if (!ts) return '';
		const d = new Date(ts);
		return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
	}

	function sourceLabel(service: string): string {
		if (!service) return '';
		try {
			return new URL(service).hostname;
		} catch {
			return service;
		}
	}
</script>

<section class="card">
	<h2 class="section-title">Подключение клиента</h2>

	<div class="info-row">
		<span class="info-key">СЛУШАТЬ</span>
		<span class="info-val">{listenLabel}</span>
	</div>
	<div class="info-row">
		<span class="info-key">SOCKS5</span>
		<div class="ip-check-wrap">
			<span class="info-val">
				{revealExternalIP ? socksHostPort : maskHostPort(socksHostPort)}
			</span>
			<button
				type="button"
				class="detail-eye"
				aria-label={revealExternalIP ? 'Скрыть SOCKS5 адрес' : 'Показать SOCKS5 адрес'}
				title={revealExternalIP ? 'Скрыть SOCKS5 адрес' : 'Показать SOCKS5 адрес'}
				onclick={toggleExternalIPReveal}
			>
				{#if revealExternalIP}
					<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<path d="M17.94 17.94A10.94 10.94 0 0 1 12 20C7 20 2.73 16.89 1 12a19.2 19.2 0 0 1 5.06-6.94"/>
						<path d="M10.58 10.58A2 2 0 0 0 12 14a2 2 0 0 0 1.42-.58"/>
						<path d="M9.9 4.24A10.75 10.75 0 0 1 12 4c5 0 9.27 3.11 11 8a19.2 19.2 0 0 1-2.22 3.59"/>
						<line x1="1" y1="1" x2="23" y2="23"/>
					</svg>
				{:else}
					<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
						<circle cx="12" cy="12" r="3"/>
					</svg>
				{/if}
			</button>
		</div>
	</div>
	<div class="info-row">
		<div class="info-key-wrap">
			<span class="info-key">Внешний IP</span>
			{#if externalIPCheckedAt}
				<span class="checked-at">
					обновлено {formatCheckedAt(externalIPCheckedAt)}
					{#if externalIP?.service}
						· {sourceLabel(externalIP.service)}
					{/if}
				</span>
			{/if}
		</div>
		<div class="ip-check-wrap">
			{#if externalIP}
				<span class="info-val">
					{revealExternalIP ? externalIP.proxyIp : maskIP(externalIP.proxyIp)}
				</span>
				<button
					type="button"
					class="detail-eye"
					aria-label={revealExternalIP ? 'Скрыть внешний IP' : 'Показать внешний IP'}
					title={revealExternalIP ? 'Скрыть внешний IP' : 'Показать внешний IP'}
					onclick={toggleExternalIPReveal}
				>
					{#if revealExternalIP}
						<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
							<path d="M17.94 17.94A10.94 10.94 0 0 1 12 20C7 20 2.73 16.89 1 12a19.2 19.2 0 0 1 5.06-6.94"/>
							<path d="M10.58 10.58A2 2 0 0 0 12 14a2 2 0 0 0 1.42-.58"/>
							<path d="M9.9 4.24A10.75 10.75 0 0 1 12 4c5 0 9.27 3.11 11 8a19.2 19.2 0 0 1-2.22 3.59"/>
							<line x1="1" y1="1" x2="23" y2="23"/>
						</svg>
					{:else}
						<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
							<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
							<circle cx="12" cy="12" r="3"/>
						</svg>
					{/if}
				</button>
			{:else if externalIPError}
				<span class="info-val info-error" title={externalIPError}>{externalIPError}</span>
			{:else}
				<span class="info-val info-pending">Не проверен</span>
			{/if}
			<Button variant="ghost" size="sm" loading={externalIPLoading} onclick={onRefreshExternalIP}>
				Проверить
			</Button>
		</div>
	</div>

	<div class="actions">
		<Button variant="ghost" size="sm" onclick={copyUrl}>Копировать URL</Button>
		<Button variant="ghost" size="sm" onclick={onOpenSettings}>Настройки</Button>
	</div>
</section>

<style>
	.section-title {
		font-size: 1rem;
		font-weight: 600;
		margin: 0 0 0.75rem 0;
	}

	.info-row {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		padding: 0.5rem 0;
		border-bottom: 1px solid var(--color-border);
		gap: 1rem;
	}

	.info-row:last-of-type {
		border-bottom: none;
	}

	.ip-check-wrap {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
	}

	.info-key-wrap {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.checked-at {
		font-size: 0.6875rem;
		color: var(--color-text-muted);
		text-transform: none;
		letter-spacing: 0;
	}

	.detail-eye {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 18px;
		height: 18px;
		padding: 0;
		border: none;
		background: transparent;
		color: var(--color-text-muted);
		cursor: pointer;
		flex-shrink: 0;
		border-radius: 4px;
	}

	.detail-eye:hover {
		color: var(--color-text-primary);
		background: var(--color-bg-hover);
	}

	.info-key {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-muted);
	}

	.info-val {
		font-family: var(--font-mono);
		font-size: 0.8125rem;
		color: var(--color-text-secondary);
		text-align: right;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.actions {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.875rem;
	}

	.info-error {
		color: var(--color-error);
		white-space: normal;
		overflow-wrap: anywhere;
		max-width: 100%;
	}

	.info-pending {
		color: var(--color-text-muted);
	}

	@media (max-width: 640px) {
		.card {
			padding: 0.75rem;
		}

		.section-title {
			font-size: 0.9375rem;
		}

		.info-row {
			flex-direction: column;
			align-items: flex-start;
			gap: 0.25rem;
		}

		.info-key {
			font-size: 0.625rem;
		}

		.checked-at {
			font-size: 0.625rem;
		}

		.info-val {
			width: 100%;
			text-align: left;
			white-space: normal;
			overflow-wrap: anywhere;
		}

		.actions {
			flex-wrap: wrap;
		}

		.ip-check-wrap {
			width: 100%;
			flex-wrap: wrap;
		}
	}
</style>
