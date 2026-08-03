// AmpliPi API Client

import type {
	State,
	Source,
	SourceUpdate,
	Zone,
	ZoneUpdate,
	Group,
	GroupUpdate,
	Stream,
	StreamCreate,
	StreamUpdate,
	Preset,
	PresetCreate,
	PresetUpdate
} from './types';
import type {
	SystemStats,
	HardwareStatus,
	LogResult,
	LogSource,
	LogQuery,
	DaemonInfo
} from './diagnostics';

const API_BASE = '/api';

class APIError extends Error {
	constructor(
		message: string,
		public status: number,
		public data?: unknown
	) {
		super(message);
		this.name = 'APIError';
	}
}

async function request<T>(
	endpoint: string,
	options: RequestInit = {},
	signal?: AbortSignal
): Promise<T> {
	const url = `${API_BASE}${endpoint}`;
	const response = await fetch(url, {
		...options,
		signal,
		headers: {
			'Content-Type': 'application/json',
			...options.headers
		}
	});

	if (!response.ok) {
		const text = await response.text();
		throw new APIError(
			`API request failed: ${response.statusText}`,
			response.status,
			text
		);
	}

	// Handle no-content responses
	if (response.status === 204) {
		return {} as T;
	}

	return response.json();
}

export const api = {
	// System state
	getState(signal?: AbortSignal): Promise<State> {
		return request<State>('/', {}, signal);
	},

	// Sources
	getSources(): Promise<{ sources: Source[] }> {
		return request('/sources');
	},

	getSource(id: number): Promise<Source> {
		return request(`/sources/${id}`);
	},

	updateSource(id: number, update: SourceUpdate): Promise<State> {
		return request<State>(`/sources/${id}`, {
			method: 'PATCH',
			body: JSON.stringify(update)
		});
	},

	// Zones
	getZones(): Promise<{ zones: Zone[] }> {
		return request('/zones');
	},

	getZone(id: number): Promise<Zone> {
		return request(`/zones/${id}`);
	},

	updateZone(id: number, update: ZoneUpdate): Promise<State> {
		return request<State>(`/zones/${id}`, {
			method: 'PATCH',
			body: JSON.stringify(update)
		});
	},

	updateZones(zones: number[], update: ZoneUpdate): Promise<State> {
		return request<State>('/zones', {
			method: 'PATCH',
			body: JSON.stringify({ zones, update })
		});
	},

	// Groups
	getGroups(): Promise<{ groups: Group[] }> {
		return request('/groups');
	},

	getGroup(id: number): Promise<Group> {
		return request(`/groups/${id}`);
	},

	createGroup(group: GroupUpdate): Promise<State> {
		return request<State>('/group', {
			method: 'POST',
			body: JSON.stringify(group)
		});
	},

	updateGroup(id: number, update: GroupUpdate): Promise<State> {
		return request<State>(`/groups/${id}`, {
			method: 'PATCH',
			body: JSON.stringify(update)
		});
	},

	deleteGroup(id: number): Promise<State> {
		return request<State>(`/groups/${id}`, {
			method: 'DELETE'
		});
	},

	// Streams
	getStreams(): Promise<{ streams: Stream[] }> {
		return request('/streams');
	},

	getStream(id: number): Promise<Stream> {
		return request(`/streams/${id}`);
	},

	createStream(stream: StreamCreate): Promise<State> {
		return request<State>('/stream', {
			method: 'POST',
			body: JSON.stringify(stream)
		});
	},

	updateStream(id: number, update: StreamUpdate): Promise<State> {
		return request<State>(`/streams/${id}`, {
			method: 'PATCH',
			body: JSON.stringify(update)
		});
	},

	deleteStream(id: number): Promise<State> {
		return request<State>(`/streams/${id}`, {
			method: 'DELETE'
		});
	},

	execStreamCommand(id: number, command: string): Promise<State> {
		return request<State>(`/streams/${id}/${command}`, {
			method: 'POST'
		});
	},

	// Presets
	getPresets(): Promise<{ presets: Preset[] }> {
		return request('/presets');
	},

	getPreset(id: number): Promise<Preset> {
		return request(`/presets/${id}`);
	},

	createPreset(preset: PresetCreate): Promise<State> {
		return request<State>('/preset', {
			method: 'POST',
			body: JSON.stringify(preset)
		});
	},

	updatePreset(id: number, update: PresetUpdate): Promise<State> {
		return request<State>(`/presets/${id}`, {
			method: 'PATCH',
			body: JSON.stringify(update)
		});
	},

	deletePreset(id: number): Promise<State> {
		return request<State>(`/presets/${id}`, {
			method: 'DELETE'
		});
	},

	loadPreset(id: number): Promise<State> {
		return request<State>(`/presets/${id}/load`, {
			method: 'POST'
		});
	},

	// System. Note this returns the computed Info directly, unlike GET /api
	// which nests the persisted (and version-less) info block inside State.
	getInfo(): Promise<DaemonInfo> {
		return request<DaemonInfo>('/info');
	},

	factoryReset(): Promise<State> {
		return request<State>('/factory_reset', {
			method: 'POST'
		});
	},

	// Diagnostics
	getSystemStats(signal?: AbortSignal): Promise<SystemStats> {
		return request<SystemStats>('/system/stats', {}, signal);
	},

	getHardwareStatus(signal?: AbortSignal): Promise<HardwareStatus> {
		return request<HardwareStatus>('/system/hardware', {}, signal);
	},

	getLogSources(): Promise<{ sources: LogSource[] }> {
		return request('/system/logs/sources');
	},

	getLogs(query: LogQuery, signal?: AbortSignal): Promise<LogResult> {
		const params = new URLSearchParams({ source: query.source });
		if (query.lines) params.set('lines', String(query.lines));
		if (query.since) params.set('since', query.since);
		if (query.level) params.set('level', query.level);
		if (query.filter) params.set('filter', query.filter);
		return request<LogResult>(`/system/logs?${params}`, {}, signal);
	}
};
