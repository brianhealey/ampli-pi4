// AmpliPi State Store using Svelte 5 Runes

import { api } from './api';
import type { State, Source, Zone, Group, Stream, Preset } from './types';

class AmpliPiStore {
	// Reactive state using runes
	sources = $state<Source[]>([]);
	zones = $state<Zone[]>([]);
	groups = $state<Group[]>([]);
	streams = $state<Stream[]>([]);
	presets = $state<Preset[]>([]);
	info = $state<State['info'] | null>(null);

	loading = $state(true);
	error = $state<string | null>(null);
	connected = $state(false);

	private eventSource: EventSource | null = null;
	private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
	private reconnectDelay = 1000; // Start with 1 second
	private maxReconnectDelay = 30000; // Max 30 seconds

	async connect() {
		this.loading = true;
		this.error = null;

		try {
			// Initial state fetch
			const state = await api.getState();
			this.updateState(state);
			this.connected = true;

			// Start SSE connection
			this.startEventSource();
		} catch (err) {
			this.error = err instanceof Error ? err.message : 'Failed to connect';
			this.connected = false;
			this.scheduleReconnect();
		} finally {
			this.loading = false;
		}
	}

	private startEventSource() {
		// Close existing connection if any
		if (this.eventSource) {
			this.eventSource.close();
		}

		this.eventSource = new EventSource('/api/subscribe');

		this.eventSource.onmessage = (event) => {
			try {
				const state = JSON.parse(event.data);
				this.updateState(state);
				this.connected = true;
				this.error = null;

				// Reset reconnect delay on successful connection
				this.reconnectDelay = 1000;
			} catch (err) {
				console.error('Failed to parse SSE event:', err);
			}
		};

		this.eventSource.onerror = () => {
			this.connected = false;
			this.eventSource?.close();
			this.eventSource = null;
			this.scheduleReconnect();
		};
	}

	private scheduleReconnect() {
		if (this.reconnectTimeout) {
			clearTimeout(this.reconnectTimeout);
		}

		this.reconnectTimeout = setTimeout(() => {
			console.log(`Reconnecting to SSE (delay: ${this.reconnectDelay}ms)...`);
			this.connect();

			// Exponential backoff
			this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxReconnectDelay);
		}, this.reconnectDelay);
	}

	private updateState(state: State) {
		this.sources = state.sources || [];
		this.zones = state.zones || [];
		this.groups = state.groups || [];
		this.streams = state.streams || [];
		this.presets = state.presets || [];
		this.info = state.info;
	}

	disconnect() {
		if (this.eventSource) {
			this.eventSource.close();
			this.eventSource = null;
		}
		if (this.reconnectTimeout) {
			clearTimeout(this.reconnectTimeout);
			this.reconnectTimeout = null;
		}
		this.connected = false;
	}

	// Helper methods for common operations
	getZone(id: number): Zone | undefined {
		return this.zones.find((z) => z.id === id);
	}

	getSource(id: number): Source | undefined {
		return this.sources.find((s) => s.id === id);
	}

	getGroup(id: number): Group | undefined {
		return this.groups.find((g) => g.id === id);
	}

	getStream(id: number): Stream | undefined {
		return this.streams.find((s) => s.id === id);
	}

	getZoneSource(zone: Zone): Source | undefined {
		return this.getSource(zone.source_id);
	}

	getZonesBySource(sourceId: number): Zone[] {
		return this.zones.filter((z) => z.source_id === sourceId && !z.disabled);
	}

	getGroupZones(group: Group): Zone[] {
		return group.zones.map((id) => this.getZone(id)).filter((z): z is Zone => z !== undefined);
	}

	// Computed properties using $derived
	activeZones = $derived(this.zones.filter((z) => !z.disabled && z.source_id >= 0));

	activeSources = $derived(
		this.sources.filter((s) => this.zones.some((z) => z.source_id === s.id && !z.disabled))
	);
}

// Create and export singleton instance
export const amplipi = new AmpliPiStore();
