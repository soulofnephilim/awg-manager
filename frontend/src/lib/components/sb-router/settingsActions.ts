import type { SingboxRouterSettings } from '$lib/types';
import { api } from '$lib/api/client';
import { singboxRouter } from '$lib/stores/singboxRouter';

export interface BypassPresetMeta {
  id: string;
  label: string;
  desc: string;
}

export const BYPASS_PRESETS: readonly BypassPresetMeta[] = [
  { id: 'l2tp', label: 'L2TP / IPsec VPN', desc: 'UDP 500, 1701, 4500' },
  { id: 'ntp', label: 'NTP (синхронизация времени)', desc: 'UDP 123' },
  { id: 'netbios-smb', label: 'NetBIOS / SMB', desc: 'UDP 137/138, TCP 139/445' },
  // Не порты, а destination-IP: статическая A-запись KeenDNS/CrazeDNS-доменов
  // (my.keenetic.net / my.netcraze.net и 4-го уровня) указывает на этот IP,
  // который роутер обслуживает локально — перехват ломает доступ (#490).
  { id: 'keendns', label: 'KeenDNS / CrazeDNS', desc: 'IP 78.47.125.180' },
];

export async function mergeAndSaveSettings(
  patch: Partial<SingboxRouterSettings>,
): Promise<void> {
  // База для merge — СВЕЖИЙ GET с сервера, а не значение стора: настройки
  // меняются и вне settings-форм (fakeipRealServer пишет бэкенд при правке
  // адреса DNS-сервера «real» — #487; selectiveBypass гасит
  // reconcile-самолечение — #486), а PUT уносит полный объект, поэтому эхо
  // устаревшего стора молча откатывало такие изменения.
  const current = await api.singboxRouterGetSettings();
  const merged: SingboxRouterSettings = { ...current, ...patch };
  await api.singboxRouterPutSettings(merged);
  await singboxRouter.loadAll();
}
