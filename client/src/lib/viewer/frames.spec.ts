import { describe, expect, it } from 'vitest';
import { frame } from '#lib/testing.js';
import { framesByDetector, sameEpoch } from './frames';

describe('framesByDetector', () => {
	it('groups by detector in date order', () => {
		const groups = framesByDetector([frame(2, 2), frame(1, 1), frame(0, 2), frame(0, 1)]);

		expect([...groups.keys()]).toEqual([1, 2]);
		expect(groups.get(1)!.map((f) => f.id)).toEqual(['e0-d1', 'e1-d1']);
		expect(groups.get(2)!.map((f) => f.id)).toEqual(['e0-d2', 'e2-d2']);
	});
});

describe('sameEpoch', () => {
	const d2 = [frame(0, 2), frame(2, 2)];

	it('finds the frame from the same epoch', () => {
		expect(sameEpoch(frame(2, 1), d2)).toBe(1);
	});

	it('falls back to the first frame', () => {
		expect(sameEpoch(frame(1, 1), d2)).toBe(0);
		expect(sameEpoch(undefined, d2)).toBe(0);
	});
});
