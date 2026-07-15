<script lang="ts">
	import { Card, Stat } from '$lib/components/ui';
	import type { FreeTurnProcessStatus } from '$lib/types';
	import { formatUptime } from './uptime';

	interface Props {
		status?: FreeTurnProcessStatus;
		/** Режим туннеля (-mode) и транспорт (-transport, только клиент). */
		mode: string;
		transport?: string;
		obfProfile: string;
	}

	let { status, mode, transport, obfProfile }: Props = $props();
</script>

<!-- Плитки всегда видимы (прочерки, когда процесс остановлен) — паттерн
     DeviceProxyStatRow: состояние читается с одного взгляда сверху. -->
<Card variant="nested" padding="sm">
	<div class="ft-stats">
		<Stat
			value={status?.running ? 'запущен' : 'остановлен'}
			label="статус"
			sub={status?.running ? formatUptime(status.startedAt) : undefined}
		/>
		<Stat value={status?.running && status.pid ? String(status.pid) : '—'} label="PID" />
		<Stat value={transport ? `${mode} / ${transport}` : mode} label={transport ? 'режим / транспорт' : 'режим'} />
		<Stat value={obfProfile || 'none'} label="обфускация" />
	</div>
</Card>

<style>
	.ft-stats {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 0.5rem;
	}

	@media (max-width: 640px) {
		.ft-stats {
			grid-template-columns: repeat(2, 1fr);
		}
	}
</style>
