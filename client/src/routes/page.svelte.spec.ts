import { page } from 'vitest/browser';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { goto } from '$app/navigation';
import Page from './+page.svelte';
import StartPage from '#lib/components/StartPage.svelte';

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$app/env/public', () => ({ PUBLIC_GATEWAY_URL: 'http://gateway.test' }));

const featured = [
	{ slug: 'barnards-star', name: "Barnard's Star", type: 'Fast-moving red dwarf' },
	{ slug: 'r-draconis', name: 'R Draconis', type: 'Variable star' }
];

// Stub the gateway: /autocomplete returns `featured`, /resolve returns `resolved`.
let resolved: { status: number; body: unknown };
const json = (status: number, body: unknown) =>
	new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });

beforeEach(() => {
	resolved = { status: 200, body: { name: 'M 31', ra: 10.68, dec: 41.27 } };
	vi.stubGlobal(
		'fetch',
		vi.fn(async (url: URL) => {
			if (url.pathname === '/api/v1/autocomplete') return json(200, { suggestions: featured });
			if (url.pathname === '/api/v1/resolve') return json(resolved.status, resolved.body);
			return json(404, { error: { code: 'not_found', message: 'Not found' } });
		})
	);
});

afterEach(() => {
	vi.unstubAllGlobals();
	vi.mocked(goto).mockClear();
});

const search = async (text: string) => {
	await page.getByLabelText('Object name or coordinates').fill(text);
	await page.getByRole('button', { name: 'Search' }).click();
};

describe('/+page.svelte', () => {
	it('lists featured targets from /autocomplete', async () => {
		render(Page);

		const link = page.getByRole('link', { name: /Barnard's Star/ });
		await expect.element(link).toHaveAttribute('href', '/object/barnards-star');
		expect(document.title).toBe('Home · Planet Caravan');
		await expect.element(page.getByRole('link', { name: /R Draconis/ })).toBeVisible();
	});

	it('opens a view for coordinates without calling the gateway', async () => {
		render(Page);
		await search('269.452 4.693');

		await vi.waitFor(() =>
			expect(goto).toHaveBeenCalledWith('/search?ra=269.452&dec=4.693&fov=3&mode=blink')
		);
		const paths = vi.mocked(fetch).mock.calls.map(([u]) => (u as URL).pathname);
		expect(paths).not.toContain('/api/v1/resolve');
	});

	it('opens the object page for a featured name', async () => {
		render(Page);
		await expect.element(page.getByRole('link', { name: /R Draconis/ })).toBeVisible();
		await search('r draconis');

		await vi.waitFor(() =>
			expect(goto).toHaveBeenCalledWith('/object/r-draconis?fov=3&mode=blink')
		);
	});

	it('resolves other names to a view', async () => {
		render(Page);
		await search('M31');

		await vi.waitFor(() =>
			expect(goto).toHaveBeenCalledWith('/search?ra=10.68&dec=41.27&name=M+31&fov=3&mode=blink')
		);
	});

	it('shows the API error message when a name does not resolve', async () => {
		resolved = {
			status: 404,
			body: { error: { code: 'not_found', message: 'No object named "Xyz"' } }
		};
		render(Page);
		await search('Xyz');

		await expect.element(page.getByRole('alert')).toHaveTextContent('No object named "Xyz"');
		expect(goto).not.toHaveBeenCalled();
	});

	it('rejects out-of-range coordinates', async () => {
		render(Page);
		await search('400 10');

		await expect.element(page.getByRole('alert')).toHaveTextContent('RA must be from 0 to 360');
		expect(goto).not.toHaveBeenCalled();
	});

	it('is also the Search page, and explains a bad link', async () => {
		render(StartPage, { props: { title: 'Search', notice: 'This link has no valid position.' } });

		await expect.element(page.getByRole('alert')).toHaveTextContent('no valid position');
		await expect.element(page.getByLabelText('Object name or coordinates')).toBeVisible();
		expect(document.title).toBe('Search · Planet Caravan');
	});
});
