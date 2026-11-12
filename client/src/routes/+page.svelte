<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { ApiError, autocomplete, resolveName, type Suggestion } from '#lib/api.js';
	import { parseQuery } from '#lib/search.js';

	let q = $state('');
	let suggestions = $state<Suggestion[]>([]);
	let error = $state('');
	let busy = $state(false);

	// F1: suggest featured targets. An empty query lists all of them.
	$effect(() => {
		const query = q.trim();
		let stale = false;
		const timer = setTimeout(
			() =>
				autocomplete(query)
					.then((s) => !stale && (suggestions = s))
					.catch(() => !stale && (suggestions = [])),
			query ? 200 : 0
		);
		return () => {
			stale = true;
			clearTimeout(timer);
		};
	});

	const openView = (ra: number, dec: number) =>
		goto(resolve(`/view?${new URLSearchParams({ ra: String(ra), dec: String(dec) })}`));

	async function search(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		const query = parseQuery(q);
		if (query.kind === 'invalid') {
			error = query.message;
			return;
		}
		if (query.kind === 'coords') return openView(query.ra, query.dec);

		const name = query.name.toLowerCase();
		const featured = suggestions.find((s) => s.name.toLowerCase() === name);
		if (featured) return goto(resolve('/object/[slug]', { slug: featured.slug }));

		busy = true;
		try {
			const target = await resolveName(query.name);
			await openView(target.ra, target.dec);
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Could not reach the server. Try again.';
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head>
	<title>Planet Caravan</title>
</svelte:head>

<div class="page">
	<header>
		<a class="brand" href={resolve('/')}>Planet Caravan</a>
	</header>

	<main>
		<div class="intro">
			<h1>See how a patch of sky changes over time</h1>
			<p>Aligned frames from NASA's SPHEREx mission, in date order, with a link you can share.</p>
		</div>

		<form onsubmit={search}>
			<label for="q">Object name or coordinates</label>
			<div class="row">
				<input
					id="q"
					type="search"
					autocomplete="off"
					placeholder="Barnard's Star, or 269.452 4.693"
					aria-describedby="q-hint"
					aria-invalid={error ? 'true' : undefined}
					bind:value={q}
				/>
				<button type="submit" disabled={busy}>{busy ? 'Searching…' : 'Search'}</button>
			</div>
			{#if error}
				<p class="error" role="alert">{error}</p>
			{/if}
			<p id="q-hint" class="hint">Coordinates are RA and Dec in degrees.</p>
		</form>

		{#if suggestions.length}
			<section aria-labelledby="featured">
				<h2 id="featured">Featured targets</h2>
				<ul>
					{#each suggestions as s (s.slug)}
						<li>
							<a href={resolve('/object/[slug]', { slug: s.slug })}>
								<span class="name">{s.name}</span>
								<span class="type">{s.type}</span>
							</a>
						</li>
					{/each}
				</ul>
				<p class="hint">Featured targets are prepared in advance and open without a wait.</p>
			</section>
		{/if}
	</main>

	<footer>Data from NASA SPHEREx through IRSA. Not affiliated with or endorsed by NASA.</footer>
</div>

<style>
	.page {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
	}

	header {
		padding: 4px 24px;
		background: var(--surface);
		border-bottom: 1px solid var(--line);
	}
	.brand {
		display: inline-block;
		padding: 12px 0;
		font-weight: 600;
		font-size: 17px;
		color: var(--text);
		text-decoration: none;
	}

	main {
		flex: 1;
		width: 100%;
		max-width: 680px;
		box-sizing: border-box;
		margin: 0 auto;
		padding: 56px 24px;
		display: flex;
		flex-direction: column;
		gap: 32px;
	}

	.intro {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	h1 {
		margin: 0;
		font-size: 32px;
		font-weight: 700;
		line-height: 1.15;
	}
	.intro p {
		margin: 0;
		font-size: 17px;
		color: var(--muted);
	}

	form,
	section {
		margin: 0;
		display: flex;
		flex-direction: column;
	}
	form {
		gap: 6px;
	}
	section {
		gap: 8px;
	}
	label,
	.hint {
		margin: 0;
		font-size: 13px;
		color: var(--muted);
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}
	input {
		flex: 1 1 280px;
		min-width: 0;
		height: 48px;
		box-sizing: border-box;
		padding: 0 16px;
		border-radius: 10px;
		border: 1px solid var(--line2);
		background: var(--surface);
		color: var(--text);
		font-size: 17px;
	}
	button {
		height: 48px;
		padding: 0 24px;
		border: 0;
		border-radius: 10px;
		background: var(--accent);
		color: var(--accentText);
		font-size: 16px;
		font-weight: 600;
		cursor: pointer;
	}
	button:disabled {
		opacity: 0.7;
		cursor: progress;
	}
	.error {
		margin: 0;
		font-size: 14px;
		color: var(--danger);
	}

	h2 {
		margin: 0;
		font-size: 13px;
		font-weight: 600;
		color: var(--muted);
	}
	ul {
		list-style: none;
		margin: 0;
		padding: 0 16px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 12px;
	}
	li + li {
		border-top: 1px solid var(--line);
	}
	li a {
		min-height: 56px;
		padding: 8px 0;
		box-sizing: border-box;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		column-gap: 16px;
		color: var(--text);
		text-decoration: none;
	}
	.name {
		font-size: 16px;
		font-weight: 600;
	}
	.type {
		font-size: 14px;
		color: var(--muted);
	}

	footer {
		padding: 16px 24px;
		border-top: 1px solid var(--line);
		font-size: 13px;
		color: var(--muted);
	}
</style>
