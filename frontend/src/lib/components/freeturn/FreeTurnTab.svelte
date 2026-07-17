<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores/notifications';
	import { copyToClipboard as copyText } from '$lib/utils/clipboard';
	import { Tabs } from '$lib/components/ui';
	import type { FreeTurnConfig, FreeTurnClientConfig, FreeTurnServerConfig, FreeTurnStatus } from '$lib/types';
	import ClientPanel from './ClientPanel.svelte';
	import ServerPanel from './ServerPanel.svelte';
	import StatusStrip from './StatusStrip.svelte';

	// Оркестратор вкладки FreeTurn на главной: владеет API-вызовами,
	// конфигом и поллингом статуса; панели — чистый UI на props/callbacks.
	type FtTab = 'client' | 'server';

	let ftTab: FtTab = $state('client');
	let loading = $state(true);
	let saving = $state(false);

	let config: FreeTurnConfig | null = $state(null);
	let status: FreeTurnStatus | null = $state(null);
	// Снапшот сохранённого конфига — источник dirty-подсветки и «Отменить».
	let savedConfig: FreeTurnConfig | null = $state(null);

	let expanded: string | null = $state(null);

	let importing = $state(false);
	let importedWG: string | null = $state(null);
	let installing = $state(false);

	let genProvider = $state('vk');
	let genMTU = $state(1376);
	let genWG = $state('');
	let genClientId = $state('');
	let genName = $state('');
	let generating = $state(false);
	let generatedLink = $state('');
	let generatedPeer = $state('');
	let generatedClientId = $state('');

	let statusPoll: ReturnType<typeof setInterval> | undefined;
	let routerHost = $state('');

	function errText(e: unknown): string {
		return e instanceof Error ? e.message : String(e ?? '');
	}

	// Бейджи «вкл» не нужны: состояние обоих процессов всегда видно в пульте сверху.
	const ftTabs = [
		{ id: 'client', label: 'Клиент' },
		{ id: 'server', label: 'Сервер' }
	];

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
			const norm = normalizeConfig(await api.getFreeTurnConfig());
			savedConfig = structuredClone(norm);
			config = norm;
		} catch (e) {
			notifications.error('Не удалось загрузить конфигурацию FreeTurn: ' + errText(e));
		}
	}

	// Backend опускает пустые опциональные поля (Go omitempty) — они приходят
	// undefined, а <Input bind:value> кидает props_invalid_value; приводим к "".
	function normalizeClient(c: FreeTurnClientConfig): FreeTurnClientConfig {
		return {
			...c,
			peer: c.peer ?? '',
			links: c.links ?? '',
			turnHost: c.turnHost ?? '',
			obfKey: c.obfKey ?? '',
			dnsServers: c.dnsServers ?? '',
			clientId: c.clientId ?? '',
			sub: c.sub ?? ''
		};
	}

	function normalizeServer(s: FreeTurnServerConfig): FreeTurnServerConfig {
		return {
			...s,
			connect: s.connect ?? '',
			obfKey: s.obfKey ?? '',
			clientsFile: s.clientsFile ?? ''
		};
	}

	function normalizeConfig(cfg: FreeTurnConfig): FreeTurnConfig {
		return { client: normalizeClient(cfg.client), server: normalizeServer(cfg.server) };
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
			const sent = $state.snapshot(cfg);
			const norm = normalizeClient(await api.updateFreeTurnClientConfig(cfg));
			if (config && savedConfig) {
				savedConfig.client = norm;
				// Не затирать правки, сделанные пока PUT был в полёте:
				// форму обновляем эхом сервера, только если она не менялась.
				if (JSON.stringify($state.snapshot(config.client)) === JSON.stringify(sent)) {
					config.client = structuredClone(norm);
				}
			}
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
			const sent = $state.snapshot(cfg);
			const norm = normalizeServer(await api.updateFreeTurnServerConfig(cfg));
			if (config && savedConfig) {
				savedConfig.server = norm;
				if (JSON.stringify($state.snapshot(config.server)) === JSON.stringify(sent)) {
					config.server = structuredClone(norm);
				}
			}
			notifications.success('Настройки сервера сохранены');
		} catch (e) {
			notifications.error('Не удалось сохранить: ' + errText(e));
		} finally {
			saving = false;
		}
	}

	function revertClient() {
		if (config && savedConfig) config.client = $state.snapshot(savedConfig.client);
	}

	function revertServer() {
		if (config && savedConfig) config.server = $state.snapshot(savedConfig.server);
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
				if (payload.n && payload.n > 0) config.client.streams = payload.n;
				if (payload.spc && payload.spc > 0) config.client.streamsPerCred = payload.spc;
				if (payload.cid) config.client.clientId = payload.cid;
				if (payload.transport) config.client.transport = payload.transport as typeof config.client.transport;
				if (payload.mode) config.client.mode = payload.mode as typeof config.client.mode;
				if (typeof payload.bond === 'boolean') config.client.bond = payload.bond;
				if (typeof payload.mcap === 'boolean') config.client.manualCaptcha = payload.mcap;
			}
			const wg = payload.wg?.trim() ? payload.wg : null;
			importedWG = wg;

			let msg = 'Ссылка распознана, поля заполнены — не забудьте сохранить';
			if (payload.cid) {
				msg += `. В ссылке был Client ID — если у сервера включён allowlist (-clients-file), владелец сервера должен добавить именно этот ID туда`;
			}
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

	async function generateLink(provider: string, mtu: number, wg: string, clientId: string, name: string) {
		generating = true;
		try {
			const result = await api.generateFreeTurnLink({
				provider,
				mtu,
				wg: wg.trim() || undefined,
				clientId: clientId.trim() || undefined,
				name: name.trim() || undefined
			});
			generatedLink = result.link;
			generatedPeer = result.peer;
			generatedClientId = result.clientId ?? '';
		} catch (e) {
			notifications.error('Не удалось сгенерировать ссылку: ' + errText(e));
		} finally {
			generating = false;
		}
	}
</script>

<StatusStrip
	client={status?.client}
	server={status?.server}
	onToggleClient={toggleClient}
	onToggleServer={toggleServer}
/>

<Tabs
	tabs={ftTabs}
	active={ftTab}
	onchange={(id) => {
		ftTab = id as FtTab;
		expanded = null;
	}}
	urlParam="ft"
	defaultTab="client"
/>

{#if loading || !config}
	<div class="ft-loading">Загрузка…</div>
{:else if ftTab === 'client'}
	<ClientPanel
		client={config.client}
		saved={savedConfig?.client ?? null}
		status={status?.client}
		{saving}
		{routerHost}
		{importing}
		{importedWG}
		installAvailable={status?.installAvailable ?? false}
		installVersion={status?.installVersion}
		installing={installing || (status?.installing ?? false)}
		bind:expanded
		onInstall={install}
		onSave={() => saveClientConfig(config!.client)}
		onRevert={revertClient}
		onImport={applyImportLink}
		onCopy={copy}
	/>
{:else}
	<ServerPanel
		server={config.server}
		saved={savedConfig?.server ?? null}
		status={status?.server}
		{saving}
		installAvailable={status?.installAvailable ?? false}
		installVersion={status?.installVersion}
		installing={installing || (status?.installing ?? false)}
		onInstall={install}
		{generating}
		{generatedLink}
		{generatedPeer}
		{generatedClientId}
		bind:genProvider
		bind:genMTU
		bind:genWG
		bind:genClientId
		bind:genName
		bind:expanded
		onSave={() => saveServerConfig(config!.server)}
		onRevert={revertServer}
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
