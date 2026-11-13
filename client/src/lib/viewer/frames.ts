import type { Frame } from '#lib/api.js';

/** Frames per detector, each list in date order. Detectors ascend. */
export function framesByDetector(frames: readonly Frame[]): Map<number, Frame[]> {
	const out = new Map<number, Frame[]>();
	for (const f of [...frames].sort((a, b) => a.detector - b.detector || a.mjd - b.mjd)) {
		const list = out.get(f.detector);
		if (list) list.push(f);
		else out.set(f.detector, [f]);
	}
	return out;
}

/** Position in `to` of the frame from the same epoch as `from`, or 0 when `to` has none. */
export function sameEpoch(from: Frame | undefined, to: readonly Frame[]): number {
	return Math.max(
		0,
		to.findIndex((f) => f.epoch_index === from?.epoch_index)
	);
}
