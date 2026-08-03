// Types for the diagnostics API (/api/system/*), mirroring the Go structs in
// internal/system, internal/logs and internal/models/hardware.go.

export interface HostStats {
	hostname: string;
	model: string;
	serial: string;
	os: string;
	kernel: string;
	architecture: string;
	uptime_seconds: number;
	boot_time: string;
}

export interface CpuStats {
	model: string;
	cores: number;
	usage_percent: number;
	per_core: number[] | null;
	frequency_mhz: number;
	load_avg_1: number;
	load_avg_5: number;
	load_avg_15: number;
	processes: number;
	total_tasks: number;
}

export interface MemoryStats {
	total_bytes: number;
	used_bytes: number;
	free_bytes: number;
	available_bytes: number;
	buffers_bytes: number;
	cached_bytes: number;
	used_percent: number;
	swap_total_bytes: number;
	swap_used_bytes: number;
	swap_percent: number;
}

export interface DiskStats {
	mount: string;
	label: string;
	total_bytes: number;
	used_bytes: number;
	free_bytes: number;
	used_percent: number;
}

export interface NetStats {
	name: string;
	mac: string;
	addresses: string[];
	up: boolean;
	rx_bytes: number;
	tx_bytes: number;
}

export interface ThermalStats {
	cpu_temp_c: number;
	throttled: boolean;
	under_voltage: boolean;
	freq_capped: boolean;
	soft_temp_limit: boolean;
	throttled_ever: boolean;
	under_volt_ever: boolean;
	raw_throttle_hex: string;
}

export interface ProcessStats {
	pid: number;
	go_version: string;
	goroutines: number;
	heap_alloc_bytes: number;
	heap_sys_bytes: number;
	stack_sys_bytes: number;
	gc_count: number;
	uptime_seconds: number;
}

export interface SystemStats {
	timestamp: string;
	host: HostStats;
	cpu: CpuStats;
	memory: MemoryStats;
	disks: DiskStats[];
	network: NetStats[];
	thermal: ThermalStats;
	process: ProcessStats;
}

export interface UnitTemps {
	amp1_c: number;
	amp2_c: number;
	psu1_c: number;
	psu2_c: number;
	pi_c: number;
}

export interface UnitPower {
	pg_9v: boolean;
	en_9v: boolean;
	pg_12v: boolean;
	en_12v: boolean;
	pg_5vd: boolean;
	pg_5va: boolean;
	hv2_present: boolean;
}

export interface UnitFans {
	control: string;
	on: boolean;
	over_temp: boolean;
	failed: boolean;
}

export interface UnitStatus {
	index: number;
	i2c_address: string;
	type: string;
	board_revision: string;
	serial: number;
	firmware_version: string;
	zone_base: number;
	zone_count: number;
	has_analog: boolean;
	rev4_plus: boolean;
	online: boolean;
	error?: string;
	temps?: UnitTemps;
	power?: UnitPower;
	fans?: UnitFans;
}

export interface StreamCapability {
	type: string;
	available: boolean;
	binary?: string;
	reason?: string;
}

export interface HardwareStatus {
	detected: boolean;
	mock: boolean;
	unit_count: number;
	total_zones: number;
	total_sources: number;
	is_streamer: boolean;
	fan_mode: string;
	hv2_present: boolean;
	display: string;
	firmware_version: string;
	physical_outputs: number[];
	units: UnitStatus[];
	stream_support: StreamCapability[];
}

/** The computed daemon info returned by GET /api/info. */
export interface DaemonInfo {
	version: string;
	offline: boolean;
	is_update?: boolean;
	units?: number;
	zones?: number;
	firmware_version?: string;
	fan_mode?: string;
	available_streams?: string[];
}

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface LogEntry {
	time: string;
	level: LogLevel;
	message: string;
	source: string;
	unit?: string;
}

export interface LogSource {
	id: string;
	label: string;
	description: string;
}

export interface LogResult {
	source: string;
	available: boolean;
	reason?: string;
	entries: LogEntry[];
	count: number;
}

export interface LogQuery {
	source: string;
	lines?: number;
	since?: string;
	level?: LogLevel;
	filter?: string;
}
