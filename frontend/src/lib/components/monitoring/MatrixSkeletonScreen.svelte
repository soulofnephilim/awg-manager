<script lang="ts">
	const COLS = 3;
	const ROWS = 4;
	const cols = Array.from({ length: COLS });
	const rows = Array.from({ length: ROWS });
</script>

<!-- Status strip skeleton -->
<div class="strip">
	{#each Array.from({ length: 4 }) as _}
		<div class="tile">
			<span class="skel skel-value"></span>
			<span class="skel skel-label"></span>
		</div>
	{/each}
</div>

<!-- Matrix table skeleton -->
<div class="wrap">
	<table class="matrix">
		<thead>
			<tr>
				<th class="th-target">
					<span class="skel skel-th-target"></span>
				</th>
				{#each cols as _}
					<th class="th-tunnel">
						<span class="skel skel-th-tunnel"></span>
					</th>
				{/each}
			</tr>
		</thead>
		<tbody>
			{#each rows as _, ri}
				<tr>
					<td class="td-target">
						<span class="skel skel-target-name" style="animation-delay:{ri * 60}ms"></span>
						<span class="skel skel-target-host" style="animation-delay:{ri * 60 + 30}ms"></span>
					</td>
					{#each cols as _, ci}
						<td class="td-cell">
							<span class="skel skel-cell" style="animation-delay:{(ri * COLS + ci) * 40}ms"></span>
						</td>
					{/each}
				</tr>
			{/each}
		</tbody>
	</table>
</div>

<style>
	@keyframes pulse {
		0%, 100% { opacity: 0.5; }
		50% { opacity: 0.18; }
	}

	.skel {
		display: block;
		border-radius: 4px;
		background: var(--color-border);
		animation: pulse 1.4s ease-in-out infinite;
	}

	/* Strip */
	.strip {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 0.75rem;
		margin-bottom: 1rem;
	}

	.tile {
		background: var(--color-bg-secondary);
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		padding: 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.skel-value {
		width: 48px;
		height: 28px;
	}

	.skel-label {
		width: 72px;
		height: 10px;
	}

	/* Table */
	.wrap {
		overflow-x: auto;
	}

	.matrix {
		border-collapse: separate;
		border-spacing: 0.375rem;
		width: 100%;
	}

	.th-target,
	.th-tunnel {
		padding: 0.4375rem 0.5rem;
		background: var(--color-bg-tertiary);
		border-bottom: 1px solid var(--color-border);
		text-align: left;
	}

	.th-tunnel {
		min-width: 100px;
		text-align: center;
	}

	.skel-th-target {
		width: 48px;
		height: 10px;
		animation-delay: 0ms;
	}

	.skel-th-tunnel {
		width: 64px;
		height: 10px;
		margin: 0 auto;
	}

	.td-target {
		padding: 0.375rem 0.5rem;
		background: var(--color-bg-secondary);
		position: sticky;
		left: 0;
		min-width: 160px;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.td-cell {
		padding: 0.125rem;
		text-align: center;
	}

	.skel-target-name {
		width: 90px;
		height: 12px;
	}

	.skel-target-host {
		width: 64px;
		height: 10px;
	}

	.skel-cell {
		display: inline-block;
		width: 84px;
		height: 32px;
		border-radius: var(--radius-sm);
	}

	@media (max-width: 768px) {
		.strip {
			grid-template-columns: repeat(2, 1fr);
		}
	}
</style>
