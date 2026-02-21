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

	private pollInterval: number | null = null;
	private pollDelay = 2000; // Poll every 2 seconds

	constructor() {
		// Auto-start polling when store is created
		this.startPolling();
	}

	async fetchState() {
		try {
			const state = await api.getState();
			this.updateState(state);
			this.connected = true;
			this.error = null;
		} catch (err) {
			this.connected = false;
			this.error = err instanceof Error ? err.message : 'Failed to fetch state';
			console.error('Failed to fetch state:', err);
		} finally {
			this.loading = false;
		}
	}

	private updateState(state: State) {
		this.sources = state.sources;
		this.zones = state.zones;
		this.groups = state.groups;
		this.streams = state.streams;
		this.presets = state.presets;
		this.info = state.info;
	}

	startPolling() {
		// Initial fetch
		this.fetchState();

		// Set up polling
		if (this.pollInterval) {
			clearInterval(this.pollInterval);
		}

		this.pollInterval = window.setInterval(() => {
			this.fetchState();
		}, this.pollDelay);
	}

	stopPolling() {
		if (this.pollInterval) {
			clearInterval(this.pollInterval);
			this.pollInterval = null;
		}
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
