import type {
	FreeTurnClientConfig,
	FreeTurnConfig,
	FreeTurnGenerateLinkRequest,
	FreeTurnGenerateLinkResult,
	FreeTurnLinkPayload,
	FreeTurnServerConfig,
	FreeTurnStatus
} from '$lib/types';
import { SubscriptionsClient } from './clientSubscriptions';

export class FreeturnClient extends SubscriptionsClient {
	// ─────────────────────────────────────────────
	// #region FreeTurn — TURN-tunnel client + server
	// ─────────────────────────────────────────────

	async getFreeTurnConfig(): Promise<FreeTurnConfig> {
		return this.request<FreeTurnConfig>('/freeturn/config');
	}

	async updateFreeTurnClientConfig(config: FreeTurnClientConfig): Promise<FreeTurnClientConfig> {
		return this.request<FreeTurnClientConfig>('/freeturn/client/config', {
			method: 'PUT',
			body: JSON.stringify(config)
		});
	}

	async updateFreeTurnServerConfig(config: FreeTurnServerConfig): Promise<FreeTurnServerConfig> {
		return this.request<FreeTurnServerConfig>('/freeturn/server/config', {
			method: 'PUT',
			body: JSON.stringify(config)
		});
	}

	async getFreeTurnStatus(): Promise<FreeTurnStatus> {
		return this.request<FreeTurnStatus>('/freeturn/status');
	}

	async startFreeTurnClient(): Promise<{ message: string }> {
		return this.request('/freeturn/client/start', { method: 'POST' });
	}

	async stopFreeTurnClient(): Promise<{ message: string }> {
		return this.request('/freeturn/client/stop', { method: 'POST' });
	}

	async startFreeTurnServer(): Promise<{ message: string }> {
		return this.request('/freeturn/server/start', { method: 'POST' });
	}

	async stopFreeTurnServer(): Promise<{ message: string }> {
		return this.request('/freeturn/server/stop', { method: 'POST' });
	}

	async generateFreeTurnLink(
		req: FreeTurnGenerateLinkRequest = {}
	): Promise<FreeTurnGenerateLinkResult> {
		return this.request<FreeTurnGenerateLinkResult>('/freeturn/server/link', {
			method: 'POST',
			body: JSON.stringify(req)
		});
	}

	async decodeFreeTurnLink(link: string): Promise<FreeTurnLinkPayload> {
		return this.request<FreeTurnLinkPayload>('/freeturn/link/decode', {
			method: 'POST',
			body: JSON.stringify({ link })
		});
	}

	/** Скачивает и активирует закреплённые бинари freeturn (client+server). */
	async installFreeTurn(): Promise<void> {
		await this.request<{ message: string }>('/freeturn/install', { method: 'POST' });
	}

	// #endregion
}
