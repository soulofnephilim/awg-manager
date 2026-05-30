<!--
  Settings Drawer для движка sing-box.
  Все 8 полей SingboxRouterSettings в 5 секциях с auto-save.
-->

<script lang="ts">
  import { onMount } from 'svelte';
  import { singboxRouter as singboxRouterStore } from '$lib/stores/singboxRouter';
  import { notifications } from '$lib/stores/notifications';
  import { Toggle, SideDrawer } from '$lib/components/ui';
  import OutboundOption from './OutboundOption.svelte';
  import { api } from '$lib/api/client';
  import type { SingboxRouterSettings, SingboxRouterWANInterface } from '$lib/types';

  import { settingsDrawerOpen, closeSettingsDrawer } from './settingsDrawerStore';
  import { mergeAndSaveSettings, BYPASS_PRESETS } from './settingsActions';

  const storeSettings = singboxRouterStore.settings;

  let wanInterfaces = $state<SingboxRouterWANInterface[]>([]);
  let saving = $state(false);
  let lastError = $state<string | null>(null);

  let portTimer: ReturnType<typeof setTimeout> | null = null;
  let policyNameTimer: ReturnType<typeof setTimeout> | null = null;

  onMount(async () => {
    void singboxRouterStore.loadAll();
    try {
      wanInterfaces = await api.singboxRouterListWANInterfaces();
    } catch (_e) {
      // ignore
    }
  });

  async function applyPatch(patch: Partial<SingboxRouterSettings>) {
    if (!$storeSettings) return;
    saving = true;
    lastError = null;
    try {
      await mergeAndSaveSettings(patch);
    } catch (e) {
      lastError = e instanceof Error ? e.message : String(e);
      notifications.error(`Не удалось сохранить: ${lastError}`);
    } finally {
      saving = false;
    }
  }

  function setDeviceMode(mode: 'policy' | 'all') {
    void applyPatch({ deviceMode: mode });
  }
  function onPolicyNameInput(e: Event) {
    const v = (e.currentTarget as HTMLInputElement).value;
    if (policyNameTimer) clearTimeout(policyNameTimer);
    policyNameTimer = setTimeout(() => {
      void applyPatch({ policyName: v });
    }, 500);
  }

  function toggleAutoDetect(checked: boolean) {
    if (checked) {
      void applyPatch({ wanAutoDetect: true, wanInterface: '' });
    } else {
      void applyPatch({ wanAutoDetect: false });
    }
  }
  function onWanInterfaceChange(e: Event) {
    const v = (e.currentTarget as HTMLSelectElement).value;
    void applyPatch({ wanAutoDetect: false, wanInterface: v });
  }

  function toggleSniffer(checked: boolean) {
    void applyPatch({ snifferEnabled: checked });
  }

  function togglePreset(id: string) {
    const current = $storeSettings?.bypassPresets ?? [];
    const next = current.includes(id)
      ? current.filter((x) => x !== id)
      : [...current, id];
    void applyPatch({ bypassPresets: next });
  }
  function onExtraPortsInput(e: Event) {
    const v = (e.currentTarget as HTMLInputElement).value;
    if (portTimer) clearTimeout(portTimer);
    portTimer = setTimeout(() => {
      void applyPatch({ bypassExtraPorts: v });
    }, 500);
  }

  function setRefreshMode(mode: 'interval' | 'daily') {
    void applyPatch({ refreshMode: mode });
  }
  function onIntervalInput(e: Event) {
    const v = parseInt((e.currentTarget as HTMLInputElement).value, 10);
    if (Number.isFinite(v) && v > 0) {
      void applyPatch({ refreshIntervalHours: v });
    }
  }
  function onDailyTimeInput(e: Event) {
    const v = (e.currentTarget as HTMLInputElement).value;
    void applyPatch({ refreshDailyTime: v });
  }
</script>

<SideDrawer
  open={$settingsDrawerOpen}
  onClose={closeSettingsDrawer}
  title="Настройки движка"
>
  <div class="sections">
      {#if $storeSettings}
      <!-- Section 1: deviceMode -->
      <section class="sec">
        <div class="sec-cap">Режим работы</div>
        <div class="card-grid">
          <OutboundOption
            label="Только устройства policy"
            sub="трафик из назначенной policy"
            tone="accent"
            selected={$storeSettings.deviceMode !== 'all'}
            onclick={() => setDeviceMode('policy')}
          />
          <OutboundOption
            label="Весь роутер"
            sub="весь LAN-трафик"
            tone="accent"
            selected={$storeSettings.deviceMode === 'all'}
            onclick={() => setDeviceMode('all')}
          />
        </div>
        {#if $storeSettings.deviceMode !== 'all'}
          <div class="field">
            <label class="lbl" for="setdrawer-policy">Имя NDMS policy</label>
            <input
              id="setdrawer-policy"
              class="inp"
              type="text"
              value={$storeSettings.policyName}
              oninput={onPolicyNameInput}
            />
          </div>
        {/if}
        <p class="hint">При policy — обрабатывается только трафик устройств привязанных к policy в LAN-настройках NDMS.</p>
      </section>

      <!-- Section 2: WAN -->
      <section class="sec">
        <div class="sec-cap">WAN-интерфейс</div>
        <div class="field-row">
          <Toggle
            checked={$storeSettings.wanAutoDetect}
            onchange={(checked) => toggleAutoDetect(checked)}
          />
          <span>Авто-определение</span>
        </div>
        {#if !$storeSettings.wanAutoDetect}
          <div class="field">
            <label class="lbl" for="setdrawer-wan">Интерфейс</label>
            <select
              id="setdrawer-wan"
              class="inp"
              value={$storeSettings.wanInterface ?? ''}
              onchange={onWanInterfaceChange}
            >
              <option value="">— выберите —</option>
              {#each wanInterfaces as iface (iface.name)}
                <option value={iface.name}>
                  {iface.name}{iface.label ? ` — ${iface.label}` : ''}
                </option>
              {/each}
            </select>
          </div>
        {/if}
        <p class="hint">Через какой внешний интерфейс sing-box будет отправлять прямой трафик.</p>
      </section>

      <!-- Section 3: sniffer -->
      <section class="sec">
        <div class="sec-cap">Анализ трафика</div>
        <div class="field-row">
          <Toggle
            checked={$storeSettings.snifferEnabled}
            onchange={(checked) => toggleSniffer(checked)}
          />
          <span>Включить sniff (HTTP/TLS/QUIC по содержимому)</span>
        </div>
        <p class="hint">Улучшает срабатывание domain-based правил при IP-only matchers.</p>
      </section>

      <!-- Section 4: bypass ports -->
      <section class="sec">
        <div class="sec-cap">Исключения портов</div>
        <div class="chips">
          {#each BYPASS_PRESETS as p (p.id)}
            {@const active = ($storeSettings.bypassPresets ?? []).includes(p.id)}
            <button
              type="button"
              class="chip"
              class:active
              onclick={() => togglePreset(p.id)}
            >
              <span class="chip-label">{p.label}</span>
              <span class="chip-desc">{p.desc}</span>
            </button>
          {/each}
        </div>
        <div class="field">
          <label class="lbl" for="setdrawer-ports">Доп. порты (формат: udp:53, tcp:443)</label>
          <input
            id="setdrawer-ports"
            class="inp"
            type="text"
            value={$storeSettings.bypassExtraPorts ?? ''}
            placeholder="udp:53, tcp:443"
            oninput={onExtraPortsInput}
          />
        </div>
        <p class="hint">Эти порты пойдут мимо sing-box (прямо в WAN). Полезно для L2TP/NTP/SMB не ломая LAN-сервисы.</p>
      </section>

      <!-- Section 5: refresh schedule -->
      <section class="sec">
        <div class="sec-cap">Расписание обновлений гео-данных</div>
        <div class="card-grid">
          <OutboundOption
            label="По интервалу"
            sub="каждые N часов"
            tone="accent"
            selected={($storeSettings.refreshMode ?? 'interval') === 'interval'}
            onclick={() => setRefreshMode('interval')}
          />
          <OutboundOption
            label="Ежедневно"
            sub="в указанное время"
            tone="accent"
            selected={$storeSettings.refreshMode === 'daily'}
            onclick={() => setRefreshMode('daily')}
          />
        </div>
        {#if ($storeSettings.refreshMode ?? 'interval') === 'interval'}
          <div class="field">
            <label class="lbl" for="setdrawer-int">Каждые (часов)</label>
            <input
              id="setdrawer-int"
              class="inp"
              type="number"
              min="1"
              value={$storeSettings.refreshIntervalHours ?? 24}
              oninput={onIntervalInput}
            />
          </div>
        {:else}
          <div class="field">
            <label class="lbl" for="setdrawer-time">Время (HH:MM)</label>
            <input
              id="setdrawer-time"
              class="inp"
              type="time"
              value={$storeSettings.refreshDailyTime ?? '03:00'}
              oninput={onDailyTimeInput}
            />
          </div>
        {/if}
      </section>
      {:else}
        <div class="loading">Загрузка настроек роутера…</div>
      {/if}
  </div>

  {#if $storeSettings}
    <div class="status-bar">
      {#if saving}
        <span class="status saving">Сохраняем…</span>
      {:else if lastError}
        <span class="status err">Ошибка: {lastError}</span>
      {:else}
        <span class="status ok">Сохранено</span>
      {/if}
    </div>
  {/if}
</SideDrawer>

<style>
  .sections {
    display: flex;
    flex-direction: column;
  }
  .sec {
    padding: 14px var(--sp-4);
    border-bottom: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .sec:last-of-type {
    border-bottom: 0;
  }
  .sec-cap {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .field-row {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 13px;
  }
  .lbl {
    font-size: 11px;
    color: var(--text-muted);
    font-weight: 500;
  }
  .inp {
    padding: 6px 10px;
    border-radius: var(--radius-sm);
    background: var(--bg-primary);
    border: 1px solid var(--border);
    color: var(--text-primary);
    font-size: 12.5px;
    font-family: inherit;
  }
  .card-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px;
  }
  @media (max-width: 480px) {
    .card-grid {
      grid-template-columns: 1fr;
    }
  }
  .hint {
    margin: 0;
    font-size: 11.5px;
    color: var(--text-muted);
    line-height: 1.4;
  }
  .chips {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .chip {
    text-align: left;
    padding: 8px 10px;
    border-radius: var(--radius-sm);
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    cursor: pointer;
    font-family: inherit;
    color: inherit;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .chip.active {
    background: var(--accent-soft);
    border-color: var(--accent);
  }
  .chip-label {
    font-size: 12.5px;
    font-weight: 600;
  }
  .chip-desc {
    font-size: 11px;
    color: var(--text-muted);
    font-family: var(--font-mono);
  }
  .status-bar {
    padding: 8px var(--sp-4);
    border-top: 1px solid var(--border);
    background: var(--bg-tertiary);
    font-size: 11px;
  }
  .status.saving { color: var(--text-muted); }
  .status.ok { color: var(--text-muted); }
  .status.err { color: var(--color-error, #dc2626); }
  .loading {
    padding: var(--sp-4);
    text-align: center;
    color: var(--text-muted);
    font-size: 12px;
  }
</style>
