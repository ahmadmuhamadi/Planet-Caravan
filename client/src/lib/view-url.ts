// View links (F7). A link holds everything needed to show the same view again:
//   /object/{slug}?fov=&mode=&det=&frame=&id=
//   /search?ra=&dec=&name=&fov=&mode=&det=&frame=&id=
// `name` is the object a search resolved to, shown instead of bare coordinates.
// fov and mode are always written, so a link keeps its meaning if the defaults change.
// `id` pins the exact view, and with it the data release set it was built from (F8).
import { resolve } from '$app/paths';
import type { Mode } from './api';
import { checkCoords } from './search';

/** Field of view limits in arcminutes (CONTRACT: fov). */
export const FOV = { min: 1, default: 3, max: 30 } as const;
export const DETECTORS = [1, 2, 3, 4, 5, 6] as const;

/** What a view is centred on: a featured target, or a position in degrees. */
export type Target = { slug: string } | Position;
/** A position in degrees, with the name it was found by, if any. */
export type Position = { ra: number; dec: number; name?: string };

/** Everything a link reproduces besides the target. */
export type ViewState = {
	fov: number;
	mode: Mode;
	/** Detector shown; null shows the first one that has frames. */
	detector: number | null;
	/** 1-based position in the blink; null plays from the start. */
	frame: number | null;
	/** The view to reopen; null builds one from the newest data. */
	viewId: string | null;
};

export const DEFAULT_STATE: ViewState = {
	fov: FOV.default,
	mode: 'blink',
	detector: null,
	frame: null,
	viewId: null
};

/** Read-only access is enough; page.url.searchParams is read-only. */
type Params = Pick<URLSearchParams, 'get'>;

const num = (s: string | null) => (s === null || s.trim() === '' ? NaN : Number(s));

const int = (s: string | null, min: number, max = Infinity) => {
	const n = num(s);
	return Number.isInteger(n) && n >= min && n <= max ? n : null;
};

/** Reads the view state from link parameters. Missing or invalid values fall back to defaults. */
export function parseViewState(params: Params): ViewState {
	const fov = num(params.get('fov'));
	return {
		fov: fov >= FOV.min && fov <= FOV.max ? fov : FOV.default,
		// Phase 2 shows blinks only; difference links need epochs A and B (Phase 3).
		mode: 'blink',
		detector: int(params.get('det'), DETECTORS[0], DETECTORS.at(-1)),
		frame: int(params.get('frame'), 1),
		viewId: params.get('id') || null
	};
}

/** Reads the position from link parameters, or null when it is missing or invalid. */
export function parseCoords(params: Params): Position | null {
	const ra = num(params.get('ra'));
	const dec = num(params.get('dec'));
	if (checkCoords(ra, dec)) return null;
	const name = params.get('name')?.trim();
	return name ? { ra, dec, name } : { ra, dec };
}

/** The app-relative link to a view, already passed through resolve(). */
export function viewHref(target: Target, state: ViewState = DEFAULT_STATE): string {
	const params = new URLSearchParams();
	if ('ra' in target) {
		params.set('ra', String(target.ra));
		params.set('dec', String(target.dec));
		if (target.name) params.set('name', target.name);
	}
	params.set('fov', String(state.fov));
	params.set('mode', state.mode);
	if (state.detector !== null) params.set('det', String(state.detector));
	if (state.frame !== null) params.set('frame', String(state.frame));
	if (state.viewId) params.set('id', state.viewId);

	const path =
		'slug' in target ? resolve('/object/[slug]', { slug: target.slug }) : resolve('/search');
	return `${path}?${params}`;
}
