// Loads the view for a target: featured lookup, then coverage, then the cached
// view or the job building it (F2, F3, F8). Holds the result as reactive state.
import { ApiError, getObject, type FeaturedTarget, type View } from '#lib/api.js';
import { openView } from '#lib/open-view.js';
import type { Target, ViewState } from '#lib/view-url.js';

export type Phase =
	| { kind: 'loading'; progress: number | null }
	| { kind: 'no-coverage' }
	| { kind: 'error'; message: string }
	| { kind: 'ready'; view: View };

export class ViewLoader {
	phase = $state<Phase>({ kind: 'loading', progress: null });
	/** The featured target, once known (object links only). */
	featured = $state<FeaturedTarget | null>(null);

	readonly #target: Target;
	readonly #request: Pick<ViewState, 'fov' | 'mode' | 'viewId'>;
	#controller: AbortController | undefined;

	constructor(target: Target, request: Pick<ViewState, 'fov' | 'mode' | 'viewId'>) {
		this.#target = target;
		this.#request = request;
	}

	get view(): View | null {
		return this.phase.kind === 'ready' ? this.phase.view : null;
	}

	/** Where the view is centred, once known. */
	get position(): { ra: number; dec: number } | null {
		if (!('ra' in this.#target)) return this.featured;
		const { ra, dec } = this.#target; // the name is for display, not part of the request
		return { ra, dec };
	}

	/** Starts (or restarts) loading; an earlier load still running is abandoned. */
	async load(): Promise<void> {
		this.stop();
		const { signal } = (this.#controller = new AbortController());
		this.phase = { kind: 'loading', progress: null };
		try {
			const position = this.position ?? (this.featured = await this.#featured(signal));
			const opened = await openView(position, this.#request, {
				signal,
				onProgress: (progress) => (this.phase = { kind: 'loading', progress })
			});
			this.phase = opened.kind === 'view' ? { kind: 'ready', view: opened.view } : opened;
		} catch (err) {
			if (signal.aborted) return;
			this.phase = {
				kind: 'error',
				message: err instanceof ApiError ? err.message : 'Could not reach the server. Try again.'
			};
		}
	}

	stop(): void {
		this.#controller?.abort();
	}

	#featured(signal: AbortSignal): Promise<FeaturedTarget> {
		if (!('slug' in this.#target)) throw new Error('unreachable: coordinates have a position');
		return getObject(this.#target.slug, signal);
	}
}
