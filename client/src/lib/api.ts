// Gateway client. The client calls the public gateway only (CONTRACT: Flow).
// Types come from the pinned gateway schema (H2): edit schema/gateway.openapi.json,
// then run `npm run gen:api`.
import { PUBLIC_GATEWAY_URL } from '$app/env/public';
import type { components } from './gateway-schema';

type Schemas = components['schemas'];
export type Suggestion = Schemas['Suggestion'];
export type Resolved = Schemas['Resolved'];
export type FeaturedTarget = Schemas['FeaturedTarget'];
export type Coverage = Schemas['Coverage'];
export type ViewRequest = Schemas['ViewRequest'];
export type View = Schemas['View'];
export type Frame = Schemas['Frame'];
export type Job = Schemas['Job'];
export type JobAccepted = Schemas['JobAccepted'];
export type Mode = Schemas['Mode'];
export type Warning = Schemas['Warning'];

export class ApiError extends Error {
	constructor(
		message: string,
		readonly code: string,
		readonly status: number
	) {
		super(message);
	}
}

type Options = {
	method?: 'GET' | 'POST';
	params?: Record<string, string | number>;
	body?: unknown;
	signal?: AbortSignal;
};

/** Calls the gateway and returns the HTTP status with the parsed body. Throws ApiError on non-2xx. */
async function request<T>(path: string, opts: Options = {}): Promise<{ status: number; data: T }> {
	const url = new URL(`/api/v1${path}`, PUBLIC_GATEWAY_URL);
	for (const [k, v] of Object.entries(opts.params ?? {})) url.searchParams.set(k, String(v));

	const res = await fetch(url, {
		method: opts.method ?? 'GET',
		signal: opts.signal,
		...(opts.body !== undefined && {
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify(opts.body)
		})
	});
	const data = await res.json().catch(() => null);
	if (!res.ok) {
		// CONTRACT: every non-2xx is {error: {code, message, details}}
		const err = data?.error;
		throw new ApiError(
			err?.message ?? `Request failed (${res.status})`,
			err?.code ?? '',
			res.status
		);
	}
	return { status: res.status, data: data as T };
}

const get = <T>(path: string, opts?: Omit<Options, 'method' | 'body'>) =>
	request<T>(path, opts).then((r) => r.data);

export const autocomplete = (q: string) =>
	get<{ suggestions: Suggestion[] }>('/autocomplete', { params: { q } }).then((b) => b.suggestions);

export const resolveName = (name: string) => get<Resolved>('/resolve', { params: { name } });

export const getObject = (slug: string, signal?: AbortSignal) =>
	get<FeaturedTarget>(`/objects/${encodeURIComponent(slug)}`, { signal });

export const getCoverage = (ra: number, dec: number, signal?: AbortSignal) =>
	get<Coverage>('/coverage', { params: { ra, dec }, signal });

/** POST /views: the view itself when cached (200), otherwise the job building it (202). */
export async function createView(
	body: ViewRequest,
	signal?: AbortSignal
): Promise<{ view: View } | { job: JobAccepted }> {
	const { status, data } = await request<View | JobAccepted>('/views', {
		method: 'POST',
		body,
		signal
	});
	return status === 202 ? { job: data as JobAccepted } : { view: data as View };
}

export const getView = (id: string, signal?: AbortSignal) =>
	get<View>(`/views/${encodeURIComponent(id)}`, { signal });

export const getJob = (id: string, signal?: AbortSignal) =>
	get<Job>(`/jobs/${encodeURIComponent(id)}`, { signal });
