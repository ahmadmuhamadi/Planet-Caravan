// Display formatting for positions, wavelengths and angles.

const pad = (n: number, width = 2) => String(n).padStart(width, '0');

/** Right ascension in degrees → "17h 57m 48.5s". */
export function formatRA(deg: number): string {
	// Round once, in tenths of a second, so 59.96s carries into the next minute.
	const tenths = Math.round((((deg % 360) + 360) % 360) * 2400) % (24 * 36000);
	const h = Math.floor(tenths / 36000);
	const m = Math.floor((tenths % 36000) / 600);
	const s = (tenths % 600) / 10;
	return `${pad(h)}h ${pad(m)}m ${s.toFixed(1).padStart(4, '0')}s`;
}

/** Declination in degrees → "+04° 41′ 35″". */
export function formatDec(deg: number): string {
	const sign = deg < 0 ? '−' : '+';
	const total = Math.round(Math.abs(deg) * 3600);
	const d = Math.floor(total / 3600);
	const m = Math.floor((total % 3600) / 60);
	return `${sign}${pad(d)}° ${pad(m)}′ ${pad(total % 60)}″`;
}

/** Wavelength range in micrometres → "1.10–1.65". */
export const formatBand = (min: number, max: number) => `${min.toFixed(2)}–${max.toFixed(2)}`;

/** An angle in arcseconds → "30″", or "2′" from one arcminute up. */
export function formatAngle(arcsec: number): string {
	return arcsec < 60 ? `${+arcsec.toFixed(1)}″` : `${+(arcsec / 60).toFixed(1)}′`;
}

/** Field of view in arcminutes → "3′ × 3′". */
export const formatFov = (arcmin: number) => `${arcmin}′ × ${arcmin}′`;

const SCALE_STEPS = [1, 2, 5, 10, 15, 20, 30, 60, 120, 300, 600, 900, 1800];

/**
 * A round scale bar length for a field `fieldArcsec` wide: the longest step up to a
 * quarter of the field. `fraction` is its share of the field width.
 */
export function scaleBar(fieldArcsec: number): { arcsec: number; fraction: number } {
	const arcsec = SCALE_STEPS.findLast((s) => s <= fieldArcsec / 4) ?? SCALE_STEPS[0];
	return { arcsec, fraction: arcsec / fieldArcsec };
}
