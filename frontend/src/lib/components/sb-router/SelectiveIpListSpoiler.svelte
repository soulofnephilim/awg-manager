<!--
  Сворачиваемый список IP/CIDR для снимка ipset.
  Используется в SelectiveIpsetSnapshot (модалка пересборки и «Содержимое ipset»).
-->
<script lang="ts">
  interface Props {
    items?: string[] | null;
    /** Текст в заголовке спойлера, напр. «179 подсетей» или «12 IP». */
    summary: string;
    /** Раскрыт по умолчанию (для коротких списков при отладке). */
    defaultOpen?: boolean;
  }
  let { items, summary, defaultOpen = false }: Props = $props();

  const safeItems = $derived(items ?? []);
</script>

{#if safeItems.length === 0}
  <span class="empty">—</span>
{:else}
  <details class="ip-spoiler" open={defaultOpen || undefined}>
    <summary>{summary}</summary>
    <ul class="chip-list">
      {#each safeItems as item (item)}
        <li>{item}</li>
      {/each}
    </ul>
  </details>
{/if}

<style>
  .empty {
    font-size: 11px;
    color: var(--text-muted);
  }
  .ip-spoiler {
    margin: 0;
  }
  .ip-spoiler summary {
    cursor: pointer;
    font-size: 11.5px;
    font-weight: 500;
    color: var(--color-accent, var(--accent));
    user-select: none;
    list-style: none;
    width: fit-content;
  }
  .ip-spoiler summary::-webkit-details-marker {
    display: none;
  }
  .ip-spoiler summary::before {
    content: '▸ ';
    display: inline-block;
    transition: transform 0.15s ease;
    color: var(--text-muted);
  }
  .ip-spoiler[open] summary::before {
    transform: rotate(90deg);
  }
  .chip-list {
    list-style: none;
    margin: 6px 0 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    max-height: 12rem;
    overflow-y: auto;
  }
  .chip-list li {
    font-family: var(--font-mono);
    font-size: 11px;
    padding: 2px 6px;
    border-radius: 3px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
  }
</style>
