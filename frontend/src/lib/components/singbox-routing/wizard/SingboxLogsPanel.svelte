<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { api } from '$lib/api/client';
	import type { LogEntry } from '$lib/types';

	interface Props {
		bufferSize?: number;
	}
	let { bufferSize = 50 }: Props = $props();

	let lines = $state<LogEntry[]>([]);
	let preEl: HTMLPreElement | undefined = $state();

	function autoScroll(): void {
		if (!preEl) return;
		preEl.scrollTop = preEl.scrollHeight;
	}

	async function refresh(): Promise<void> {
		try {
			const resp = await api.getLogs({ bucket: 'singbox', limit: bufferSize });
			lines = resp.logs.slice(-bufferSize);
			await tick();
			autoScroll();
		} catch {
			// transient — keep last known lines
		}
	}

	onMount(() => {
		void refresh();
		const interval = setInterval(refresh, 1500);
		return () => clearInterval(interval);
	});

	function fmtLine(e: LogEntry): string {
		const t = e.timestamp.length > 19 ? e.timestamp.slice(11, 19) : e.timestamp;
		const sub = e.subgroup ? `${e.group}/${e.subgroup}` : e.group;
		return `${t} [${e.level}] ${sub}: ${e.message}`;
	}
</script>

<pre bind:this={preEl} class="logs">
{#each lines as l, i (i)}{fmtLine(l)}{'\n'}{/each}{#if lines.length === 0}(нет логов sing-box за последнее время){/if}
</pre>

<style>
	.logs {
		font-family: ui-monospace, monospace;
		font-size: 0.78rem;
		line-height: 1.4;
		background: var(--color-bg-primary);
		border: 1px solid var(--color-border);
		border-radius: 4px;
		padding: 0.5rem 0.75rem;
		max-height: 300px;
		overflow-y: auto;
		white-space: pre-wrap;
		color: var(--color-text-secondary);
		margin: 0;
	}
</style>
