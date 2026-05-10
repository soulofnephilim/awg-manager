import type { SubscriptionHeader } from '$lib/types';

export function parseHeadersText(text: string): SubscriptionHeader[] {
	const lines = text.split('\n');
	const out: SubscriptionHeader[] = [];
	for (const raw of lines) {
		const line = raw.trim();
		if (!line || line.startsWith('#')) continue;
		const idx = line.indexOf(':');
		if (idx <= 0) continue;
		const name = line.slice(0, idx).trim();
		const value = line.slice(idx + 1).trim();
		if (name && value) out.push({ name, value });
	}
	return out;
}

export function serializeHeaders(headers: SubscriptionHeader[]): string {
	return headers.map((h) => `${h.name}: ${h.value}`).join('\n');
}

// DEFAULT_PRESET is applied automatically when the user opens the
// "create subscription" modal. A sing-box User-Agent makes most
// providers respond with sing-box JSON config (single, array of
// configs, or array of outbounds) — formats our parser supports for
// vless / trojan / ss / hysteria2 outbound types.
export const DEFAULT_PRESET = `User-Agent: sing-box/v1.14.20`;

// MIHOMO_PRESET stays available for providers that branch on a
// Clash/mihomo UA and only emit Clash YAML or base64 share-links.
// Use it when the default sing-box UA returns nothing useful.
export const MIHOMO_PRESET = `User-Agent: mihomo/v1.19.20`;

// HAPP_PRESET stays available for providers that gate access on the
// vendor-specific Happ iOS headers. Note: sites that branch on this
// UA typically return a V2Ray-style JSON config which our parser
// does NOT understand — only use this preset if your provider
// explicitly requires Happ-format headers.
export const HAPP_PRESET = `User-Agent: Happ/4.6.0/ios/2603181556604
X-Device-OS: iOS
X-HWID: d1c1da1b1b111111
X-Device-Locale: ru
X-Ver-OS: 26.4
X-App-Version: 4.6.0
X-Device-Model: iPhone 17 Pro Max`;
