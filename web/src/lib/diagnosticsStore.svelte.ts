// Diagnostics store: polls device telemetry and retains a rolling history so
// the About page can chart CPU, memory and temperature over time.

import { api } from './api';
import type { SystemStats, HardwareStatus } from './diagnostics';

/** A single point in the rolling telemetry history. */
export interface HistoryPoint {
	t: number; // epoch milliseconds
	cpu: number; // percent
	memory: number; // percent
	temp: number; // degrees Celsius
}

/** How many samples the chart retains. At 2s cadence this is ~5 minutes. */
export const HISTORY_LENGTH = 150;

/** Poll cadence in milliseconds. */
const POLL_INTERVAL = 2000;

/** Hardware topology changes only on reboot, so it is refetched rarely. */
const HARDWARE_REFRESH_INTERVAL = 30000;

class DiagnosticsStore {
	stats = $state<SystemStats | null>(null);
	hardware = $state<HardwareStatus | null>(null);
	history = $state<HistoryPoint[]>([]);

	// GET /api reports the persisted state's info block, whose version is
	// blank; /api/info carries the version the daemon actually computed.
	version = $state<string>('');

	loading = $state(true);
	error = $state<string | null>(null);

	private pollTimer: number | null = null;
	private abortController: AbortController | null = null;
	private lastHardwareFetch = 0;

	/** Begins polling. Safe to call repeatedly; a running poll is replaced. */
	start() {
		this.stop();
		this.loadVersion();
		this.refresh();
		this.pollTimer = window.setInterval(() => this.refresh(), POLL_INTERVAL);
	}

	/** Fetches the daemon version once — it cannot change while it runs. */
	private async loadVersion() {
		if (this.version) return;
		try {
			const info = await api.getInfo();
			this.version = info?.version ?? '';
		} catch (err) {
			console.error('Failed to load daemon version:', err);
		}
	}

	/** Stops polling and aborts any in-flight request. */
	stop() {
		if (this.pollTimer !== null) {
			clearInterval(this.pollTimer);
			this.pollTimer = null;
		}
		this.abortController?.abort();
		this.abortController = null;
	}

	/** Clears retained history, e.g. when the page is reopened. */
	reset() {
		this.history = [];
		this.stats = null;
		this.loading = true;
		this.error = null;
	}

	private async refresh() {
		// Cancel a still-pending poll so a slow response cannot overwrite a
		// newer one.
		this.abortController?.abort();
		this.abortController = new AbortController();
		const signal = this.abortController.signal;

		try {
			const stats = await api.getSystemStats(signal);
			if (signal.aborted) return;

			this.stats = stats;
			this.appendHistory(stats);
			this.error = null;

			await this.refreshHardware(signal);
		} catch (err) {
			if (err instanceof Error && err.name === 'AbortError') return;
			this.error = err instanceof Error ? err.message : 'Failed to load system stats';
		} finally {
			if (!signal.aborted) this.loading = false;
		}
	}

	/** Refetches hardware topology on its own slower cadence. */
	private async refreshHardware(signal: AbortSignal) {
		const now = Date.now();
		if (this.hardware && now - this.lastHardwareFetch < HARDWARE_REFRESH_INTERVAL) return;

		this.lastHardwareFetch = now;
		const hardware = await api.getHardwareStatus(signal);
		if (!signal.aborted) this.hardware = hardware;
	}

	/** Appends a sample, dropping the oldest once the window is full. */
	private appendHistory(stats: SystemStats) {
		const point: HistoryPoint = {
			t: new Date(stats.timestamp).getTime(),
			cpu: stats.cpu.usage_percent,
			memory: stats.memory.used_percent,
			temp: stats.thermal.cpu_temp_c
		};
		const next = [...this.history, point];
		this.history = next.length > HISTORY_LENGTH ? next.slice(-HISTORY_LENGTH) : next;
	}

	/** True once at least one sample has been charted. */
	hasHistory = $derived(this.history.length > 1);

	/** True when the device reports any temperature sensor. */
	hasTemperature = $derived(this.history.some((p) => p.temp > 0));
}

export const diagnostics = new DiagnosticsStore();
