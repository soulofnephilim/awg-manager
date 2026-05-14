<script lang="ts">
	import type { ManagedServerBackupFile } from '$lib/types';

	interface Props {
		open: boolean;
		file: ManagedServerBackupFile;
		onclose: () => void;
	}
	let { open = $bindable(false), file: _file, onclose }: Props = $props();
	// Suppress "unused prop" warning during stub phase.
	void _file;
	void onclose;
</script>

<!-- TODO Task 14: replace this stub with the full import-results modal. -->
{#if open}{/if}
