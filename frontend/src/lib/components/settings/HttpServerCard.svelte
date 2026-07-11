<!--
  Карточка «HTTP-сервер»: порт и интерфейсы, на которых слушает веб-интерфейс
  awg-manager. Применение ЖИВОЕ (без рестарта демона) с confirm-or-revert:

    1. POST /server/listen/change — бэкенд make-before-break перебиндивает
       листенеры и взводит откат (~2 мин); настройки ещё НЕ сохранены.
    2. Фронт уходит на новый origin, унося одноразовый токен в URL-фрагменте
       (#listen-confirm=...) — cookie сессии привязана к хосту и смену
       интерфейса не переживает, токен аутентифицирует confirm сам.
    3. Эта же карточка на новой странице настроек в onMount видит фрагмент,
       зовёт POST /server/listen/confirm — бэкенд гасит откат и персистит.
       Не дошли (адрес недоступен) — бэкенд сам откатывается к старому.

  Листенер 127.0.0.1 держится всегда (реверс-прокси NDMS, health-пробы,
  спасательный люк) — потому loopback в списке не предлагается.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores/notifications';
	import { ConfirmModal } from '$lib/components/ui';
	import type { RouterInterface, ServerListenState } from '$lib/types';

	let listen = $state<ServerListenState | null>(null);
	let ifaces = $state<RouterInterface[]>([]);
	let portDraft = $state('');
	let allIfaces = $state(true);
	let selected = $state<string[]>([]);
	let busy = $state(false);
	let confirmOpen = $state(false);

	const port = $derived(Number(portDraft.trim()));
	const portValid = $derived(Number.isInteger(port) && port >= 1 && port <= 65535);
	const dirty = $derived.by(() => {
		if (!listen) return false;
		const curAll = listen.interfaces.length === 0;
		if (portValid && port !== listen.port) return true;
		if (allIfaces !== curAll) return true;
		if (!allIfaces && [...selected].sort().join(',') !== [...listen.interfaces].sort().join(',')) return true;
		return false;
	});
	const applyDisabled = $derived(busy || !portValid || !dirty || (!allIfaces && selected.length === 0));

	async function load(): Promise<void> {
		listen = await api.serverListenState();
		portDraft = String(listen.port);
		allIfaces = listen.interfaces.length === 0;
		selected = [...listen.interfaces];
	}

	onMount(async () => {
		// Токен подтверждения из URL-фрагмента — мы только что приехали со
		// старого origin после живой смены адреса.
		const m = window.location.hash.match(/listen-confirm=([0-9a-f]{16,})/);
		if (m) {
			history.replaceState(null, '', window.location.pathname + window.location.search);
			try {
				await api.serverListenConfirm(m[1]);
				notifications.success('Новый адрес HTTP-сервера подтверждён и сохранён');
			} catch (e) {
				notifications.error(
					`Не удалось подтвердить смену адреса — бэкенд откатится на старый: ${e instanceof Error ? e.message : String(e)}`,
				);
			}
		}
		try {
			await load();
		} catch {
			listen = null;
		}
		try {
			ifaces = (await api.getAllInterfaces()).filter((i) => i.name !== 'lo');
		} catch {
			ifaces = [];
		}
	});

	function toggleIface(name: string): void {
		selected = selected.includes(name) ? selected.filter((n) => n !== name) : [...selected, name];
	}

	// Куда редиректить после живого перебинда: порт-only смена — тот же хост;
	// смена интерфейсов — первый не-loopback адрес из фактически забинженных
	// (0.0.0.0 покрывает текущий хост — тоже оставляем его).
	function redirectTarget(newPort: number, boundAddrs: string[]): string {
		const cur = window.location;
		const sameIfaces = listen
			? (allIfaces && listen.interfaces.length === 0) ||
				(!allIfaces && [...selected].sort().join(',') === [...listen.interfaces].sort().join(','))
			: true;
		let host = cur.hostname;
		if (!sameIfaces && !allIfaces) {
			const ext = boundAddrs.find((a) => !a.startsWith('127.') && !a.startsWith('0.0.0.0'));
			if (ext) host = ext.slice(0, ext.lastIndexOf(':'));
		}
		return `${cur.protocol}//${host}:${newPort}`;
	}

	async function apply(): Promise<void> {
		if (applyDisabled) return;
		busy = true;
		try {
			const res = await api.serverListenChange(port, allIfaces ? [] : selected);
			const target = redirectTarget(port, res.boundAddrs);
			notifications.success('Адрес применён — переходим на новый и подтверждаем…');
			window.location.href = `${target}/settings#listen-confirm=${res.confirmToken}`;
		} catch (e) {
			notifications.error(`Не удалось сменить адрес: ${e instanceof Error ? e.message : String(e)}`);
			busy = false;
		}
	}
</script>

{#if listen}
	<div class="hs-body">
		{#if listen.pendingConfirm}
			<div class="pending">
				Предыдущая смена адреса ожидает подтверждения (откат в {new Date(listen.confirmDeadline ?? '').toLocaleTimeString()}).
			</div>
		{/if}

		<div class="row">
			<label class="lbl" for="hs-port">Порт</label>
			<input id="hs-port" class="port" type="number" min="1" max="65535" bind:value={portDraft} />
			{#if portDraft !== '' && !portValid}
				<span class="err">1–65535</span>
			{:else if portValid && port < 1024}
				<span class="warn-inline">порты &lt;1024 могут конфликтовать с системными сервисами</span>
			{/if}
		</div>

		<div class="row">
			<span class="lbl">Интерфейсы</span>
			<div class="chips">
				<button type="button" class="chip" class:active={allIfaces} onclick={() => (allIfaces = true)}>
					<span class="chip-label">Все интерфейсы</span>
					<span class="chip-desc">0.0.0.0</span>
				</button>
				{#each ifaces as i (i.name)}
					<button
						type="button"
						class="chip"
						class:active={!allIfaces && selected.includes(i.name)}
						onclick={() => {
							allIfaces = false;
							toggleIface(i.name);
						}}
					>
						<span class="chip-label">{i.name}</span>
						{#if i.label}<span class="chip-desc">{i.label}</span>{/if}
					</button>
				{/each}
			</div>
		</div>

		<p class="hint">
			Сейчас слушает: <code>{listen.boundAddrs.join(', ')}</code>. Применяется без перезапуска: вы будете
			перенаправлены на новый адрес; если он не откроется — через ~2 минуты вернётся старый.
			Loopback (127.0.0.1) остаётся всегда — реверс-прокси KeenDNS и health-проверки работают при любом выборе.
		</p>

		<div class="actions">
			<button type="button" class="apply" disabled={applyDisabled} onclick={() => (confirmOpen = true)}>
				{busy ? 'Применяем…' : 'Применить'}
			</button>
		</div>
	</div>

	<ConfirmModal
		open={confirmOpen}
		title="Сменить адрес HTTP-сервера"
		message={`Веб-интерфейс переедет на порт ${portValid ? port : '?'}${allIfaces ? ' (все интерфейсы)' : ` (${selected.join(', ')})`}. Вы будете перенаправлены на новый адрес для подтверждения; без подтверждения через ~2 минуты вернётся текущий адрес.`}
		busy={busy}
		onConfirm={() => {
			confirmOpen = false;
			void apply();
		}}
		onClose={() => {
			if (!busy) confirmOpen = false;
		}}
	/>
{:else}
	<p class="hint">Состояние HTTP-сервера недоступно.</p>
{/if}

<style>
	.hs-body {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.pending {
		padding: 0.5rem 0.75rem;
		font-size: 0.8125rem;
		color: var(--color-warning, #d97706);
		background: color-mix(in srgb, var(--color-warning, #d97706) 10%, transparent);
		border-left: 2px solid var(--color-warning, #d97706);
		border-radius: var(--radius-sm, 6px);
	}
	.row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}
	.lbl {
		min-width: 6.5rem;
		color: var(--text-secondary);
		font-size: 0.875rem;
	}
	.port {
		width: 7rem;
		padding: 0.4rem 0.6rem;
		background: var(--color-bg-tertiary, var(--bg-tertiary));
		border: 1px solid var(--color-border, var(--border));
		border-radius: var(--radius-sm, 6px);
		color: var(--text-primary);
		font-family: var(--font-mono);
	}
	.err {
		color: var(--color-error, #e06a5a);
		font-size: 0.8125rem;
	}
	.warn-inline {
		color: var(--color-warning, #d97706);
		font-size: 0.8125rem;
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}
	.chip {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 0.1rem;
		padding: 0.4rem 0.7rem;
		background: var(--color-bg-tertiary, var(--bg-tertiary));
		border: 1px solid var(--color-border, var(--border));
		border-radius: var(--radius-sm, 8px);
		cursor: pointer;
		text-align: left;
	}
	.chip.active {
		border-color: var(--color-accent, var(--accent));
		background: color-mix(in srgb, var(--color-accent, var(--accent)) 12%, transparent);
	}
	.chip-label {
		color: var(--text-primary);
		font-size: 0.8125rem;
		font-weight: 600;
		font-family: var(--font-mono);
	}
	.chip-desc {
		color: var(--text-muted);
		font-size: 0.6875rem;
	}
	.hint {
		color: var(--text-muted);
		font-size: 0.8125rem;
		line-height: 1.45;
		margin: 0;
	}
	.hint code {
		font-family: var(--font-mono);
		color: var(--text-secondary);
	}
	.actions {
		display: flex;
		justify-content: flex-end;
	}
	.apply {
		padding: 0.45rem 1rem;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--color-bg-secondary, #0a0a0a);
		background: var(--color-accent, var(--accent));
		border: none;
		border-radius: var(--radius-sm, 8px);
		cursor: pointer;
	}
	.apply:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
</style>
