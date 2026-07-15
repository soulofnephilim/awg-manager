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
	/** Для этой архитектуры есть закреплённая сборка — доступна установка в один клик */
	installAvailable: boolean;
	/** Версия freeturn, которую поставит установка */
	installVersion?: string;
	/** Установка сейчас идёт */
	installing: boolean;
}

// Share-link payload: freeturn://base64(JSON). Two flavors exist and both
// decode into this same shape — the upstream free-turn-proxy format (see
// samosvalishe/free-turn-proxy docs/uri.md: v/provider/peer/transport/mode/
// bond/obf/key/n/spc/cid/listen/dns/dnss/mcap/name) and the informal
// freeturn-entware-installer one (v/provider/peer/obf/key/mtu/wg). `cid` is
// a Client ID the link's creator generated and must separately allowlist in
// their own server's clients.json (if -clients-file auth is on) — importing
// a link does NOT do that registration for you.
export interface FreeTurnLinkPayload {
	v: number;
	provider?: string;
	peer?: string;
	transport?: string;
	mode?: string;
	bond?: boolean;
	obf?: string;
	key?: string;
	n?: number;
	spc?: number;
	cid?: string;
	listen?: string;
	dns?: string;
	dnss?: string;
	mcap?: boolean;
	name?: string;
	mtu?: number;
	wg?: string;
}

export interface FreeTurnGenerateLinkRequest {
	peer?: string;
	provider?: string;
	mtu?: number;
	wg?: string;
	clientId?: string;
	name?: string;
	n?: number;
	streamsPerCred?: number;
}

export interface FreeTurnGenerateLinkResult {
	link: string;
	peer: string;
	clientId?: string;
}

// #endregion
