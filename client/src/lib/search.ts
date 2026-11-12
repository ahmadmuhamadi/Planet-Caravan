export type Query =
	| { kind: 'coords'; ra: number; dec: number }
	| { kind: 'name'; name: string }
	| { kind: 'invalid'; message: string };

const NUM = String.raw`([+-]?(?:\d+\.?\d*|\.\d+))`;
// "269.452 4.693" or "269.452, 4.693": RA and Dec in decimal degrees.
const COORDS = new RegExp(String.raw`^${NUM}\s*[,\s]\s*${NUM}$`);

/** Decides whether the search box holds coordinates or an object name. */
export function parseQuery(input: string): Query {
	const q = input.trim();
	if (!q) return { kind: 'invalid', message: 'Enter an object name or coordinates.' };

	const m = COORDS.exec(q);
	if (!m) return { kind: 'name', name: q };

	const ra = Number(m[1]);
	const dec = Number(m[2]);
	if (ra < 0 || ra >= 360) return { kind: 'invalid', message: 'RA must be from 0 to 360 degrees.' };
	if (dec < -90 || dec > 90)
		return { kind: 'invalid', message: 'Dec must be from −90 to 90 degrees.' };
	return { kind: 'coords', ra, dec };
}
