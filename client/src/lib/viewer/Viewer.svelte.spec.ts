import { page } from 'vitest/browser';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { goto } from '$app/navigation';
import type { View } from '#lib/api.js';
import { barnard, covered, fakeGateway, frame, json, view } from '#lib/testing.js';
import { DEFAULT_STATE, viewHref, type Target, type ViewState } from '#lib/view-url.js';
import Viewer from './Viewer.svelte';

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$app/env/public', () => ({ PUBLIC_GATEWAY_URL: 'http://gateway.test' }));

const coords = { ra: barnard.ra, dec: barnard.dec };
const featured: Target = { slug: barnard.slug };

/** A gateway that has `v` cached for the featured target. */
const cached = (v: View = view()) =>
	vi.stubGlobal(
		'fetch',
		vi.fn(
			fakeGateway({
				'GET /objects/barnards-star': () => json(200, barnard),
				'GET /coverage': () => json(200, covered),
				'POST /views': () => json(200, v),
				[`GET /views/${v.id}`]: () => json(200, v)
			})
		)
	);

const open = (target: Target, initial: ViewState = DEFAULT_STATE) =>
	render(Viewer, { props: { target, initial } });

const epochLabel = () => page.getByText(/^Epoch \d of \d/);
const shareLink = () => page.getByLabelText('Link to this view');

afterEach(() => {
	vi.unstubAllGlobals();
	vi.mocked(goto).mockClear();
});

describe('Viewer states', () => {
	it('says when there is no SPHEREx data (F2)', async () => {
		vi.stubGlobal(
			'fetch',
			fakeGateway({ 'GET /coverage': () => json(200, { ...covered, covered: false }) })
		);
		open(coords);

		await expect.element(page.getByText('No SPHEREx data here yet')).toBeVisible();
		await expect.element(page.getByRole('heading', { name: '269.452 4.693' })).toBeVisible();
	});

	it('shows progress, then the error message of a failed job (F3)', async () => {
		let polls = 0;
		vi.stubGlobal(
			'fetch',
			fakeGateway({
				'GET /coverage': () => json(200, covered),
				'POST /views': () => json(202, { job_id: 'j1', status_url: '/api/v1/jobs/j1' }),
				'GET /jobs/j1': () =>
					json(
						200,
						++polls === 1
							? { id: 'j1', status: 'running', progress: 40 }
							: {
									id: 'j1',
									status: 'failed',
									progress: 40,
									error: { code: 'timeout', message: 'The view took too long to build.' }
								}
					)
			})
		);
		open(coords);

		await expect.element(page.getByText('40%')).toBeVisible();
		await expect.element(page.getByRole('progressbar')).toHaveAttribute('value', '40');
		await expect
			.element(page.getByRole('alert'), { timeout: 4000 })
			.toHaveTextContent('The view took too long to build.');
		await expect.element(page.getByRole('button', { name: 'Try again' })).toBeVisible();
	});

	it('explains a link with an unknown featured target', async () => {
		vi.stubGlobal('fetch', fakeGateway({}));
		open({ slug: 'nope' });

		await expect.element(page.getByRole('alert')).toHaveTextContent('Not found');
	});
});

describe('Blink player (F4)', () => {
	it('opens a featured target from cache and plays its blink', async () => {
		cached();
		open(featured);

		await expect.element(page.getByRole('heading', { name: "Barnard's Star" })).toBeVisible();
		await expect.element(page.getByText('High proper motion star')).toBeVisible();
		expect(document.title).toBe("Barnard's Star · Search · Planet Caravan");
		await expect.element(page.getByRole('button', { name: 'Pause' })).toBeVisible();
		await expect.element(epochLabel()).toHaveTextContent(/of 3 · alignment residual 0.4″/);
		await expect.element(page.getByText('DR1')).toBeVisible();
		// Detector 1 is shown first; detector 3 has no frames in this view.
		await expect
			.element(page.getByRole('button', { name: /^Detector 1, 0.75–1.12/ }))
			.toHaveAttribute('aria-pressed', 'true');
		await expect.element(page.getByRole('button', { name: /^Detector 3/ })).toBeDisabled();
	});

	it('shows the date and wavelength range of each frame', async () => {
		cached();
		open(featured, { ...DEFAULT_STATE, frame: 2 });

		await expect.element(page.getByText('2025-02-15 · 0.75–1.12 µm')).toBeVisible();
		await expect.element(epochLabel()).toHaveTextContent('Epoch 2 of 3');
		await expect
			.element(page.getByRole('img', { name: /^Sky frame, Epoch 2 of 3, 2025-02-15/ }))
			.toBeVisible();
	});

	it('draws every frame in the same box', async () => {
		cached();
		open(featured, { ...DEFAULT_STATE, frame: 1 });
		await expect.element(epochLabel()).toBeVisible();

		const boxes = [...document.querySelectorAll('figure img')].map((img) =>
			JSON.stringify(img.getBoundingClientRect())
		);
		expect(boxes).toHaveLength(3);
		expect(new Set(boxes).size).toBe(1);
	});

	it('pauses on the slider and keeps the epoch when the detector changes', async () => {
		cached();
		open(featured, { ...DEFAULT_STATE, frame: 1 });
		await expect.element(page.getByRole('button', { name: 'Play' })).toBeVisible();

		await page.getByRole('slider').fill('1');
		await expect.element(epochLabel()).toHaveTextContent('Epoch 2 of 3');

		await page.getByRole('button', { name: /^Detector 2/ }).click();
		await expect.element(page.getByText('2025-02-15 · 1.10–1.65 µm')).toBeVisible();
		await expect
			.element(page.getByText('This detector has frames from 2 of 3 epochs.'))
			.toBeVisible();
	});
});

describe('Links (F7, F8)', () => {
	it('opening a copied link restores the same view', async () => {
		cached();
		const state: ViewState = { ...DEFAULT_STATE, detector: 2, frame: 2, viewId: 'v1' };
		const href = viewHref(featured, state);
		open(featured, state);

		await expect.element(shareLink()).toHaveValue(new URL(href, location.origin).href);
		await expect.element(epochLabel()).toHaveTextContent('Epoch 2 of 3');
		await expect
			.element(page.getByRole('button', { name: /^Detector 2/ }))
			.toHaveAttribute('aria-pressed', 'true');
		await expect.element(page.getByRole('button', { name: 'Play' })).toBeVisible();
		// The view came from its id, not from a new build.
		const paths = vi.mocked(fetch).mock.calls.map(([u]) => new URL(u as URL).pathname);
		expect(paths).not.toContain('/api/v1/views');
	});

	it('keeps the address bar on the view shown', async () => {
		cached();
		open(featured);
		await expect.element(page.getByRole('button', { name: 'Pause' })).toBeVisible();

		const playing = '/object/barnards-star?fov=3&mode=blink&det=1&id=v1';
		await vi.waitFor(() =>
			expect(goto).toHaveBeenLastCalledWith(playing, { shallow: true, replace: true })
		);

		await page.getByRole('button', { name: 'Pause' }).click();
		await vi.waitFor(() =>
			expect(goto).toHaveBeenLastCalledWith(expect.stringMatching(/&det=1&frame=\d&id=v1$/), {
				shallow: true,
				replace: true
			})
		);
	});

	it('copying pauses, so the link holds the frame on screen', async () => {
		cached();
		const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
		open(featured);
		await expect.element(page.getByRole('button', { name: 'Pause' })).toBeVisible();

		await page.getByRole('button', { name: 'Copy link' }).click();

		await expect.element(page.getByText('Link copied to the clipboard.')).toBeVisible();
		expect(writeText).toHaveBeenCalledWith(expect.stringMatching(/&frame=\d&id=v1$/));
		await expect.element(page.getByRole('button', { name: 'Play' })).toBeVisible();
		writeText.mockRestore();
	});

	it('notes newer data on an older link, and links to the newest view', async () => {
		cached(view({ newer_data_available: true }));
		open(featured, { ...DEFAULT_STATE, viewId: 'v1' });

		await expect.element(page.getByText('Newer data available.')).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Open with the newest data' }))
			.toHaveAttribute('href', '/object/barnards-star?fov=3&mode=blink&det=1');
	});
});

describe('Warnings (F9)', () => {
	it('shows frame warnings on the frame and view warnings above it', async () => {
		cached(
			view({
				warnings: ['catalog_check_incomplete'],
				frames: [frame(0, 1), frame(1, 1, { warnings: ['low_alignment_quality'] })]
			})
		);
		open(featured, { ...DEFAULT_STATE, frame: 1 });

		await expect.element(page.getByText('Catalogue check incomplete.')).toBeVisible();
		const frameNotices = page.getByRole('list', { name: 'Notices for this frame' });
		await expect.element(frameNotices).not.toBeInTheDocument();

		await page.getByRole('slider').fill('1');
		await expect.element(frameNotices).toHaveTextContent('Low alignment quality');
	});
});
