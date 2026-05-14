<script lang="ts">
	import { Button, Modal } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores/notifications';
	import ManagedServerImportModal from './ManagedServerImportModal.svelte';
	import type { ManagedServerBackupFile } from '$lib/types';

	let exportModalOpen = $state(false);
	let importModalOpen = $state(false);
	let pendingFile = $state<ManagedServerBackupFile | null>(null);
	let exporting = $state(false);

	function startExport() {
		exportModalOpen = true;
	}

	async function confirmExport() {
		exporting = true;
		try {
			const data = await api.managedServerExport();
			const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			const date = new Date().toISOString().slice(0, 10);
			a.href = url;
			a.download = `managed-backup-${date}.json`;
			document.body.appendChild(a);
			a.click();
			document.body.removeChild(a);
			URL.revokeObjectURL(url);
			exportModalOpen = false;
		} catch (e) {
			notifications.error((e as Error).message);
		} finally {
			exporting = false;
		}
	}

	function openFilePicker() {
		const input = document.createElement('input');
		input.type = 'file';
		input.accept = 'application/json';
		input.onchange = async () => {
			const file = input.files?.[0];
			if (!file) return;
			try {
				const text = await file.text();
				const parsed = JSON.parse(text) as ManagedServerBackupFile;
				if (parsed.type !== 'awg-manager-managed-server-backup') {
					notifications.error('Это не файл резервной копии awg-manager.');
					return;
				}
				pendingFile = parsed;
				importModalOpen = true;
			} catch (e) {
				notifications.error('Не удалось прочитать файл: ' + (e as Error).message);
			}
		};
		input.click();
	}
</script>

<div class="backup-toolbar">
	<Button variant="secondary" size="sm" onclick={startExport}>Экспорт</Button>
	<Button variant="secondary" size="sm" onclick={openFilePicker}>Импорт</Button>
</div>

<Modal
	bind:open={exportModalOpen}
	title="Экспорт резервной копии"
	size="sm"
	onclose={() => (exportModalOpen = false)}
>
	<p>Файл будет содержать приватные ключи сервера и пиров. Храните его в безопасном месте.</p>
	{#snippet actions()}
		<Button variant="secondary" size="md" onclick={() => (exportModalOpen = false)}>Отмена</Button>
		<Button variant="outline-primary" size="md" onclick={confirmExport} loading={exporting}>Скачать</Button>
	{/snippet}
</Modal>

{#if importModalOpen && pendingFile}
	<ManagedServerImportModal
		bind:open={importModalOpen}
		file={pendingFile}
		onclose={() => {
			importModalOpen = false;
			pendingFile = null;
		}}
	/>
{/if}

<style>
	.backup-toolbar {
		display: flex;
		gap: 0.5rem;
	}
</style>
