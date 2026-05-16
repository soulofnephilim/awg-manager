<script lang="ts" module>
    export interface ChipOption {
        value: string;
        label?: string;
    }
</script>

<script lang="ts">
    import { tick } from 'svelte';

    interface Props {
        values: string[];
        options: ChipOption[];
        onchange: (next: string[]) => void;
        placeholder?: string;
        allowOrphans?: boolean;
        disabled?: boolean;
    }

    let {
        values,
        options,
        onchange,
        placeholder = 'Не выбрано',
        allowOrphans = false,
        disabled = false,
    }: Props = $props();

    let open = $state(false);
    let triggerEl = $state<HTMLButtonElement | null>(null);
    let containerEl = $state<HTMLDivElement | null>(null);
    let panelEl = $state<HTMLDivElement | null>(null);
    let panelTop = $state(0);
    let panelLeft = $state(0);
    let panelWidth = $state(0);

    const selectedSet = $derived(new Set(values));
    const orphanValues = $derived(
        allowOrphans ? values.filter((v) => !options.find((o) => o.value === v)) : [],
    );
    const knownChips = $derived(
        values
            .map((v) => options.find((o) => o.value === v))
            .filter((o): o is ChipOption => o !== undefined),
    );
    const dropdownItems = $derived(options.filter((o) => !selectedSet.has(o.value)));
    const allSelected = $derived(dropdownItems.length === 0);

    function portal(node: HTMLElement, target: HTMLElement = document.body) {
        target.appendChild(node);
        return {
            destroy() {
                if (node.parentNode === target) {
                    target.removeChild(node);
                }
            },
        };
    }

    function recomputePlacement() {
        const el = containerEl ?? triggerEl;
        if (!el) return;
        const r = el.getBoundingClientRect();
        panelTop = r.bottom + 4;
        panelLeft = r.left;
        panelWidth = r.width;
    }

    async function toggleOpen() {
        if (disabled || allSelected) return;
        open = !open;
        if (open) {
            await tick();
            recomputePlacement();
        }
    }

    function addValue(v: string) {
        if (selectedSet.has(v)) return;
        onchange([...values, v]);
    }

    function removeValue(v: string) {
        onchange(values.filter((x) => x !== v));
    }

    function handleOutsideClick(e: MouseEvent) {
        if (!open) return;
        const target = e.target as Node | null;
        if (panelEl?.contains(target as Node)) return;
        if (triggerEl?.contains(target as Node)) return;
        open = false;
    }

    function handleScroll() {
        if (open) recomputePlacement();
    }

    function handleKeydown(e: KeyboardEvent) {
        if (e.key === 'Escape' && open) {
            open = false;
            triggerEl?.focus();
        }
    }

    $effect(() => {
        if (!open) return;
        document.addEventListener('mousedown', handleOutsideClick);
        document.addEventListener('keydown', handleKeydown);
        window.addEventListener('scroll', handleScroll, true);
        window.addEventListener('resize', handleScroll);
        return () => {
            document.removeEventListener('mousedown', handleOutsideClick);
            document.removeEventListener('keydown', handleKeydown);
            window.removeEventListener('scroll', handleScroll, true);
            window.removeEventListener('resize', handleScroll);
        };
    });
</script>

<div class="picker">
    <div class="chips" bind:this={containerEl}>
        {#if values.length === 0}
            <span class="placeholder">{placeholder}</span>
        {/if}
        {#each knownChips as opt (opt.value)}
            <span class="chip">
                <span class="chip-label">{opt.label ?? opt.value}</span>
                <button
                    type="button"
                    class="chip-remove"
                    aria-label="Удалить"
                    onclick={() => removeValue(opt.value)}
                    {disabled}
                >×</button>
            </span>
        {/each}
        {#each orphanValues as v (v)}
            <span class="chip chip-orphan" title="Набор не найден в текущем конфиге">
                <span class="chip-label">{v}</span>
                <span class="chip-orphan-badge">орфан</span>
                <button
                    type="button"
                    class="chip-remove"
                    aria-label="Удалить"
                    onclick={() => removeValue(v)}
                    {disabled}
                >×</button>
            </span>
        {/each}
        <button
            type="button"
            class="trigger"
            bind:this={triggerEl}
            onclick={toggleOpen}
            disabled={disabled || allSelected}
            aria-haspopup="listbox"
            aria-expanded={open}
        >+</button>
    </div>
</div>

{#if open}
    <div
        use:portal
        class="panel"
        bind:this={panelEl}
        style="top: {panelTop}px; left: {panelLeft}px; min-width: {panelWidth}px;"
        role="listbox"
    >
        {#each dropdownItems as opt (opt.value)}
            <button
                type="button"
                class="panel-item"
                onclick={() => addValue(opt.value)}
                role="option"
                aria-selected="false"
            >
                {opt.label ?? opt.value}
            </button>
        {/each}
    </div>
{/if}

<style>
    .picker {
        display: block;
    }
    .chips {
        display: flex;
        flex-wrap: wrap;
        gap: 0.3rem;
        align-items: center;
        padding: 0.35rem 0.45rem;
        background: var(--bg);
        border: 1px solid var(--border);
        border-radius: 4px;
        min-height: 2rem;
    }
    .placeholder {
        color: var(--muted-text);
        font-size: 0.85rem;
        padding: 0 0.25rem;
    }
    .chip {
        display: inline-flex;
        align-items: center;
        gap: 0.25rem;
        padding: 0.15rem 0.45rem;
        background: var(--bg-tertiary, var(--surface-bg));
        border: 1px solid var(--border);
        border-radius: 999px;
        font-family: ui-monospace, monospace;
        font-size: 0.78rem;
        color: var(--text);
    }
    .chip-orphan {
        border-color: var(--warning, #e0af68);
        background: rgba(224, 175, 104, 0.12);
    }
    .chip-orphan-badge {
        font-size: 0.65rem;
        font-weight: 600;
        color: var(--warning, #e0af68);
        text-transform: uppercase;
    }
    .chip-remove {
        background: none;
        border: none;
        color: var(--muted-text);
        cursor: pointer;
        font-size: 1rem;
        line-height: 1;
        padding: 0 0.15rem;
    }
    .chip-remove:hover {
        color: var(--text);
    }
    .chip-remove:disabled {
        cursor: not-allowed;
        opacity: 0.5;
    }
    .trigger {
        background: none;
        border: 1px dashed var(--border);
        border-radius: 999px;
        color: var(--muted-text);
        cursor: pointer;
        font-size: 0.95rem;
        line-height: 1;
        padding: 0.15rem 0.55rem;
    }
    .trigger:hover:not(:disabled) {
        color: var(--text);
        border-color: var(--accent, #3b82f6);
    }
    .trigger:disabled {
        cursor: not-allowed;
        opacity: 0.4;
    }
    .panel {
        position: fixed;
        z-index: var(--z-floating);
        background: var(--bg-tertiary, var(--surface-bg));
        border: 1px solid var(--border-bright, var(--border));
        border-radius: 4px;
        box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
        max-height: min(60vh, calc(100vh - 200px));
        overflow-y: auto;
        padding: 0.25rem 0;
    }
    .panel-item {
        display: block;
        width: 100%;
        text-align: left;
        background: none;
        border: none;
        padding: 0.45rem 0.7rem;
        font-family: ui-monospace, monospace;
        font-size: 0.82rem;
        color: var(--text);
        cursor: pointer;
    }
    .panel-item:hover {
        background: var(--bg-hover);
    }
</style>
