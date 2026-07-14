<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores/notifications';
	import { PageContainer, PageHeader } from '$lib/components/layout';
	import { Card, Tabs, Toggle, Input, Button, StatusDot, Stat } from '$lib/components/ui';
	import type {
		FreeTurnConfig,
		FreeTurnClientConfig,
		FreeTurnServerConfig,
		FreeTurnStatus
	} from '$lib/types';

	type ActiveTab = 'client' | 'server';

	let activeTab: ActiveTab = $state('client');
	let loading = $state(true);
	let saving = $state(false);

	let config: FreeTurnConfig | null = $state(null);
	let status: FreeTurnStatus | null = $state(null);

	let statusPoll: ReturnType<typeof setInterval> | undefined;

	onMount(async () => {
		await Promise.all([loadConfig(), loadStatus()]);
		loading = false;
		// Poll status every 3s while the page is open — same cadence the
		// tunnel list uses for its ping/latency badges.
		statusPoll = setInterval(loadStatus, 3000);
	});

	onDestroy(() => {
		if (statusPoll) clearInterval(statusPoll);
	});

	async function loadConfig() {
		try {
			config = normalizeConfig(await api.getFreeTurnConfig());
		} catch (e: any) {
			notifications.error('Не удалось загрузить конфигурацию FreeTurn: ' + (e.message || ''));
		}
	}

	// The backend omits empty optional fields (Go `omitempty`), so they
	// arrive as undefined. Svelte's <Input bind:value> throws
	// props_invalid_value on an undefined value, so coerce every optional
	// string field to "" before binding.
	function normalizeConfig(cfg: FreeTurnConfig): FreeTurnConfig {
		return {
			client: {
				...cfg.client,
				peer: cfg.client.peer ?? '',
				link: cfg.client.link ?? '',
				turnHost: cfg.client.turnHost ?? '',
				obfKey: cfg.client.obfKey ?? '',
				dnsServers: cfg.client.dnsServers ?? '',
				clientId: cfg.client.clientId ?? '',
				sub: cfg.client.sub ?? ''
			},
			server: {
				...cfg.server,
				connect: cfg.server.connect ?? '',
				obfKey: cfg.server.obfKey ?? '',
				clientsFile: cfg.server.clientsFile ?? ''
			}
		};
	}

	async function loadStatus() {
		try {
			status = await api.getFreeTurnStatus();
		} catch {
			// Status polling failures are silent — the page just keeps
			// showing the last known state, same as tunnel ping badges do.
		}
	}

	async function saveClientConfig(cfg: FreeTurnClientConfig) {
		saving = true;
		try {
			const updated = await api.updateFreeTurnClientConfig(cfg);
			if (config) config = { ...config, client: updated };
			notifications.success('Настройки клиента сохранены');
		} catch (e: any) {
			notifications.error('Не удалось сохранить: ' + (e.message || ''));
		} finally {
			saving = false;
		}
	}

	async function saveServerConfig(cfg: FreeTurnServerConfig) {
		saving = true;
		try {
			const updated = await api.updateFreeTurnServerConfig(cfg);
			if (config) config = { ...config, server: updated };
			notifications.success('Настройки сервера сохранены');
		} catch (e: any) {
			notifications.error('Не удалось сохранить: ' + (e.message || ''));
		} finally {
			saving = false;
		}
	}

	async function toggleClient(on: boolean) {
		try {
			if (on) {
				await api.startFreeTurnClient();
				notifications.success('FreeTurn клиент запущен');
			} else {
				await api.stopFreeTurnClient();
				notifications.success('FreeTurn клиент остановлен');
			}
			await loadStatus();
		} catch (e: any) {
			notifications.error(e.message || 'Не удалось переключить клиент');
			await loadStatus();
		}
	}

	async function toggleServer(on: boolean) {
		try {
			if (on) {
				await api.startFreeTurnServer();
				notifications.success('FreeTurn сервер запущен');
			} else {
				await api.stopFreeTurnServer();
				notifications.success('FreeTurn сервер остановлен');
			}
			await loadStatus();
		} catch (e: any) {
			notifications.error(e.message || 'Не удалось переключить сервер');
			await loadStatus();
		}
	}

	function formatUptime(startedAt?: string): string {
		if (!startedAt) return '';
		const ms = Date.now() - new Date(startedAt).getTime();
		const mins = Math.floor(ms / 60000);
		if (mins < 60) return `${mins} мин`;
		const hrs = Math.floor(mins / 60);
		return `${hrs}ч ${mins % 60}м`;
	}

	const tabs = [
		{ id: 'client', label: 'Клиент' },
		{ id: 'server', label: 'Сервер' }
	];
</script>

<svelte:head>
	<title>FreeTurn — AWG Manager</title>
</svelte:head>

<PageContainer width="wide">
	<PageHeader
		title="FreeTurn"
		description="TURN-туннель для обхода блокировок: клиент на роутере, сервер на VPS"
	/>

	<Tabs {tabs} active={activeTab} onchange={(id) => (activeTab = id as ActiveTab)} urlParam="tab" defaultTab="client" />

	{#if loading || !config}
		<div class="ft-loading">Загрузка…</div>
	{:else if activeTab === 'client'}
		{@const clientStatus = status?.client}
		<Card>
			{#snippet header()}
				<div class="ft-card-header">
					<div class="ft-card-title">
						<StatusDot variant={clientStatus?.running ? 'success' : 'muted'} pulse={clientStatus?.running} />
						<span>FreeTurn клиент</span>
						{#if clientStatus?.running}
							<span class="ft-uptime">запущен · {formatUptime(clientStatus.startedAt)}</span>
						{:else if clientStatus?.lastError}
							<span class="ft-error-hint">{clientStatus.lastError}</span>
						{/if}
					</div>
					<Toggle
						checked={!!clientStatus?.running}
						onchange={toggleClient}
						label=""
					/>
				</div>
			{/snippet}

			<div class="ft-section-label">TURN-сервер</div>
			<div class="ft-grid-2">
				<Input label="Адрес сервера (-peer)" bind:value={config.client.peer} placeholder="vinvanvlad.com:56000" />
				<Input label="Провайдер (-provider)" bind:value={config.client.provider} placeholder="vk" />
			</div>
			<Input
				label="Ссылка VK Calls (-link)"
				bind:value={config.client.link}
				placeholder="https://vk.ru/call/join/..."
			/>
			<p class="ft-hint">Обязательна, если -provider = vk</p>

			<div class="ft-section-label">Туннелирование</div>
			<div class="ft-grid-3">
				<div>
					<label class="ft-label" for="ft-c-mode">Режим (-mode)</label>
					<select id="ft-c-mode" bind:value={config.client.mode} class="ft-select">
						<option value="udp">udp</option>
						<option value="tcp">tcp</option>
					</select>
				</div>
				<div>
					<label class="ft-label" for="ft-c-transport">Транспорт до TURN (-transport)</label>
					<select id="ft-c-transport" bind:value={config.client.transport} class="ft-select">
						<option value="tcp">tcp</option>
						<option value="udp">udp</option>
					</select>
				</div>
				<Input
					label="Локальный адрес (-listen)"
					bind:value={config.client.listen}
					placeholder="127.0.0.1:9000"
				/>
			</div>
			<div class="ft-grid-2">
				<Input
					label="Потоков TURN (-n)"
					type="number"
					value={String(config.client.streams)}
					onchange={(v) => (config!.client.streams = Number(v) || 0)}
				/>
				<div class="ft-checkbox-row">
					<input id="ft-c-bond" type="checkbox" bind:checked={config.client.bond} />
					<label for="ft-c-bond">Бондинг через все smux-сессии (-bond, только mode=tcp)</label>
				</div>
			</div>

			<div class="ft-section-label">Обфускация</div>
			<div class="ft-grid-2">
				<div>
					<label class="ft-label" for="ft-c-obf">Профиль (-obf-profile)</label>
					<select id="ft-c-obf" bind:value={config.client.obfProfile} class="ft-select">
						<option value="none">none</option>
						<option value="rtpopus">rtpopus</option>
						<option value="rtpopus2">rtpopus2</option>
					</select>
				</div>
				<Input
					label="Ключ обфускации (-obf-key)"
					type="password"
					bind:value={config.client.obfKey}
					placeholder="64 hex-символа"
				/>
			</div>

			{#snippet footer()}
				<div class="ft-footer">
					<Button variant="primary" size="sm" loading={saving} onclick={() => saveClientConfig(config!.client)}>
						Сохранить
					</Button>
				</div>
			{/snippet}
		</Card>

		{#if clientStatus?.running}
			<Card variant="nested" padding="sm">
				<div class="ft-stats">
					<Stat value={clientStatus.pid ? String(clientStatus.pid) : '—'} label="PID" />
					<Stat value={config.client.streams ? String(config.client.streams) : '—'} label="потоков" />
					<Stat value={config.client.obfProfile} label="профиль" />
				</div>
			</Card>
		{/if}
	{:else}
		{@const serverStatus = status?.server}
		<Card>
			{#snippet header()}
				<div class="ft-card-header">
					<div class="ft-card-title">
						<StatusDot variant={serverStatus?.running ? 'success' : 'muted'} pulse={serverStatus?.running} />
						<span>FreeTurn сервер</span>
						{#if serverStatus?.running}
							<span class="ft-uptime">запущен · {formatUptime(serverStatus.startedAt)}</span>
						{:else if serverStatus?.lastError}
							<span class="ft-error-hint">{serverStatus.lastError}</span>
						{/if}
					</div>
					<Toggle checked={!!serverStatus?.running} onchange={toggleServer} label="" />
				</div>
			{/snippet}

			<div class="ft-section-label">Приём подключений</div>
			<div class="ft-grid-2">
				<Input label="Слушать (-listen)" bind:value={config.server.listen} placeholder="0.0.0.0:56000" />
				<div>
					<label class="ft-label" for="ft-s-mode">Режим (-mode)</label>
					<select id="ft-s-mode" bind:value={config.server.mode} class="ft-select">
						<option value="udp">udp</option>
						<option value="tcp">tcp</option>
					</select>
				</div>
			</div>

			<div class="ft-section-label">Куда форвардить</div>
			<Input
				label="Backend-адрес (-connect)"
				bind:value={config.server.connect}
				placeholder="127.0.0.1:51820"
			/>
			<p class="ft-hint">WireGuard — обычно 127.0.0.1:51820, Xray — 127.0.0.1:443</p>

			<div class="ft-section-label">Обфускация и доступ</div>
			<div class="ft-grid-2">
				<div>
					<label class="ft-label" for="ft-s-obf">Профиль (-obf-profile)</label>
					<select id="ft-s-obf" bind:value={config.server.obfProfile} class="ft-select">
						<option value="none">none</option>
						<option value="rtpopus">rtpopus</option>
						<option value="rtpopus2">rtpopus2</option>
					</select>
				</div>
				<Input
					label="Ключ обфускации (-obf-key)"
					type="password"
					bind:value={config.server.obfKey}
					placeholder="64 hex-символа"
				/>
			</div>
			<Input
				label="Файл allowlist клиентов (-clients-file)"
				bind:value={config.server.clientsFile}
				placeholder="оставьте пустым — без проверки Client ID"
			/>

			{#snippet footer()}
				<div class="ft-footer">
					<Button variant="primary" size="sm" loading={saving} onclick={() => saveServerConfig(config!.server)}>
						Сохранить
					</Button>
				</div>
			{/snippet}
		</Card>
	{/if}
</PageContainer>

<style>
	.ft-loading {
		padding: 2rem;
		text-align: center;
		color: var(--color-text-secondary);
	}

	.ft-card-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.ft-card-title {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: 500;
		flex-wrap: wrap;
	}

	.ft-uptime,
	.ft-error-hint {
		font-size: 0.75rem;
		color: var(--color-text-secondary);
		font-weight: 400;
	}

	.ft-error-hint {
		color: var(--color-error);
	}

	.ft-section-label {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--color-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.03em;
		margin: 1.25rem 0 0.625rem;
	}

	.ft-section-label:first-of-type {
		margin-top: 0.5rem;
	}

	.ft-grid-2 {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
	}

	.ft-grid-3 {
		display: grid;
		grid-template-columns: 1fr 1fr 1fr;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
	}

	.ft-label {
		display: block;
		font-size: 0.8125rem;
		color: var(--color-text-secondary);
		margin-bottom: 0.25rem;
	}

	.ft-select {
		width: 100%;
		padding: 0.5rem 0.625rem;
		border-radius: var(--radius-sm);
		border: 1px solid var(--color-border);
		background: var(--color-bg-tertiary);
		color: var(--color-text-primary);
		font: inherit;
	}

	.ft-checkbox-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding-top: 1.5rem;
		font-size: 0.8125rem;
		color: var(--color-text-secondary);
	}

	.ft-hint {
		font-size: 0.75rem;
		color: var(--color-text-secondary);
		margin: -0.5rem 0 0.75rem;
	}

	.ft-footer {
		display: flex;
		justify-content: flex-end;
	}

	.ft-stats {
		display: flex;
		gap: 0.5rem;
	}

	@media (max-width: 640px) {
		.ft-grid-2,
		.ft-grid-3 {
			grid-template-columns: 1fr;
		}
	}
</style>
