import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from './api';
import { openView } from './open-view';
import { covered, fakeGateway, json, view } from './testing';
import { DEFAULT_STATE } from './view-url';

vi.mock('$app/env/public', () => ({ PUBLIC_GATEWAY_URL: 'http://gateway.test' }));

const position = { ra: 269.452, dec: 4.693 };
const open = (state = DEFAULT_STATE, onProgress?: (p: number) => void) =>
	openView(position, state, { pollMs: 0, onProgress });

const stub = (routes: Parameters<typeof fakeGateway>[0]) => {
	const fetch = vi.fn(fakeGateway(routes));
	vi.stubGlobal('fetch', fetch);
	return fetch;
};
const calls = (fetch: ReturnType<typeof stub>) =>
	fetch.mock.calls.map(([u, init]) => `${init?.method ?? 'GET'} ${new URL(u).pathname}`);

afterEach(() => vi.unstubAllGlobals());

describe('openView', () => {
	it('checks coverage before creating a view (F2)', async () => {
		const fetch = stub({ 'GET /coverage': () => json(200, { ...covered, covered: false }) });

		await expect(open()).resolves.toEqual({ kind: 'no-coverage' });
		expect(calls(fetch)).toEqual(['GET /api/v1/coverage']);
		expect(new URL(fetch.mock.calls[0][0]).search).toBe('?ra=269.452&dec=4.693');
	});

	it('returns a cached view at once', async () => {
		const fetch = stub({
			'GET /coverage': () => json(200, covered),
			'POST /views': () => json(200, view())
		});

		await expect(open()).resolves.toEqual({ kind: 'view', view: view() });
		const body = JSON.parse(fetch.mock.calls[1][1]!.body as string);
		expect(body).toEqual({ target: position, fov_arcmin: 3, mode: 'blink' });
	});

	it('polls the job and reports progress (F3)', async () => {
		const polls = [
			{ id: 'j1', status: 'queued', progress: 0 },
			{ id: 'j1', status: 'running', progress: 40 },
			{ id: 'j1', status: 'done', progress: 100, result: view() }
		];
		stub({
			'GET /coverage': () => json(200, covered),
			'POST /views': () => json(202, { job_id: 'j1', status_url: '/api/v1/jobs/j1' }),
			'GET /jobs/j1': () => json(200, polls.shift())
		});
		const progress: number[] = [];

		await expect(open(DEFAULT_STATE, (p) => progress.push(p))).resolves.toEqual({
			kind: 'view',
			view: view()
		});
		expect(progress).toEqual([0, 0, 40]);
	});

	it('fails with the job error message', async () => {
		stub({
			'GET /coverage': () => json(200, covered),
			'POST /views': () => json(202, { job_id: 'j1', status_url: '/api/v1/jobs/j1' }),
			'GET /jobs/j1': () =>
				json(200, {
					id: 'j1',
					status: 'failed',
					progress: 30,
					error: { code: 'timeout', message: 'The view took too long to build.' }
				})
		});

		const err = await open().catch((e) => e);
		expect(err).toBeInstanceOf(ApiError);
		expect(err).toMatchObject({ message: 'The view took too long to build.', code: 'timeout' });
	});

	it('treats no_coverage from POST /views as no coverage', async () => {
		stub({
			'GET /coverage': () => json(200, covered),
			'POST /views': () =>
				json(422, { error: { code: 'no_coverage', message: 'No SPHEREx data here yet.' } })
		});

		await expect(open()).resolves.toEqual({ kind: 'no-coverage' });
	});

	it('reopens a linked view by id without building one (F8)', async () => {
		const linked = view({ id: 'old', newer_data_available: true });
		const fetch = stub({ 'GET /views/old': () => json(200, linked) });

		await expect(open({ ...DEFAULT_STATE, viewId: 'old' })).resolves.toEqual({
			kind: 'view',
			view: linked
		});
		expect(calls(fetch)).toEqual(['GET /api/v1/views/old']);
	});

	it('builds the view again when the linked one has expired', async () => {
		const fetch = stub({
			'GET /coverage': () => json(200, covered),
			'POST /views': () => json(200, view())
		});

		await expect(open({ ...DEFAULT_STATE, viewId: 'gone' })).resolves.toMatchObject({
			kind: 'view'
		});
		expect(calls(fetch)).toEqual([
			'GET /api/v1/views/gone',
			'GET /api/v1/coverage',
			'POST /api/v1/views'
		]);
	});

	it('stops polling when aborted', async () => {
		stub({
			'GET /coverage': () => json(200, covered),
			'POST /views': () => json(202, { job_id: 'j1', status_url: '/api/v1/jobs/j1' }),
			'GET /jobs/j1': () => json(200, { id: 'j1', status: 'running', progress: 10 })
		});
		const controller = new AbortController();
		const opened = openView(position, DEFAULT_STATE, {
			signal: controller.signal,
			pollMs: 5,
			onProgress: (p) => p === 10 && controller.abort()
		});

		await expect(opened).rejects.toThrow();
	});
});
