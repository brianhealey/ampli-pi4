// AmpliPi API Types

export interface Source {
	id: number;
	name: string;
	input: string;
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
