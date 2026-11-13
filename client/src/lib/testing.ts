// Test helpers: typed gateway fixtures and a fake gateway for `fetch`.
// Fixtures use the generated schema types, so a schema change that breaks them fails `npm run check`.
import type { Coverage, FeaturedTarget, Frame, View } from './api';

export const barnard: FeaturedTarget = {
	slug: 'barnards-star',
	name: "Barnard's Star",
	type: 'High proper motion star',
	ra: 269.452,
	dec: 4.693
};

export const covered: Coverage = {
	covered: true,
	epochs: [],
	releases: [{ name: 'DR1', collection: 'spherex_dr1' }]
};

const bands: Record<number, [number, number]> = {
	1: [0.75, 1.12],
	2: [1.1, 1.65],
	3: [1.63, 2.44]
};

export function frame(epoch: number, detector: number, extra: Partial<Frame> = {}): Frame {
	const [min, max] = bands[detector];
	return {
		id: `e${epoch}-d${detector}`,
		epoch_index: epoch,
		date: `2025-0${epoch + 1}-15`,
		mjd: 60690 + epoch * 90,
		detector,
		wavelength_min_um: min,
		wavelength_max_um: max,
		center_ra: barnard.ra,
		center_dec: barnard.dec,
		pixel_scale_arcsec: 6.2,
		width: 29,
		height: 29,
		orientation_deg: 0,
		alignment_residual_arcsec: 0.4,
		warnings: [],
		url: `https://frames.test/views/v1/e${epoch}-d${detector}.png`,
		...extra
	};
}

/** Three epochs; detector 2 is missing from the last one. */
export function view(extra: Partial<View> = {}): View {
	return {
		id: 'v1',
		target: { ra: barnard.ra, dec: barnard.dec },
		fov_arcmin: 3,
		mode: 'blink',
		epochs: [0, 1, 2].map((i) => ({
			index: i,
			date: `2025-0${i + 1}-15`,
			mjd: 60690 + i * 90,
			detectors: i < 2 ? [1, 2] : [1]
		})),
		frames: [frame(0, 1), frame(1, 1), frame(2, 1), frame(0, 2), frame(1, 2)],
		stretch: { type: 'asinh', pmin: 1, pmax: 99.5 },
		warnings: [],
		created_at: '2026-11-13T10:00:00Z',
		release_set: [{ name: 'DR1', collection: 'spherex_dr1' }],
		newer_data_available: false,
		...extra
	};
}

export const json = (status: number, body: unknown) =>
	new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });

export const notFound = () => json(404, { error: { code: 'not_found', message: 'Not found' } });

type Handler = (url: URL, init?: RequestInit) => Response | Promise<Response>;

/**
 * A fetch stand-in that routes "METHOD /path" (path after /api/v1) to handlers.
 * Unknown routes answer 404 in the contract error format.
 */
export function fakeGateway(routes: Record<string, Handler>) {
	return async (input: URL | string, init?: RequestInit) => {
		const url = new URL(input);
		const key = `${init?.method ?? 'GET'} ${url.pathname.replace('/api/v1', '')}`;
		return (routes[key] ?? notFound)(url, init);
	};
}
