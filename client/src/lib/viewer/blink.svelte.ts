// Blink playback (F4): which detector and frame are shown, and whether it plays.
// Create it while a component initialises: its timer is an effect of that component.
import { SvelteSet } from 'svelte/reactivity';
import type { Frame, View } from '#lib/api.js';
import { framesByDetector, sameEpoch } from './frames.js';

export type BlinkSeed = {
	/** Detector to show; null or missing: the first with frames. */
	detector: number | null;
	/** 1-based frame to open on, paused; null: play from the start. */
	frame: number | null;
	autoplay: boolean;
};

export class Blink {
	playing = $state(false);
	/** Frame images that loaded or failed; playback waits for all of the shown detector's. */
	readonly settled = new SvelteSet<string>();
	readonly failed = new SvelteSet<string>();

	#detector = $state<number | null>(null);
	#index = $state(0);

	readonly byDetector: Map<number, Frame[]>;
	readonly detector: number | null;
	readonly frames: Frame[];
	/** Clamped, so a link to frame 9 of a 3-frame blink shows the last frame. */
	readonly index: number;
	readonly frame: Frame | undefined;
	readonly ready: boolean;

	constructor(view: () => View | null, seed: BlinkSeed, intervalMs = 900) {
		this.#detector = seed.detector;
		this.#index = Math.max(0, (seed.frame ?? 1) - 1);
		this.playing = seed.frame === null && seed.autoplay;

		this.byDetector = $derived(framesByDetector(view()?.frames ?? []));
		this.detector = $derived.by(() => {
			const d = this.#detector;
			return d !== null && this.byDetector.has(d)
				? d
				: (this.byDetector.keys().next().value ?? null);
		});
		this.frames = $derived(
			this.detector === null ? [] : (this.byDetector.get(this.detector) ?? [])
		);
		this.index = $derived(Math.max(0, Math.min(this.#index, this.frames.length - 1)));
		this.frame = $derived(this.frames[this.index]);
		this.ready = $derived(this.frames.every((f) => this.settled.has(f.id)));

		$effect(() => {
			if (!this.playing || !this.ready || this.frames.length < 2) return;
			const timer = setInterval(() => this.#step(), intervalMs);
			return () => clearInterval(timer);
		});
	}

	toggle(): void {
		this.playing = !this.playing;
	}

	pause(): void {
		this.playing = false;
	}

	/** Shows frame i (0-based) and pauses. */
	seek(i: number): void {
		this.#index = i;
		this.playing = false;
	}

	/** Switches detector, staying on the same epoch when it has a frame there. */
	selectDetector(d: number): void {
		const next = this.byDetector.get(d);
		if (!next) return;
		this.#index = sameEpoch(this.frame, next);
		this.#detector = d;
	}

	/** Records that a frame image finished loading, or failed to. */
	settle(id: string, ok: boolean): void {
		this.settled.add(id);
		if (!ok) this.failed.add(id);
	}

	#step(): void {
		this.#index = (this.index + 1) % this.frames.length;
	}
}
