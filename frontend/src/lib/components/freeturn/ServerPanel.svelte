<script lang="ts">
	import { Card, Input, Button, Dropdown } from '$lib/components/ui';
	import type { FreeTurnServerConfig, FreeTurnProcessStatus } from '$lib/types';
	import ProcessCard from './ProcessCard.svelte';
	import ProcessStatsRow from './ProcessStatsRow.svelte';

	interface Props {
		server: FreeTurnServerConfig;
		status?: FreeTurnProcessStatus;
		saving: boolean;
		installAvailable: boolean;
		installVersion?: string;
		installing: boolean;
		onInstall: () => void;
		generating: boolean;
		generatedLink: string;
		generatedPeer: string;
		genProvider: string;
		genMTU: number;
		genWG: string;
		onToggle: (on: boolean) => void;
		onSave: () => void;
		onGenerate: (provider: string, mtu: number, wg: string) => void;
		onCopy: (text: string) => void;
	}

	let {
		server,
		status,
		saving,
		installAvailable,
		installVersion,
		installing,
		onInstall,
		generating,
		generatedLink,
		generatedPeer,
		genProvider = $bindable(),
		genMTU = $bindable(),
		genWG = $bindable(),
		onToggle,
		onSave,
		onGenerate,
		onCopy
	}: Props = $props();

	const modeOptions = [
		{ value: 'udp', label: 'udp' },
		{ value: 'tcp', label: 'tcp' }
	];
	const obfOptions = [
		{ value: 'none', label: 'none' },
		{ value: 'rtpopus', label: 'rtpopus' },
		{ value: 'rtpopus2', label: 'rtpopus2' },
		{ value: 'rtpopus3', label: 'rtpopus3' }
	];
</script>

<ProcessStatsRow {status} mode={server.mode} obfProfile={server.obfProfile} />

<ProcessCard
	title="FreeTurn сервер"
	{status}
	{saving}
	{installAvailable}
	{installVersion}
	{installing}
	{onInstall}
	{onToggle}
	{onSave}
>
	<div class="ft-section-label" style="margin-top: 0">Приём подключений</div>
	<div class="ft-grid-2">
		<Input label="Слушать (-listen)" bind:value={server.listen} placeholder="0.0.0.0:56000" />
		<Dropdown label="Режим (-mode)" bind:value={server.mode} options={modeOptions} />
	</div>

	<div class="ft-section-label">Куда форвардить</div>
	<Input label="Backend-адрес (-connect)" bind:value={server.connect} placeholder="127.0.0.1:51820" />
	<p class="ft-hint">WireGuard — обычно 127.0.0.1:51820, Xray — 127.0.0.1:443</p>

	<div class="ft-section-label">Обфускация и доступ</div>
	<div class="ft-grid-2">
		<Dropdown label="Профиль (-obf-profile)" bind:value={server.obfProfile} options={obfOptions} />
		<Input
			label="Ключ обфускации (-obf-key)"
			type="password"
			bind:value={server.obfKey}
			placeholder="64 hex-символа"
		/>
	</div>
	<Input
		label="Файл allowlist клиентов (-clients-file)"
		bind:value={server.clientsFile}
		placeholder="оставьте пустым — без проверки Client ID"
	/>
</ProcessCard>

<Card>
	{#snippet header()}
		<div class="ft-card-title">Ссылка для клиента</div>
	{/snippet}
	<p class="ft-hint" style="margin-top: 0">
		Соберёт freeturn:// ссылку из обфускации/ключа сервера выше и внешнего IP роутера —
		вставьте её в клиентскую панель или в приложение
	</p>
	<div class="ft-grid-2">
		<Input label="Провайдер" bind:value={genProvider} placeholder="vk" />
		<Input label="MTU" type="number" value={String(genMTU)} onchange={(v) => (genMTU = Number(v) || 1376)} />
	</div>
	<div class="ft-section-label">WireGuard-конфиг клиента (опционально)</div>
	<textarea
		class="ft-textarea"
		bind:value={genWG}
		placeholder="Вставьте сюда конфиг WireGuard-клиента, если хотите передать его вместе со ссылкой..."
	></textarea>
	<p class="ft-hint">
		Внимание: конфиг (включая приватный ключ WireGuard) вкладывается в ссылку в открытом виде
		(base64, без шифрования) — передавайте её только доверенному получателю по защищённому каналу
	</p>

	<div class="ft-footer">
		<Button variant="primary" size="sm" loading={generating} onclick={() => onGenerate(genProvider, genMTU, genWG)}>
			Сгенерировать ссылку
		</Button>
	</div>

	{#if generatedLink}
		<div class="ft-result">
			<div class="ft-section-label" style="margin-top: 0">Готовая ссылка ({generatedPeer})</div>
			<div class="ft-link-box">{generatedLink}</div>
			<Button variant="ghost" size="sm" onclick={() => onCopy(generatedLink)}>Скопировать в буфер</Button>
		</div>
	{/if}
</Card>

<style>
	.ft-section-label {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--color-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.03em;
		margin: 1.25rem 0 0.625rem;
	}

	.ft-card-title {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: 500;
	}

	.ft-grid-2 {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
	}

	.ft-hint {
		font-size: 0.75rem;
		color: var(--color-text-secondary);
		margin: -0.5rem 0 0.75rem;
	}

	.ft-textarea {
		width: 100%;
		min-height: 100px;
		padding: 0.5rem 0.625rem;
		border-radius: var(--radius-sm);
		border: 1px solid var(--color-border);
		background: var(--color-bg-tertiary);
		color: var(--color-text-primary);
		font-family: monospace;
		font-size: 0.8125rem;
		resize: vertical;
		white-space: pre;
		margin-bottom: 0.75rem;
	}

	.ft-footer {
		display: flex;
		justify-content: flex-end;
	}

	.ft-result {
		margin-top: 1rem;
		padding: 0.875rem;
		border-radius: var(--radius-sm);
		border: 1px solid var(--color-border);
		background: var(--color-bg-tertiary);
	}

	.ft-link-box {
		font-family: monospace;
		font-size: 0.8125rem;
		word-break: break-all;
		margin-bottom: 0.625rem;
	}

	@media (max-width: 640px) {
		.ft-grid-2 {
			grid-template-columns: 1fr;
		}
	}
</style>
