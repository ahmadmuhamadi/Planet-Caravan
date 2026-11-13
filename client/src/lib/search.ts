export type Query =
	| { kind: 'coords'; ra: number; dec: number }
	| { kind: 'name'; name: string }
	| { kind: 'invalid'; message: string };

const NUM = String.raw`([+-]?(?:\d+\.?\d*|\.\d+))`;
// "269.452 4.693" or "269.452, 4.693": RA and Dec in decimal degrees.
const COORDS = new RegExp(String.raw`^${NUM}\s*[,\s]\s*${NUM}$`);

/** Returns why a position is invalid, or null when it is a valid RA/Dec in degrees. */
export function checkCoords(ra: number, dec: number): string | null {
	if (!(ra >= 0 && ra < 360)) return 'RA must be from 0 to 360 degrees.';
	if (!(dec >= -90 && dec <= 90)) return 'Dec must be from −90 to 90 degrees.';
	return null;
}

/** Decides whether the search box holds coordinates or an object name. */
export function parseQuery(input: string): Query {
	const q = input.trim();
	if (!q) return { kind: 'invalid', message: 'Enter an object name or coordinates.' };

	const m = COORDS.exec(q);
	if (!m) return { kind: 'name', name: q };

	const ra = Number(m[1]);
	const dec = Number(m[2]);
	const problem = checkCoords(ra, dec);
	return problem ? { kind: 'invalid', message: problem } : { kind: 'coords', ra, dec };
}
