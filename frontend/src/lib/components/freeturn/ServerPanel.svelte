<script lang="ts">
	import { Input, Button, Dropdown } from '$lib/components/ui';
	import type { FreeTurnServerConfig, FreeTurnProcessStatus } from '$lib/types';
	import ProcessHero from './ProcessHero.svelte';
	import UnsavedBar from './UnsavedBar.svelte';
	import SettingRows from './SettingRows.svelte';
	import SettingRow from './SettingRow.svelte';
	import { formatUptime } from './uptime';
	import { changedKeys } from './dirty';
	import { modeOptions, obfOptions } from './options';

	interface Props {
		server: FreeTurnServerConfig;
		/** Снапшот сохранённого конфига — для dirty-подсветки и счётчика. */
		saved: FreeTurnServerConfig | null;
		status?: FreeTurnProcessStatus;
		saving: boolean;
		installAvailable: boolean;
		installVersion?: string;
		installing: boolean;
		generating: boolean;
		generatedLink: string;
		generatedPeer: string;
		generatedClientId: string;
		genProvider: string;
		genMTU: number;
		genWG: string;
		genClientId: string;
		genName: string;
		expanded: string | null;
		genOpen: boolean;
		logOpen: boolean;
		onInstall: () => void;
		onToggle: (on: boolean) => void;
		onSave: () => void;
		onRevert: () => void;
		onGenerate: (provider: string, mtu: number, wg: string, clientId: string, name: string) => void;
		onCopy: (text: string) => void;
	}

	let {
		server,
		saved,
		status,
		saving,
		installAvailable,
		installVersion,
		installing,
		generating,
		generatedLink,
		generatedPeer,
		generatedClientId,
		genProvider = $bindable(),
		genMTU = $bindable(),
		genWG = $bindable(),
		genClientId = $bindable(),
		genName = $bindable(),
		expanded = $bindable(),
		genOpen = $bindable(),
		logOpen = $bindable(),
		onInstall,
		onToggle,
		onSave,
		onRevert,
		onGenerate,
		onCopy
	}: Props = $props();

	// Доп. поля генератора (Client ID + WireGuard-конфиг) свёрнуты по умолчанию,
	// но раскрыты сразу, если в них уже есть данные — они попадают в ссылку,
	// и пользователь должен их видеть.
	let genMore = $state(genClientId.trim() !== '' || genWG.trim() !== '');

	function randomClientId() {
		const bytes = new Uint8Array(16);
		crypto.getRandomValues(bytes);
		genClientId = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
	}

	const dirtyKeys = $derived(changedKeys(server, saved));
	const dirtyCount = $derived(dirtyKeys.length);

	function changed(...keys: (keyof FreeTurnServerConfig)[]): boolean {
		return keys.some((k) => dirtyKeys.includes(k));
	}

	const metaParts = $derived(
		[
			formatUptime(status?.startedAt),
			status?.pid ? `PID ${status.pid}` : '',
			server.mode,
			server.obfProfile
		].filter(Boolean)
	);

	function toggleRow(id: string) {
		expanded = expanded === id ? null : id;
	}
</script>

<ProcessHero
	title="Сервер"
	{status}
	{metaParts}
	actionLabel="Ссылка для клиента"
	{logOpen}
	{installAvailable}
	{installVersion}
	{installing}
	{onInstall}
	onAction={() => (genOpen = !genOpen)}
	{onToggle}
	onToggleLog={() => (logOpen = !logOpen)}
/>

{#if dirtyCount > 0}
	<UnsavedBar count={dirtyCount} target="сервера" {saving} {onSave} {onRevert} />
{/if}

<SettingRows>
	<SettingRow
		id="listen"
		group="Приём подключений"
		label="Слушать"
		flag="-listen"
		summary={server.listen || '—'}
		dirty={changed('listen')}
		expanded={expanded === 'listen'}
		ontoggle={toggleRow}
	>
		<Input label="Слушать (-listen)" bind:value={server.listen} placeholder="0.0.0.0:56000" />
	</SettingRow>
	<SettingRow
		id="smode"
		label="Режим"
		flag="-mode"
		summary={server.mode}
		dirty={changed('mode')}
		expanded={expanded === 'smode'}
		ontoggle={toggleRow}
	>
		<Dropdown label="Режим (-mode)" bind:value={server.mode} options={modeOptions} />
	</SettingRow>
	<SettingRow
		id="connect"
		group="Форвардинг и доступ"
		label="Backend-адрес"
		flag="-connect"
		summary={server.connect || '—'}
		dirty={changed('connect')}
		expanded={expanded === 'connect'}
		ontoggle={toggleRow}
	>
		<div class="ft-span">
			<Input label="Backend-адрес (-connect)" bind:value={server.connect} placeholder="127.0.0.1:51820" />
			<p class="ft-hint" style="margin-top: 0.375rem">
				WireGuard — обычно 127.0.0.1:51820, Xray — 127.0.0.1:443
			</p>
		</div>
	</SettingRow>
	<SettingRow
		id="sobf"
		label="Обфускация"
		flag="-obf-profile / -obf-key"
		summary={`${server.obfProfile}${server.obfKey ? ' · ключ задан' : ''}`}
		dirty={changed('obfProfile', 'obfKey')}
		expanded={expanded === 'sobf'}
		ontoggle={toggleRow}
	>
		<Dropdown label="Профиль (-obf-profile)" bind:value={server.obfProfile} options={obfOptions} />
		<Input
			label="Ключ обфускации (-obf-key)"
			type="password"
			bind:value={server.obfKey}
			placeholder="64 hex-символа"
		/>
	</SettingRow>
	<SettingRow
		id="clientsFile"
		label="Allowlist клиентов"
		flag="-clients-file"
		summary={server.clientsFile ? 'вкл' : 'выкл'}
		dirty={changed('clientsFile')}
		expanded={expanded === 'clientsFile'}
		ontoggle={toggleRow}
	>
		<div class="ft-span">
			<Input
				label="Файл allowlist клиентов (-clients-file)"
				bind:value={server.clientsFile}
				placeholder="оставьте пустым — без проверки Client ID"
			/>
		</div>
	</SettingRow>
</SettingRows>

{#if genOpen}
	<div class="ft-panel-accent">
		<div class="section-label">Ссылка для клиента</div>
		<div class="ft-gen-row">
			<Input label="Провайдер" bind:value={genProvider} placeholder="vk" />
			<Input
				label="MTU"
				type="number"
				value={String(genMTU)}
				onchange={(v) => (genMTU = Number(v) || 1376)}
			/>
			<Button
				variant="primary"
				size="sm"
				loading={generating}
				onclick={() => onGenerate(genProvider, genMTU, genWG, genClientId, genName)}
			>
				Сгенерировать
			</Button>
		</div>
		<p class="ft-hint">
			Соберёт freeturn:// ссылку из обфускации/ключа сервера выше и внешнего IP роутера —
			передавайте её только доверенному получателю
		</p>
		{#if server.clientsFile}
			<p class="ft-hint">
				У сервера включён allowlist (-clients-file): без Client ID в ссылке (раздел ниже)
				сервер отклонит подключение получателя. Ссылка сама ничего не регистрирует —
				добавьте ID на сервере отдельно
			</p>
		{/if}

		<button type="button" class="ft-gen-more" onclick={() => (genMore = !genMore)}>
			{genMore ? '−' : '+'} Client ID и WireGuard-конфиг
		</button>
		{#if genMore}
			<div class="ft-gen-grid">
				<Input bind:value={genClientId} placeholder="Client ID — пусто, если allowlist не используется" />
				<Input bind:value={genName} placeholder="комментарий (например, имя получателя)" />
			</div>
			<div class="ft-gen-idrow">
				<Button variant="ghost" size="sm" onclick={randomClientId}>Сгенерировать ID</Button>
			</div>
			<textarea
				class="field-textarea ft-textarea"
				bind:value={genWG}
				placeholder="Вставьте сюда конфиг WireGuard-клиента, если хотите передать его вместе со ссылкой..."
			></textarea>
			<p class="ft-hint">
				Внимание: конфиг (включая приватный ключ WireGuard) вкладывается в ссылку в открытом виде
				(base64, без шифрования) — передавайте её только доверенному получателю по защищённому каналу
			</p>
		{/if}

		{#if generatedLink}
			<div class="ft-result">
				<div class="section-label">Готовая ссылка ({generatedPeer})</div>
				<div class="ft-link-box">{generatedLink}</div>
				<Button variant="ghost" size="sm" onclick={() => onCopy(generatedLink)}>
					Скопировать в буфер
				</Button>
				{#if generatedClientId && server.clientsFile}
					<p class="ft-hint" style="margin-top: 0.625rem">
						У сервера включён allowlist — прежде чем отдавать эту ссылку, зарегистрируйте
						Client ID <code>{generatedClientId}</code> в <code>{server.clientsFile}</code> по SSH:
						бинарь сервера умеет <code>clients add {generatedClientId} "{genName || 'client'}"</code>
						(укажите путь к файлу через переменную окружения <code>CLIENTS_FILE</code>, если бинарь
						запускается не из той же директории)
					</p>
				{/if}
			</div>
		{/if}
	</div>
{/if}

<style>
	.ft-panel-accent {
		padding: 0.875rem 1rem;
		background: var(--color-bg-secondary);
		border: 1px solid var(--color-accent-border);
		border-radius: var(--radius);
		margin-bottom: 0.625rem;
	}

	.ft-gen-row {
		display: grid;
		grid-template-columns: 1fr 100px auto;
		gap: 0.5rem;
		align-items: end;
		margin-bottom: 0.5rem;
	}

	.ft-gen-more {
		display: block;
		background: none;
		border: none;
		padding: 0;
		margin: 0.625rem 0 0;
		font: inherit;
		font-size: 0.75rem;
		color: var(--color-accent);
		cursor: pointer;
	}

	.ft-gen-more:hover {
		text-decoration: underline;
	}

	.ft-gen-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.75rem;
		margin-top: 0.625rem;
		margin-bottom: 0.5rem;
	}

	.ft-gen-idrow {
		display: flex;
		justify-content: flex-end;
		margin-bottom: 0.5rem;
	}

	.ft-span {
		grid-column: 1 / -1;
		min-width: 0;
	}

	.ft-hint {
		font-size: 0.75rem;
		color: var(--color-text-secondary);
		margin: 0;
	}

	/* Поверх глобального .field-textarea: mono + вертикальный resize. */
	.ft-textarea {
		min-height: 100px;
		font-family: var(--font-mono);
		resize: vertical;
		white-space: pre;
		margin: 0.375rem 0;
	}

	.ft-result {
		margin-top: 0.875rem;
		padding: 0.875rem;
		border-radius: var(--radius-sm);
		border: 1px solid var(--color-border);
		background: var(--color-bg-tertiary);
	}

	.ft-link-box {
		font-family: var(--font-mono);
		font-size: 0.8125rem;
		word-break: break-all;
		margin-bottom: 0.625rem;
	}

	@media (max-width: 640px) {
		.ft-gen-row,
		.ft-gen-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
