<!--
	Search box (F1): an object name or "RA Dec" in degrees. Featured names open their
	object page; other names are resolved by the gateway; coordinates open a view directly.
-->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, autocomplete, resolveName } from '#lib/api.js';
	import { parseQuery } from '#lib/search.js';
	import { viewHref, type Target } from '#lib/view-url.js';

	let {
		value = $bindable(''),
		compact = false
	}: {
		value?: string;
		/** Sidebar layout: no button or hint; Enter submits. */
		compact?: boolean;
	} = $props();

	const id = $props.id();
	let error = $state('');
	let busy = $state(false);

	async function featuredSlug(name: string): Promise<string | undefined> {
		const wanted = name.toLowerCase();
		const suggestions = await autocomplete(name).catch(() => []);
		return suggestions.find((s) => s.name.toLowerCase() === wanted)?.slug;
	}

	async function targetFor(name: string): Promise<Target> {
		const slug = await featuredSlug(name);
		if (slug) return { slug };
		const resolved = await resolveName(name);
		return { ra: resolved.ra, dec: resolved.dec, name: resolved.name };
	}

	async function search(e: SubmitEvent) {
		e.preventDefault();
		if (busy) return;
		error = '';
		const query = parseQuery(value);
		if (query.kind === 'invalid') {
			error = query.message;
			return;
		}
		busy = true;
		try {
			const target = query.kind === 'coords' ? query : await targetFor(query.name);
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- viewHref resolves
			await goto(viewHref(target));
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Could not reach the server. Try again.';
		} finally {
			busy = false;
		}
	}
</script>

<form class:compact onsubmit={search} aria-busy={busy}>
	<label class="caption" for="{id}-q">Object name or coordinates</label>
	<div class="row">
		<input
			id="{id}-q"
			class="input"
			type="search"
			autocomplete="off"
			placeholder="Barnard's Star, or 269.452 4.693"
			aria-describedby={compact ? undefined : `${id}-hint`}
			aria-invalid={error ? 'true' : undefined}
			bind:value
		/>
		{#if !compact}
			<button class="btn" type="submit" disabled={busy}>{busy ? 'Searching…' : 'Search'}</button>
		{/if}
	</div>
	{#if error}
		<p class="error" role="alert">{error}</p>
	{/if}
	{#if !compact}
		<p id="{id}-hint" class="caption">Coordinates are RA and Dec in degrees.</p>
	{/if}
</form>

<style>
	form {
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}
	input {
		flex: 1 1 280px;
		min-width: 0;
	}
	.compact input {
		flex-basis: auto;
		background: var(--bg);
	}
	/* The start page uses a larger search box. */
	form:not(.compact) {
		--control-h: 48px;
	}
	form:not(.compact) input {
		padding: 0 16px;
		font-size: 17px;
	}
	form:not(.compact) .btn {
		padding: 0 24px;
		font-size: 16px;
	}
	.error {
		margin: 0;
		font-size: 14px;
		color: var(--danger);
	}
</style>
