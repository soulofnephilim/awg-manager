// ─────────────────────────────────────────────
// #region FreeTurn — TURN-tunnel client + server config/status
// See https://github.com/samosvalishe/free-turn-proxy/blob/master/docs/flags.md
// ─────────────────────────────────────────────

export interface FreeTurnClientConfig {
	enabled: boolean;
	listen: string;
	peer: string;
	provider: string;
	links?: string;
	streams: number;
	transport: 'tcp' | 'udp';
	mode: 'udp' | 'tcp';
	bond: boolean;
	turnHost?: string;
	turnPort?: number;
	obfProfile: 'none' | 'rtpopus' | 'rtpopus2' | 'rtpopus3';
	obfKey?: string;
	streamsPerCred: number;
	browser: 'chrome' | 'firefox';
	manualCaptcha: boolean;
	dnsMode: 'plain' | 'doh' | 'auto';
	dnsServers?: string;
	clientId?: string;
	sub?: string;
	debug: boolean;
}

export interface FreeTurnServerConfig {
	enabled: boolean;
	listen: string;
	connect: string;
	mode: 'udp' | 'tcp';
	obfProfile: 'none' | 'rtpopus' | 'rtpopus2' | 'rtpopus3';
	obfKey?: string;
	clientsFile?: string;
	debug: boolean;
}

export interface FreeTurnConfig {
	client: FreeTurnClientConfig;
	server: FreeTurnServerConfig;
}

export interface FreeTurnProcessStatus {
	running: boolean;
	pid?: number;
	startedAt?: string;
	lastError?: string;
	log?: string;
	/** Путь к бинарю и признак его наличия — awg-manager freeturn не поставляет */
	binary: string;
	binaryPresent: boolean;
}

export interface FreeTurnStatus {
	client: FreeTurnProcessStatus;
	server: FreeTurnProcessStatus;
}

// Share-link payload: freeturn://base64(JSON), the same format produced by
// the original freeturn-entware-installer web generator. Bundles the
// server's connection params (+ optionally a WireGuard client config) so
// the receiving side can auto-fill its form instead of retyping peer/obf/key.
export interface FreeTurnLinkPayload {
	v: number;
	provider: string;
	peer: string;
	obf: string;
	key: string;
	mtu: number;
	wg?: string;
}

export interface FreeTurnGenerateLinkRequest {
	peer?: string;
	provider?: string;
	mtu?: number;
	wg?: string;
}

export interface FreeTurnGenerateLinkResult {
	link: string;
	peer: string;
}

// #endregion
