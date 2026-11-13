import { describe, expect, it } from 'vitest';
import {
	DEFAULT_STATE,
	parseCoords,
	parseViewState,
	viewHref,
	type Target,
	type ViewState
} from './view-url';

const open = (href: string) => new URL(href, 'https://example.test').searchParams;

describe('view links', () => {
	const states: [Target, ViewState][] = [
		[{ slug: 'barnards-star' }, DEFAULT_STATE],
		[{ slug: 'r-draconis' }, { fov: 12.5, mode: 'blink', detector: 3, frame: 2, viewId: 'v42' }],
		[{ ra: 269.452, dec: 4.693 }, DEFAULT_STATE],
		[
			{ ra: 133.7598, dec: -7.24219, name: 'WISE 0855' },
			{ ...DEFAULT_STATE, frame: 2 }
		],
		[
			{ ra: 0, dec: -90 },
			{ fov: 1, mode: 'blink', detector: 6, frame: 7, viewId: null }
		],
		[
			{ ra: 359.99999, dec: 0.000001 },
			{ ...DEFAULT_STATE, detector: 1, viewId: 'a/b c' }
		]
	];

	it.each(states)('round trips %j %j', (target, state) => {
		const href = viewHref(target, state);
		const params = open(href);

		expect(parseViewState(params)).toEqual(state);
		if ('ra' in target) {
			expect(href.startsWith('/search?')).toBe(true);
			expect(parseCoords(params)).toEqual(target);
		} else {
			expect(href.startsWith(`/object/${target.slug}?`)).toBe(true);
		}
	});

	it('always writes fov and mode, and leaves out unset values', () => {
		expect(viewHref({ ra: 10, dec: 20 })).toBe('/search?ra=10&dec=20&fov=3&mode=blink');
		expect(viewHref({ slug: 'm31' })).toBe('/object/m31?fov=3&mode=blink');
	});

	it('falls back to defaults for invalid values', () => {
		const params = open('/search?fov=31&mode=video&det=7&frame=0&id=');
		expect(parseViewState(params)).toEqual(DEFAULT_STATE);
		expect(parseViewState(open('/search?fov=abc&det=1.5&frame=-2'))).toEqual(DEFAULT_STATE);
	});

	it.each(['', '?ra=10', '?ra=&dec=', '?ra=360&dec=0', '?ra=10&dec=91', '?ra=x&dec=1'])(
		'rejects coordinates in %j',
		(search) => {
			expect(parseCoords(open(`/search${search}`))).toBeNull();
		}
	);
});
