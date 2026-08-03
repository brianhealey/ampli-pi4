// Display formatting helpers for the diagnostics views.

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];

/** Formats a byte count using binary units, e.g. 1536 -> "1.5 KB". */
export function formatBytes(bytes: number, decimals = 1): string {
	if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';

	const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), BYTE_UNITS.length - 1);
	const value = bytes / Math.pow(1024, exponent);
	// Whole bytes never need a decimal point.
	return `${value.toFixed(exponent === 0 ? 0 : decimals)} ${BYTE_UNITS[exponent]}`;
}

/** Formats a duration in seconds as a compact uptime, e.g. "3d 4h 12m". */
export function formatDuration(seconds: number): string {
	if (!Number.isFinite(seconds) || seconds < 1) return '—';

	const days = Math.floor(seconds / 86400);
	const hours = Math.floor((seconds % 86400) / 3600);
	const minutes = Math.floor((seconds % 3600) / 60);
	const secs = Math.floor(seconds % 60);

	if (days > 0) return `${days}d ${hours}h ${minutes}m`;
	if (hours > 0) return `${hours}h ${minutes}m`;
	if (minutes > 0) return `${minutes}m ${secs}s`;
	return `${secs}s`;
}

/** Formats a 0-100 value as a percentage string. */
export function formatPercent(value: number, decimals = 0): string {
	if (!Number.isFinite(value)) return '—';
	return `${value.toFixed(decimals)}%`;
}

/** Formats a temperature in Celsius; the firmware reports -999 for "no sensor". */
export function formatTemp(celsius: number): string {
	if (!Number.isFinite(celsius) || celsius <= -273) return '—';
	return `${celsius.toFixed(1)}°C`;
}

/** Formats a clock speed in MHz, switching to GHz above 1000. */
export function formatFrequency(mhz: number): string {
	if (!Number.isFinite(mhz) || mhz <= 0) return '—';
	return mhz >= 1000 ? `${(mhz / 1000).toFixed(2)} GHz` : `${Math.round(mhz)} MHz`;
}

/** Formats an ISO timestamp as a local wall-clock time with seconds. */
export function formatClockTime(iso: string): string {
	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) return '—';
	return date.toLocaleTimeString(undefined, {
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit'
	});
}

/** Formats an ISO timestamp with milliseconds, for log lines. */
export function formatLogTime(iso: string): string {
	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) return '—';
	const time = date.toLocaleTimeString(undefined, {
		hour12: false,
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit'
	});
	return `${time}.${String(date.getMilliseconds()).padStart(3, '0')}`;
}

/** Formats an ISO timestamp as a full local date and time. */
export function formatDateTime(iso: string): string {
	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) return '—';
	return date.toLocaleString();
}
