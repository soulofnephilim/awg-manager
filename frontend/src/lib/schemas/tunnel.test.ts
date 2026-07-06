import { describe, it, expect } from 'vitest';
import { editTunnelSchema } from './tunnel';

// Minimal valid payload; individual tests override `endpoint`.
function base(endpoint: string) {
    return {
        name: 'wg0',
        address: '10.0.0.2/32',
        endpoint,
        allowedIPs: '0.0.0.0/0',
    };
}

function endpointErr(endpoint: string): string | undefined {
    const res = editTunnelSchema.safeParse(base(endpoint));
    if (res.success) return undefined;
    return res.error.issues.find(i => i.path[0] === 'endpoint')?.message;
}

describe('editTunnelSchema endpoint', () => {
    it('accepts hostname:port', () => {
        expect(endpointErr('vpn.example.com:51820')).toBeUndefined();
    });

    it('accepts IPv4:port', () => {
        expect(endpointErr('1.2.3.4:51820')).toBeUndefined();
    });

    it('accepts bracketed IPv6:port', () => {
        expect(endpointErr('[2001:db8::1]:51820')).toBeUndefined();
    });

    it('rejects bare (unbracketed) IPv6', () => {
        expect(endpointErr('2001:db8::1:51820')).toBe(
            'IPv6 endpoint указывается в квадратных скобках: [2001:db8::1]:51820',
        );
    });

    it('rejects empty endpoint', () => {
        expect(endpointErr('')).toBe('Endpoint обязателен');
    });
});
