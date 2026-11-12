// Gateway client. The client calls the public gateway only (CONTRACT: Flow).
import { PUBLIC_GATEWAY_URL } from '$app/env/public';

// GET /autocomplete item. CONTRACT.md only says "featured target suggestions";
// this shape is the client's assumption until the contract pins it.
export type Suggestion = { slug: string; name: string; type: string };

// GET /resolve. Only the fields the search needs.
export type Resolved = { ra: number; dec: number };

export class ApiError extends Error {
	constructor(
		message: string,
		readonly code: string,
		readonly status: number
	) {
		super(message);
	}
}

async function get<T>(path: string, params: Record<string, string>): Promise<T> {
	const url = new URL(`/api/v1${path}`, PUBLIC_GATEWAY_URL);
	for (const [k, v] of Object.entries(params)) url.searchParams.set(k, v);

	const res = await fetch(url);
	const body = await res.json().catch(() => null);
	if (!res.ok) {
		// CONTRACT: every non-2xx is {error: {code, message, details}}
		const err = body?.error;
		throw new ApiError(
			err?.message ?? `Request failed (${res.status})`,
			err?.code ?? '',
			res.status
		);
	}
	return body as T;
}

export const autocomplete = (q: string) =>
	get<{ suggestions: Suggestion[] }>('/autocomplete', { q }).then((b) => b.suggestions);

export const resolveName = (name: string) => get<Resolved>('/resolve', { name });
