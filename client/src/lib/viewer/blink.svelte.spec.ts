import { flushSync } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import type { View } from '#lib/api.js';
import { view } from '#lib/testing.js';
import { Blink, type BlinkSeed } from './blink.svelte.js';

const PLAY: BlinkSeed = { detector: null, frame: null, autoplay: true };
let cleanup = () => {};

/** A Blink with a 10 ms interval, inside an effect root like a component would give it. */
function blinkOf(v: View | null, seed: BlinkSeed = PLAY): Blink {
	let blink!: Blink;
	cleanup = $effect.root(() => {
		blink = new Blink(() => v, seed, 10);
	});
	flushSync();
	return blink;
}

const settleAll = (blink: Blink) => {
	for (const f of blink.frames) blink.settle(f.id, true);
	flushSync();
};
/** Whether the shown frame changes at any moment within ms (sampling, so a wrap back
 * to the first frame between two checks still counts as a change). */
async function moves(blink: Blink, ms = 60): Promise<boolean> {
	const start = blink.index;
	for (let t = 0; t < ms; t += 3) {
		await new Promise((r) => setTimeout(r, 3));
		if (blink.index !== start) return true;
	}
	return false;
}

afterEach(() => cleanup());

describe('Blink', () => {
	it('shows the first detector with frames, in date order', () => {
		const blink = blinkOf(view());
		expect(blink.detector).toBe(1);
		expect(blink.frames.map((f) => f.id)).toEqual(['e0-d1', 'e1-d1', 'e2-d1']);
	});

	it('waits for every frame image before it plays', async () => {
		const blink = blinkOf(view());
		expect(await moves(blink)).toBe(false);

		blink.settle('e0-d1', true);
		blink.settle('e1-d1', false); // a failed image counts as done
		flushSync();
		expect(await moves(blink)).toBe(false);

		settleAll(blink);
		expect(await moves(blink)).toBe(true);
	});

	it('opens a link paused on its frame, clamped to the frames there are', () => {
		expect(blinkOf(view(), { detector: 2, frame: 2, autoplay: true })).toMatchObject({
			detector: 2,
			index: 1,
			playing: false
		});
		expect(blinkOf(view(), { detector: 1, frame: 9, autoplay: true }).index).toBe(2);
	});

	it('falls back to the first detector when the linked one has no frames', () => {
		expect(blinkOf(view(), { detector: 5, frame: null, autoplay: true }).detector).toBe(1);
	});

	it('does not autoplay when asked not to (reduced motion)', () => {
		expect(blinkOf(view(), { ...PLAY, autoplay: false }).playing).toBe(false);
	});

	it('seeking pauses on that frame', () => {
		const blink = blinkOf(view());
		blink.seek(2);
		expect(blink).toMatchObject({ index: 2, playing: false });
	});

	it('keeps the epoch when switching detector, or starts over when it has none', () => {
		const blink = blinkOf(view(), { detector: 1, frame: 2, autoplay: false });
		blink.selectDetector(2);
		expect(blink.frame?.id).toBe('e1-d2');

		blink.selectDetector(1);
		blink.seek(2); // epoch 2: detector 2 has no frame there
		blink.selectDetector(2);
		expect(blink.frame?.id).toBe('e0-d2');
	});

	it('has nothing to show before the view loads', () => {
		const blink = blinkOf(null);
		expect(blink).toMatchObject({ detector: null, frames: [], index: 0, frame: undefined });
	});
});
