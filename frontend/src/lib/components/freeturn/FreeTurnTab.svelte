<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores/notifications';
	import { copyToClipboard as copyText } from '$lib/utils/clipboard';
	import { Tabs } from '$lib/components/ui';
	import type { FreeTurnConfig, FreeTurnClientConfig, FreeTurnServerConfig, FreeTurnStatus } from '$lib/types';
	import ClientPanel from './ClientPanel.svelte';
	import ServerPanel from './ServerPanel.svelte';

	// Оркестратор вкладки FreeTurn на главной: владеет API-вызовами,
	// конфигом и поллингом статуса; панели — чистый UI на props/callbacks.
	type FtTab = 'client' | 'server';

	let ftTab: FtTab = $state('client');
	let loading = $state(true);
	let saving = $state(false);

	let config: FreeTurnConfig | null = $state(null);
	let status: FreeTurnStatus | null = $state(null);

	let importing = $state(false);
	let importedWG: string | null = $state(null);
	let installing = $state(false);

	let genProvider = $state('vk');
	let genMTU = $state(1376);
	let genWG = $state('');
	let generating = $state(false);
	let generatedLink = $state('');
	let generatedPeer = $state('');

	let statusPoll: ReturnType<typeof setInterval> | undefined;
	let routerHost = $state('');

	function errText(e: unknown): string {
		return e instanceof Error ? e.message : String(e ?? '');
	}

	const ftTabs = $derived.by(() => [
		{
			id: 'client',
			label: 'Клиент',
			badge: status?.client.running ? 'вкл' : undefined,
			badgeTone: 'success' as const
		},
		{
			id: 'server',
			label: 'Сервер',
			badge: status?.server.running ? 'вкл' : undefined,
			badgeTone: 'success' as const
		}
	]);

	onMount(async () => {
		routerHost = window.location.hostname;
		await Promise.all([loadConfig(), loadStatus()]);
		loading = false;
		// Поллинг живёт, пока вкладка смонтирована (уход с вкладки = unmount).
		statusPoll = setInterval(loadStatus, 3000);
	});

	onDestroy(() => {
		if (statusPoll) clearInterval(statusPoll);
	});

	async function loadConfig() {
		try {
			config = normalizeConfig(await api.getFreeTurnConfig());
		} catch (e) {
			notifications.error('Не удалось загрузить конфигурацию FreeTurn: ' + errText(e));
		}
	}

	// Backend опускает пустые опциональные поля (Go omitempty) — они приходят
	// undefined, а <Input bind:value> кидает props_invalid_value; приводим к "".
	function normalizeConfig(cfg: FreeTurnConfig): FreeTurnConfig {
		return {
			client: {
				...cfg.client,
				peer: cfg.client.peer ?? '',
				links: cfg.client.links ?? '',
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
			// Молча: страница показывает последнее известное состояние,
			// как ping-бейджи списка туннелей.
		}
	}

	async function saveClientConfig(cfg: FreeTurnClientConfig) {
		saving = true;
		try {
			const updated = await api.updateFreeTurnClientConfig(cfg);
			if (config) config = { ...config, client: updated };
			notifications.success('Настройки клиента сохранены');
		} catch (e) {
			notifications.error('Не удалось сохранить: ' + errText(e));
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
		} catch (e) {
			notifications.error('Не удалось сохранить: ' + errText(e));
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
		} catch (e) {
			notifications.error(errText(e) || 'Не удалось переключить клиент');
		} finally {
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
		} catch (e) {
			notifications.error(errText(e) || 'Не удалось переключить сервер');
		} finally {
			await loadStatus();
		}
	}

	async function install() {
		installing = true;
		try {
			await api.installFreeTurn();
			notifications.success('freeturn установлен (клиент + сервер)');
		} catch (e) {
			notifications.error('Не удалось установить freeturn: ' + errText(e));
		} finally {
			installing = false;
			await loadStatus();
		}
	}

	async function copy(text: string) {
		if (await copyText(text)) {
			notifications.success('Скопировано в буфер');
		} else {
			notifications.error('Не удалось скопировать');
		}
	}

	async function applyImportLink(link: string) {
		if (!link.trim()) return;
		importing = true;
		try {
			const payload = await api.decodeFreeTurnLink(link.trim());
			if (config) {
				config.client.peer = payload.peer ?? config.client.peer;
				config.client.provider = payload.provider || config.client.provider;
				if (payload.obf) {
					config.client.obfProfile = payload.obf as typeof config.client.obfProfile;
				}
				config.client.obfKey = payload.key ?? config.client.obfKey;
			}
			const wg = payload.wg?.trim() ? payload.wg : null;
			importedWG = wg;

			let msg = 'Ссылка распознана, поля заполнены — не забудьте сохранить';
			if (wg) {
				try {
					const tunnel = await api.importConfig(wg, `FreeTurn ${payload.peer}`.slice(0, 60));
					msg += `. Создан туннель «${tunnel.name}»`;
				} catch (e) {
					notifications.error('Поля заполнены, но не удалось создать туннель из конфига: ' + errText(e));
				}
			}
			notifications.success(msg);
		} catch (e) {
			notifications.error('Не удалось разобрать ссылку: ' + errText(e));
		} finally {
			importing = false;
		}
	}

	async function generateLink(provider: string, mtu: number, wg: string) {
		generating = true;
		try {
			const result = await api.generateFreeTurnLink({
				provider,
				mtu,
				wg: wg.trim() || undefined
			});
			generatedLink = result.link;
			generatedPeer = result.peer;
		} catch (e) {
			notifications.error('Не удалось сгенерировать ссылку: ' + errText(e));
		} finally {
			generating = false;
		}
	}
</script>

<Tabs tabs={ftTabs} active={ftTab} onchange={(id) => (ftTab = id as FtTab)} urlParam="ft" defaultTab="client" />

{#if loading || !config}
	<div class="ft-loading">Загрузка…</div>
{:else if ftTab === 'client'}
	<ClientPanel
		client={config.client}
		status={status?.client}
		{saving}
		{routerHost}
		{importing}
		{importedWG}
		installAvailable={status?.installAvailable ?? false}
		installVersion={status?.installVersion}
		installing={installing || (status?.installing ?? false)}
		onInstall={install}
		onToggle={toggleClient}
		onSave={() => saveClientConfig(config!.client)}
		onImport={applyImportLink}
		onCopy={copy}
	/>
{:else}
	<ServerPanel
		server={config.server}
		status={status?.server}
		{saving}
		installAvailable={status?.installAvailable ?? false}
		installVersion={status?.installVersion}
		installing={installing || (status?.installing ?? false)}
		onInstall={install}
		{generating}
		{generatedLink}
		{generatedPeer}
		bind:genProvider
		bind:genMTU
		bind:genWG
		onToggle={toggleServer}
		onSave={() => saveServerConfig(config!.server)}
		onGenerate={generateLink}
		onCopy={copy}
	/>
{/if}

<style>
	.ft-loading {
		padding: 2rem;
		text-align: center;
		color: var(--color-text-secondary);
	}
</style>
