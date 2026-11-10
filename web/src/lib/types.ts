// AmpliPi API Types

export interface Source {
	id: number;
	name: string;
	input: string;
	// false when no DAC feeds this source on this unit: only its analog RCA
	// input can play here. Omitted when the source has a DAC.
	has_dac?: boolean;
	// Parametric EQ for digital streams on this source. Omitted when never set.
	eq?: SourceEQ;
	// Whether the EQ is being applied; only present when eq is set.
	eq_status?: EQStatus;
}

export type EQBandType = 'peaking' | 'lowshelf' | 'highshelf';

export interface EQBand {
	type: EQBandType;
	freq: number; // Hz, 20-20000
	gain: number; // dB, -12..12
	q: number; // 0.1-10
}

export interface SourceEQ {
	enabled: boolean;
	preamp_db: number; // -24..0
	bands: EQBand[]; // at most 10
}

export type EQState =
	| 'off'
	| 'active'
	| 'starting'
	| 'idle'
	| 'analog'
	| 'no_dac'
	| 'unavailable'
	| 'failed';

export interface EQStatus {
	state: EQState;
	reason?: string;
}

export interface Zone {
	id: number;
	name: string;
	source_id: number;
	mute: boolean;
	vol: number;
	vol_f: number;
	vol_min: number;
	vol_max: number;
	disabled: boolean;
}

export interface Group {
	id: number;
	name: string;
	zones: number[];
	source_id?: number;
	vol_delta?: number;
	vol_f?: number;
	mute?: boolean;
}

export interface StreamInfo {
	name: string;
	state: string;
	track?: string;
	artist?: string;
	album?: string;
	station?: string;
	img_url?: string;
	rating?: number;
}

export interface Stream {
	id: number;
	name: string;
	type: string;
	info?: StreamInfo;
	config?: Record<string, unknown>;
	disabled?: boolean;
	browsable?: boolean;
}

export interface PresetState {
	sources?: Partial<Source>[];
	zones?: Partial<Zone>[];
	groups?: Partial<Group>[];
}

export interface Command {
	endpoint: string;
	method: string;
	data?: Record<string, unknown>;
}

export interface Preset {
	id: number;
	name: string;
	state?: PresetState;
	commands?: Command[];
}

export interface Info {
	version: string;
	unit_id?: number;
	is_update?: boolean;
	offline: boolean;
	units?: number;
	zones?: number;
	firmware_version?: string;
	fan_mode?: string;
	available_streams?: string[];
	eq_available?: boolean; // CamillaDSP installed: per-source EQ can run
}

export interface State {
	sources: Source[];
	zones: Zone[];
	groups: Group[];
	streams: Stream[];
	presets: Preset[];
	info: Info;
}

// Update request types
export interface SourceUpdate {
	name?: string;
	input?: string;
	eq?: SourceEQ; // replaces the whole EQ; omit to leave it unchanged
}

export interface ZoneUpdate {
	name?: string;
	source_id?: number;
	mute?: boolean;
	vol?: number;
	vol_f?: number;
	vol_delta_f?: number;
	disabled?: boolean;
}

export interface GroupUpdate {
	name?: string;
	zones?: number[];
	source_id?: number;
	vol_delta?: number;
	vol_f?: number;
	mute?: boolean;
}

export interface StreamCreate {
	name: string;
	type: string;
	config?: Record<string, unknown>;
}

export interface StreamUpdate {
	name?: string;
	config?: Record<string, unknown>;
}

export interface PresetCreate {
	name: string;
	state?: PresetState;
	commands?: Command[];
}

export interface PresetUpdate {
	name?: string;
	state?: PresetState;
	commands?: Command[];
}
