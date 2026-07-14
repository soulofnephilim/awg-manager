import type {
	FreeTurnClientConfig,
	FreeTurnConfig,
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

	// #endregion
}
