import { describe, it, expect } from 'vitest';
import {
  encodeServerPeerValue,
  decodeServerPeerValue,
  buildServerPeerDropdownOptions,
} from './serverPeerOptions';
import type { ServersSnapshot } from '$lib/stores/servers';

function snap(over: Partial<ServersSnapshot> = {}): ServersSnapshot {
  return { servers: [], managed: [], managedStats: {}, ...over };
}

describe('serverPeerOptions value codec', () => {
  it('round-trips kind/serverId/pubkey', () => {
    const v = encodeServerPeerValue('system', 'wg-id-1', 'PUBKEYAAA');
    expect(decodeServerPeerValue(v)).toEqual({
      kind: 'system',
      serverId: 'wg-id-1',
      pubkey: 'PUBKEYAAA',
    });
  });
});

describe('buildServerPeerDropdownOptions', () => {
  it('null/empty snapshot → empty list', () => {
    expect(buildServerPeerDropdownOptions(null)).toEqual([]);
    expect(buildServerPeerDropdownOptions(snap())).toEqual([]);
  });

  it('filters system peers by confAvailable, keeps managed peers always', () => {
    const s = snap({
      servers: [
        {
          id: 'sys1',
          interfaceName: 'Wireguard0',
          description: 'Sys',
          peers: [
            { publicKey: 'SYS_OK', description: 'ok', confAvailable: true },
            { publicKey: 'SYS_NO', description: 'no', confAvailable: false },
            { publicKey: 'SYS_UNDEF', description: 'undef' },
          ],
        } as any,
      ],
      managed: [
        {
          interfaceName: 'awg-mng0',
          description: 'Mng',
          peers: [{ publicKey: 'MNG1', description: 'm1' }],
        } as any,
      ],
    });

    const opts = buildServerPeerDropdownOptions(s);
    const values = opts.map((o) => o.value);

    // system: only confAvailable === true
    expect(values).toContain(encodeServerPeerValue('system', 'sys1', 'SYS_OK'));
    expect(values).not.toContain(encodeServerPeerValue('system', 'sys1', 'SYS_NO'));
    expect(values).not.toContain(encodeServerPeerValue('system', 'sys1', 'SYS_UNDEF'));
    // managed: always (serverId = interfaceName)
    expect(values).toContain(encodeServerPeerValue('managed', 'awg-mng0', 'MNG1'));
  });

  it('pins per-kind serverId source: system uses id, not interfaceName', () => {
    // The documented 404-blocker: system must encode WireguardServer.id
    // ('sys1'), NOT interfaceName ('Wireguard0'). Fixture gives distinct
    // values so a swap is caught here, not only at runtime.
    const s = snap({
      servers: [
        {
          id: 'sys1',
          interfaceName: 'Wireguard0',
          description: 'Sys',
          peers: [{ publicKey: 'SYS_OK', description: 'ok', confAvailable: true }],
        } as any,
      ],
    });
    const opt = buildServerPeerDropdownOptions(s)[0];
    expect(decodeServerPeerValue(opt.value).serverId).toBe('sys1');
  });
});
