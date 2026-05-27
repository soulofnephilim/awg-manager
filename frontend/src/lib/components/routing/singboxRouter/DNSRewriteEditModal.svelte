<script lang="ts">
	import Modal from '$lib/components/ui/Modal.svelte';
	import { Button } from '$lib/components/ui';
	import type { SingboxRouterDNSRewrite } from '$lib/types';

	interface Props {
		rewrite?: SingboxRouterDNSRewrite;
		onClose: () => void;
		onSave: (rewrite: SingboxRouterDNSRewrite) => Promise<void> | void;
	}
	let { rewrite, onClose, onSave }: Props = $props();

	// svelte-ignore state_referenced_locally
	let pattern = $state(rewrite?.pattern ?? '');
	// svelte-ignore state_referenced_locally
	let ipsStr = $state((rewrite?.ips ?? []).join(', '));
	let busy = $state(false);
	let error = $state('');

	async function save(): Promise<void> {
		busy = true;
		error = '';
		try {
			const p = pattern.trim();
			if (!p) { error = 'Шаблон обязателен'; busy = false; return; }
			const ips = ipsStr.split(',').map((s) => s.trim()).filter(Boolean);
			if (ips.length === 0) { error = 'Укажите хотя бы один IP'; busy = false; return; }
			await onSave({ pattern: p, ips });
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy = false;
		}
	}
</script>

<Modal open onclose={onClose} title={rewrite ? 'Редактировать перезапись' : 'Новая перезапись'} size="md">
	<div class="form">
		<label class="field">
			<div class="lbl">Шаблон домена</div>
			<input class="mono" bind:value={pattern} placeholder="nas.lan · *.discord.media · finland10*.discord.media" />
			<div class="hint">
				Без <code>*</code> — точный домен. <code>*.suffix</code> — все поддомены.
				<code>prefix*.suffix</code> — wildcard внутри первой метки (нужен доменный хвост после <code>*</code>).
			</div>
		</label>
		<label class="field">
			<div class="lbl">IP-адреса (через запятую)</div>
			<input class="mono" bind:value={ipsStr} placeholder="104.25.158.178, fd00::5" />
		</label>
		{#if error}<div class="error">{error}</div>{/if}
	</div>

	{#snippet actions()}
		<Button variant="ghost" size="md" onclick={onClose} type="button">Отмена</Button>
		<Button variant="primary" size="md" onclick={save} disabled={busy} loading={busy} type="button">Сохранить</Button>
	{/snippet}
</Modal>

<style>
	.form { display: flex; flex-direction: column; gap: 0.75rem; }
	.field { display: grid; gap: 0.25rem; }
	.lbl { font-size: 0.75rem; color: var(--muted-text); }
	.field input { background: var(--bg); border: 1px solid var(--border); padding: 0.4rem 0.6rem; border-radius: 4px; color: var(--text); font-size: 0.85rem; box-sizing: border-box; width: 100%; }
	.mono { font-family: ui-monospace, monospace; }
	.hint { font-size: 0.75rem; color: var(--muted-text); line-height: 1.4; }
	.hint code { background: var(--bg); padding: 0.05rem 0.25rem; border-radius: 2px; font-family: ui-monospace, monospace; }
	.error { color: var(--danger, #dc2626); font-size: 0.85rem; }
</style>
