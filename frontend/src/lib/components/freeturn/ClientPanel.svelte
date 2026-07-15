<script lang="ts">
	import { Card, Input, Button, Dropdown, FormToggle } from '$lib/components/ui';
	import type { FreeTurnClientConfig, FreeTurnProcessStatus } from '$lib/types';
	import ProcessCard from './ProcessCard.svelte';
	import ProcessStatsRow from './ProcessStatsRow.svelte';

	interface Props {
		client: FreeTurnClientConfig;
		status?: FreeTurnProcessStatus;
		saving: boolean;
		routerHost: string;
		importing: boolean;
		importedWG: string | null;
		onToggle: (on: boolean) => void;
		onSave: () => void;
		onImport: (link: string) => void;
		onCopy: (text: string) => void;
	}

	let {
		client,
		status,
		saving,
		routerHost,
		importing,
		importedWG,
		onToggle,
		onSave,
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
</script>

<ProcessStatsRow {status} mode={client.mode} transport={client.transport} obfProfile={client.obfProfile} />

<ProcessCard title="FreeTurn клиент" {status} {saving} {onToggle} {onSave}>
	<div class="ft-section-label" style="margin-top: 0">TURN-сервер</div>
	<div class="ft-grid-2">
		<Input label="Адрес сервера (-peer)" bind:value={client.peer} placeholder="vinvanvlad.com:56000" />
		<Input label="Провайдер (-provider)" bind:value={client.provider} placeholder="vk" />
	</div>

	<div class="ft-section-label">Провайдер VK</div>
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
	<div class="ft-grid-2">
		<Input
			label="Потоков на кред (-streams-per-cred)"
			type="number"
			value={String(client.streamsPerCred)}
			onchange={(v) => (client.streamsPerCred = Number(v) || 0)}
		/>
		<div class="ft-toggle-slot">
			<FormToggle bind:checked={client.manualCaptcha} label="Ручная капча (-manual-captcha)" />
		</div>
	</div>
	{#if client.manualCaptcha}
		<p class="ft-hint">
			Капча решается локальным HTTP-сервером самого freeturn-client на роутере
			(127.0.0.1:8765) — снаружи он недоступен. Пробросьте порт с вашего ПК:
			<code>ssh -N -L 8765:127.0.0.1:8765 root@{routerHost || '<IP роутера>'}</code>
			и откройте <code>http://127.0.0.1:8765</code> в браузере (порт SSH может отличаться
			от 22 — на Keenetic часто 222).
		</p>
	{/if}

	<div class="ft-section-label">Туннелирование</div>
	<div class="ft-grid-3">
		<Dropdown label="Режим (-mode)" bind:value={client.mode} options={modeOptions} />
		<Dropdown label="Транспорт до TURN (-transport)" bind:value={client.transport} options={transportOptions} />
		<Input label="Локальный адрес (-listen)" bind:value={client.listen} placeholder="127.0.0.1:9000" />
	</div>
	<div class="ft-grid-2">
		<Input
			label="Потоков TURN (-n)"
			type="number"
			value={String(client.streams)}
			onchange={(v) => (client.streams = Number(v) || 0)}
		/>
		<div class="ft-toggle-slot">
			<FormToggle bind:checked={client.bond} label="Бондинг через все smux-сессии (-bond, только mode=tcp)" />
		</div>
	</div>

	<div class="ft-section-label">Обфускация</div>
	<div class="ft-grid-2">
		<Dropdown label="Профиль (-obf-profile)" bind:value={client.obfProfile} options={obfOptions} />
		<Input
			label="Ключ обфускации (-obf-key)"
			type="password"
			bind:value={client.obfKey}
			placeholder="64 hex-символа"
		/>
	</div>
</ProcessCard>

<!-- Импорт нужен один раз при настройке — компактно ПОД карточкой клиента,
     чтобы при повторных визитах первым был статус/тумблер. -->
<Card variant="nested" padding="sm">
	<div class="ft-section-label" style="margin-top: 0">Импорт по ссылке freeturn://</div>
	<div class="ft-import-row">
		<Input bind:value={importLink} placeholder="freeturn://..." />
		<Button variant="secondary" size="sm" loading={importing} onclick={() => onImport(importLink)}>
			Применить
		</Button>
	</div>
	<p class="ft-hint">
		Заполнит адрес сервера, провайдера и обфускацию выше (сохранение — кнопкой «Сохранить») и,
		если в ссылке есть WireGuard-конфиг, сразу создаст из него туннель во вкладке «AWG»
	</p>
	{#if importedWG}
		<div class="ft-section-label">WireGuard-конфиг из ссылки</div>
		<textarea class="ft-textarea" readonly value={importedWG}></textarea>
		<Button variant="ghost" size="sm" onclick={() => onCopy(importedWG!)}>Скопировать конфиг</Button>
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

	.ft-toggle-slot {
		display: flex;
		align-items: flex-end;
		padding-bottom: 0.25rem;
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

	.ft-import-row {
		display: grid;
		grid-template-columns: 1fr auto;
		gap: 0.5rem;
		align-items: center;
		margin-bottom: 0.5rem;
	}

	@media (max-width: 640px) {
		.ft-grid-2,
		.ft-grid-3 {
			grid-template-columns: 1fr;
		}
	}
</style>
