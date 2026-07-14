// ─────────────────────────────────────────────
// #region FreeTurn — TURN-tunnel client + server config/status
// See https://github.com/samosvalishe/free-turn-proxy/blob/master/docs/flags.md
// ─────────────────────────────────────────────

export interface FreeTurnClientConfig {
	enabled: boolean;
	listen: string;
	peer: string;
	provider: string;
	link?: string;
	streams: number;
	transport: 'tcp' | 'udp';
	mode: 'udp' | 'tcp';
	bond: boolean;
	turnHost?: string;
	turnPort?: number;
	obfProfile: 'none' | 'rtpopus' | 'rtpopus2';
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
	obfProfile: 'none' | 'rtpopus' | 'rtpopus2';
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
}

export interface FreeTurnStatus {
	client: FreeTurnProcessStatus;
	server: FreeTurnProcessStatus;
}

// #endregion
