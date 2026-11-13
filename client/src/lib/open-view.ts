// Opens a view: reuse the linked one, or check coverage, then get the cached view
// or poll the job that builds it (F2, F3, F8).
import { ApiError, createView, getCoverage, getJob, getView, type View } from './api';
import type { ViewState } from './view-url';

export type Opened = { kind: 'view'; view: View } | { kind: 'no-coverage' };

export type OpenOptions = {
	signal?: AbortSignal;
	/** Called with 0-100 while a job builds the view. */
	onProgress?: (percent: number) => void;
	pollMs?: number;
};

const POLL_MS = 1000;

export async function openView(
	position: { ra: number; dec: number },
	state: Pick<ViewState, 'fov' | 'mode' | 'viewId'>,
	{ signal, onProgress, pollMs = POLL_MS }: OpenOptions = {}
): Promise<Opened> {
	// A linked view opens with the same frames even after a newer release (F8).
	// If it has expired, fall through and build it again from the newest data.
	if (state.viewId) {
		try {
			return { kind: 'view', view: await getView(state.viewId, signal) };
		} catch (err) {
			if (!(err instanceof ApiError && err.status === 404)) throw err;
		}
	}

	const coverage = await getCoverage(position.ra, position.dec, signal);
	if (!coverage.covered) return { kind: 'no-coverage' };

	let created;
	try {
		created = await createView(
			{ target: position, fov_arcmin: state.fov, mode: state.mode },
			signal
		);
	} catch (err) {
		// Coverage can change between the two calls.
		if (err instanceof ApiError && err.code === 'no_coverage') return { kind: 'no-coverage' };
		throw err;
	}
	if ('view' in created) return { kind: 'view', view: created.view };

	onProgress?.(0);
	for (;;) {
		await delay(pollMs, signal);
		const job = await getJob(created.job.job_id, signal);
		if (job.status === 'done' && job.result) return { kind: 'view', view: job.result };
		if (job.status === 'failed' || job.status === 'done') {
			throw new ApiError(
				job.error?.message ?? 'The view could not be built. Please try again.',
				job.error?.code ?? 'job_failed',
				200
			);
		}
		onProgress?.(job.progress);
	}
}

function delay(ms: number, signal?: AbortSignal): Promise<void> {
	return new Promise((resolve, reject) => {
		signal?.throwIfAborted();
		const timer = setTimeout(resolve, ms);
		signal?.addEventListener(
			'abort',
			() => {
				clearTimeout(timer);
				reject(signal.reason);
			},
			{ once: true }
		);
	});
}
