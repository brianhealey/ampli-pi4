/**
 * Volume conversion utilities for AmpliPi
 * Handles conversion between dB values, percentages, and normalized float values
 */

/**
 * Default volume range in dB
 */
export const DEFAULT_VOL_MIN = -79;
export const DEFAULT_VOL_MAX = 0;

/**
 * Convert a dB volume value to a percentage (0-100)
 * @param db - Volume in dB
 * @param min - Minimum volume in dB (default: -79)
 * @param max - Maximum volume in dB (default: 0)
 * @returns Percentage value (0-100)
 */
export function dbToPercent(db: number, min: number = DEFAULT_VOL_MIN, max: number = DEFAULT_VOL_MAX): number {
	if (db <= min) return 0;
	if (db >= max) return 100;
	return Math.round(((db - min) / (max - min)) * 100);
}

/**
 * Convert a percentage (0-100) to a dB volume value
 * @param percent - Percentage value (0-100)
 * @param min - Minimum volume in dB (default: -79)
 * @param max - Maximum volume in dB (default: 0)
 * @returns Volume in dB
 */
export function percentToDb(percent: number, min: number = DEFAULT_VOL_MIN, max: number = DEFAULT_VOL_MAX): number {
	return Math.round(min + (percent / 100) * (max - min));
}

/**
 * Convert a normalized float volume (0.0-1.0) to a percentage (0-100)
 * @param volF - Normalized float volume (0.0-1.0)
 * @returns Percentage value (0-100)
 */
export function volFToPercent(volF: number): number {
	return Math.round(volF * 100);
}

/**
 * Convert a percentage (0-100) to a normalized float volume (0.0-1.0)
 * @param percent - Percentage value (0-100)
 * @returns Normalized float volume (0.0-1.0)
 */
export function percentToVolF(percent: number): number {
	return percent / 100;
}

/**
 * Convert a dB volume value to a normalized float (0.0-1.0)
 * @param db - Volume in dB
 * @param min - Minimum volume in dB (default: -79)
 * @param max - Maximum volume in dB (default: 0)
 * @returns Normalized float volume (0.0-1.0)
 */
export function dbToVolF(db: number, min: number = DEFAULT_VOL_MIN, max: number = DEFAULT_VOL_MAX): number {
	if (db <= min) return 0;
	if (db >= max) return 1;
	return (db - min) / (max - min);
}

/**
 * Convert a normalized float volume (0.0-1.0) to dB
 * @param volF - Normalized float volume (0.0-1.0)
 * @param min - Minimum volume in dB (default: -79)
 * @param max - Maximum volume in dB (default: 0)
 * @returns Volume in dB
 */
export function volFToDb(volF: number, min: number = DEFAULT_VOL_MIN, max: number = DEFAULT_VOL_MAX): number {
	return Math.round(min + volF * (max - min));
}

/**
 * Clamp a volume value to a valid range
 * @param vol - Volume value to clamp
 * @param min - Minimum allowed value
 * @param max - Maximum allowed value
 * @returns Clamped volume value
 */
export function clampVolume(vol: number, min: number, max: number): number {
	return Math.max(min, Math.min(max, vol));
}
