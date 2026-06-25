import type { DropdownOption } from '$lib/components/ui';
import type { ServersSnapshot } from '$lib/stores/servers';

export type ServerPeerKind = 'managed' | 'system';

const SEP = '\0';

export function encodeServerPeerValue(
  kind: ServerPeerKind,
  serverId: string,
  pubkey: string,
): string {
  return `${kind}${SEP}${serverId}${SEP}${pubkey}`;
}

export function decodeServerPeerValue(value: string): {
  kind: ServerPeerKind;
  serverId: string;
  pubkey: string;
} {
  const [kind, serverId, pubkey] = value.split(SEP);
  return { kind: kind as ServerPeerKind, serverId, pubkey };
}

function peerLabel(pubkey: string, description: string): string {
  return description || `${pubkey.slice(0, 16)}…`;
}

/**
 * Сгруппированные dropdown-опции «сервер → пир» для анализатора.
 * serverId per-kind: managed = interfaceName, system = id.
 * Системные пиры включаются только при confAvailable === true (де-факто
 * «создан средствами awgm»); managed-пиры — всегда.
 */
export function buildServerPeerDropdownOptions(
  snap: ServersSnapshot | null,
): DropdownOption[] {
  if (!snap) return [];
  const opts: DropdownOption[] = [];

  for (const s of snap.managed ?? []) {
    const group = `Managed · ${s.description || s.interfaceName}`;
    for (const p of s.peers ?? []) {
      opts.push({
        value: encodeServerPeerValue('managed', s.interfaceName, p.publicKey),
        label: peerLabel(p.publicKey, p.description),
        group,
      });
    }
  }

  for (const s of snap.servers ?? []) {
    const group = `Системный WG · ${s.description || s.interfaceName}`;
    for (const p of s.peers ?? []) {
      if (p.confAvailable !== true) continue;
      opts.push({
        value: encodeServerPeerValue('system', s.id, p.publicKey),
        label: peerLabel(p.publicKey, p.description),
        group,
      });
    }
  }

  return opts;
}
