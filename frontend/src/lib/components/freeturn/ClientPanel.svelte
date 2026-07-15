<script lang="ts">
	import { Input, Button, Dropdown, FormToggle } from '$lib/components/ui';
	import type { FreeTurnClientConfig, FreeTurnProcessStatus } from '$lib/types';
	import ProcessHero from './ProcessHero.svelte';
	import UnsavedBar from './UnsavedBar.svelte';
	import SettingRows from './SettingRows.svelte';
	import SettingRow from './SettingRow.svelte';
	import { formatUptime } from './uptime';

	interface Props {
		client: FreeTurnClientConfig;
		/** Снапшот сохранённого конфига — для dirty-подсветки и счётчика. */
		saved: FreeTurnClientConfig | null;
		status?: FreeTurnProcessStatus;
		saving: boolean;
		routerHost: string;
		importing: boolean;
		importedWG: string | null;
		installAvailable: boolean;
		installVersion?: string;
		installing: boolean;
		expanded: string | null;
		importOpen: boolean;
		logOpen: boolean;
		onInstall: () => void;
		onToggle: (on: boolean) => void;
		onSave: () => void;
		onRevert: () => void;
		onImport: (link: string) => void;
		onCopy: (text: string) => void;
	}

	let {
		client,
		saved,
		status,
		saving,
		routerHost,
		importing,
		importedWG,
		installAvailable,
		installVersion,
		installing,
		expanded = $bindable(),
		importOpen = $bindable(),
		logOpen = $bindable(),
		onInstall,
		onToggle,
		onSave,
		onRevert,
		onImport,
		onCopy
	}: Props = $props();

	let importLink = $state('');

	const modeOptions = [
		{ value: 'udp', label: 'udp' },
		{ value: 'tcp', label: 'tcp' }
	];
	const transportOptions = [
		{ value: 'tcp', label: 'tcp' },
		{ value: 'udp', label: 'udp' }
	];
	const obfOptions = [
		{ value: 'none', label: 'none' },
		{ value: 'rtpopus', label: 'rtpopus' },
		{ value: 'rtpopus2', label: 'rtpopus2' },
		{ value: 'rtpopus3', label: 'rtpopus3' }
	];

	function changed(...keys: (keyof FreeTurnClientConfig)[]): boolean {
		return saved != null && keys.some((k) => client[k] !== saved[k]);
	}

	const dirtyCount = $derived(
		saved
			? (Object.keys(client) as (keyof FreeTurnClientConfig)[]).filter(
					(k) => client[k] !== saved[k]
				).length
			: 0
	);

	const linksCount = $derived(
		client.links ? client.links.split(',').filter((s) => s.trim()).length : 0
	);

	function linksSummary(n: number): string {
		if (!n) return '—';
		const mod10 = n % 10;
		const mod100 = n % 100;
		if (mod10 === 1 && mod100 !== 11) return `${n} ссылка`;
		if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20)) return `${n} ссылки`;
		return `${n} ссылок`;
	}

	const metaParts = $derived(
		[
			formatUptime(status?.startedAt),
			status?.pid ? `PID ${status.pid}` : '',
			`${client.mode} / ${client.transport}`,
			client.obfProfile
		].filter(Boolean)
	);

	function toggleRow(id: string) {
		expanded = expanded === id ? null : id;
	}
</script>

<ProcessHero
	title="Клиент"
	{status}
	{metaParts}
	actionLabel="Импорт ссылки"
	{logOpen}
	{installAvailable}
	{installVersion}
	{installing}
	{onInstall}
	onAction={() => (importOpen = !importOpen)}
	{onToggle}
	onToggleLog={() => (logOpen = !logOpen)}
/>

{#if importOpen}
	<div class="ft-panel-accent">
		<div class="ft-section-label">Импорт по ссылке freeturn://</div>
		<div class="ft-import-row">
			<Input bind:value={importLink} placeholder="freeturn://..." />
			<Button variant="primary" size="sm" loading={importing} onclick={() => onImport(importLink)}>
				Применить
			</Button>
			<Button variant="ghost" size="sm" onclick={() => (importOpen = false)}>Закрыть</Button>
		</div>
		<p class="ft-hint">
			Заполнит поля ниже (сохранение — кнопкой «Сохранить») и, если в ссылке есть
			WireGuard-конфиг, сразу создаст из него туннель во вкладке «AWG»
		</p>
		{#if importedWG}
			<div class="ft-section-label">WireGuard-конфиг из ссылки</div>
			<textarea class="ft-textarea" readonly value={importedWG}></textarea>
			<Button variant="ghost" size="sm" onclick={() => onCopy(importedWG!)}>Скопировать конфиг</Button>
		{/if}
	</div>
{/if}

{#if dirtyCount > 0}
	<UnsavedBar count={dirtyCount} target="клиента" {saving} {onSave} {onRevert} />
{/if}

<SettingRows>
	<SettingRow
		id="peer"
		group="Подключение"
		label="Адрес сервера"
		flag="-peer"
		summary={client.peer || '—'}
		dirty={changed('peer')}
		expanded={expanded === 'peer'}
		ontoggle={toggleRow}
	>
		<Input label="Адрес сервера (-peer)" bind:value={client.peer} placeholder="vinvanvlad.com:56000" />
	</SettingRow>
	<SettingRow
		id="provider"
		label="Провайдер"
		flag="-provider"
		summary={client.provider || '—'}
		dirty={changed('provider')}
		expanded={expanded === 'provider'}
		ontoggle={toggleRow}
	>
		<Input label="Провайдер (-provider)" bind:value={client.provider} placeholder="vk" />
	</SettingRow>
	<SettingRow
		id="links"
		label="Ссылки VK Calls"
		flag="-links"
		summary={linksSummary(linksCount)}
		dirty={changed('links')}
		expanded={expanded === 'links'}
		ontoggle={toggleRow}
	>
		<div class="ft-span">
			<label class="ft-label" for="ft-c-links">Ссылки VK Calls, через запятую (-links)</label>
			<textarea
				id="ft-c-links"
				class="ft-textarea"
				style="min-height: 70px"
				bind:value={client.links}
				placeholder="https://vk.ru/call/join/...,https://vk.ru/call/join/..."
			></textarea>
			<p class="ft-hint">
				Обязательны, если -provider = vk. Несколько ссылок дают несколько независимых пулов
				TURN-кредов — больше суммарных потоков и меньше риск бана одного звонка
			</p>
		</div>
	</SettingRow>
	<SettingRow
		id="vk"
		label="VK: капча и креды"
		flag="-streams-per-cred / -manual-captcha"
		summary={`${client.streamsPerCred} на кред · капча ${client.manualCaptcha ? 'вручную' : 'авто'}`}
		dirty={changed('streamsPerCred', 'manualCaptcha')}
		expanded={expanded === 'vk'}
		ontoggle={toggleRow}
	>
		<Input
			label="Потоков на кред (-streams-per-cred)"
			type="number"
			value={String(client.streamsPerCred)}
			onchange={(v) => (client.streamsPerCred = Number(v) || 0)}
		/>
		<div class="ft-toggle-slot">
			<FormToggle bind:checked={client.manualCaptcha} label="Ручная капча (-manual-captcha)" />
		</div>
		{#if client.manualCaptcha}
			<p class="ft-hint ft-span">
				Капча решается локальным HTTP-сервером самого freeturn-client на роутере
				(127.0.0.1:8765) — снаружи он недоступен. Пробросьте порт с вашего ПК:
				<code>ssh -N -L 8765:127.0.0.1:8765 root@{routerHost || '<IP роутера>'}</code>
				и откройте <code>http://127.0.0.1:8765</code> в браузере (порт SSH может отличаться
				от 22 — на Keenetic часто 222).
			</p>
		{/if}
	</SettingRow>
	<SettingRow
		id="modeTransport"
		group="Туннель и обфускация"
		label="Режим и транспорт"
		flag="-mode / -transport"
		summary={`${client.mode} / ${client.transport}`}
		dirty={changed('mode', 'transport', 'listen')}
		expanded={expanded === 'modeTransport'}
		ontoggle={toggleRow}
	>
		<Dropdown label="Режим (-mode)" bind:value={client.mode} options={modeOptions} />
		<Dropdown
			label="Транспорт до TURN (-transport)"
			bind:value={client.transport}
			options={transportOptions}
		/>
		<Input label="Локальный адрес (-listen)" bind:value={client.listen} placeholder="127.0.0.1:9000" />
	</SettingRow>
	<SettingRow
		id="streams"
		label="Потоки"
		flag="-n"
		summary={`${client.streams}${client.bond ? ' · бондинг' : ''}`}
		dirty={changed('streams', 'bond')}
		expanded={expanded === 'streams'}
		ontoggle={toggleRow}
	>
		<Input
			label="Потоков TURN (-n)"
			type="number"
			value={String(client.streams)}
			onchange={(v) => (client.streams = Number(v) || 0)}
		/>
		<div class="ft-toggle-slot">
			<FormToggle
				bind:checked={client.bond}
				label="Бондинг через все smux-сессии (-bond, только mode=tcp)"
			/>
		</div>
	</SettingRow>
	<SettingRow
		id="obf"
		label="Обфускация"
		flag="-obf-profile / -obf-key"
		summary={`${client.obfProfile}${client.obfKey ? ' · ключ задан' : ''}`}
		dirty={changed('obfProfile', 'obfKey')}
		expanded={expanded === 'obf'}
		ontoggle={toggleRow}
	>
		<Dropdown label="Профиль (-obf-profile)" bind:value={client.obfProfile} options={obfOptions} />
		<Input
			label="Ключ обфускации (-obf-key)"
			type="password"
			bind:value={client.obfKey}
			placeholder="64 hex-символа"
		/>
	</SettingRow>
	<SettingRow
		id="clientId"
		label="Client ID"
		flag="-client-id"
		summary={client.clientId ? client.clientId.slice(0, 12) + '…' : 'авто'}
		dirty={changed('clientId')}
		expanded={expanded === 'clientId'}
		ontoggle={toggleRow}
	>
		<div class="ft-span">
			<Input
				label="Client ID (-client-id)"
				bind:value={client.clientId}
				placeholder="оставьте пустым — сгенерируется и сохранится автоматически"
			/>
			<p class="ft-hint" style="margin-top: 0.375rem">
				Заполняется автоматически из ссылки freeturn:// (поле «cid»), если она с ним пришла. Если
				сервер использует allowlist (-clients-file), этот же ID должен быть добавлен там владельцем
				сервера — иначе сервер отклонит подключение с «Unauthorized Client ID»
			</p>
		</div>
	</SettingRow>
</SettingRows>

<style>
	.ft-section-label {
		font-size: 0.6875rem;
		font-weight: 600;
		color: var(--color-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		margin: 0 0 0.5rem;
	}

	.ft-panel-accent {
		padding: 0.875rem 1rem;
		background: var(--color-bg-secondary);
		border: 1px solid var(--color-accent-border);
		border-radius: var(--radius);
		margin-bottom: 0.625rem;
	}

	.ft-import-row {
		display: grid;
		grid-template-columns: 1fr auto auto;
		gap: 0.5rem;
		align-items: center;
		margin-bottom: 0.5rem;
	}

	.ft-span {
		grid-column: 1 / -1;
		min-width: 0;
	}

	.ft-label {
		display: block;
		font-size: 0.8125rem;
		color: var(--color-text-secondary);
		margin-bottom: 0.25rem;
	}

	.ft-toggle-slot {
		display: flex;
		align-items: flex-end;
		padding-bottom: 0.25rem;
	}

	.ft-hint {
		font-size: 0.75rem;
		color: var(--color-text-secondary);
		margin: 0;
	}

	.ft-textarea {
		width: 100%;
		box-sizing: border-box;
		min-height: 100px;
		padding: 0.5rem 0.625rem;
		border-radius: var(--radius-sm);
		border: 1px solid var(--color-border);
		background: var(--color-bg-tertiary);
		color: var(--color-text-primary);
		font-family: var(--font-mono);
		font-size: 0.8125rem;
		resize: vertical;
		white-space: pre;
		margin-bottom: 0.375rem;
	}

	@media (max-width: 640px) {
		.ft-import-row {
			grid-template-columns: 1fr;
		}
	}
</style>
