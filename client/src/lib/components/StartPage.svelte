<!--
	The start page: search box and featured targets. Shown at / (Home) and at
	/search when there is no position to open (Search).
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import { autocomplete, type Suggestion } from '#lib/api.js';
	import SearchForm from '#lib/components/SearchForm.svelte';

	let { title, notice = '' }: { title: string; notice?: string } = $props();

	let q = $state('');
	let suggestions = $state<Suggestion[]>([]);

	// F1: suggest featured targets as the user types. An empty query lists all of them.
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
</script>

<svelte:head>
	<title>{title} · Planet Caravan</title>
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

		{#if notice}
			<p class="notice" role="alert">{notice}</p>
		{/if}

		<SearchForm bind:value={q} />

		{#if suggestions.length}
			<section aria-labelledby="featured">
				<h2 id="featured" class="caption">Featured targets</h2>
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
				<p class="caption">Featured targets are prepared in advance and open without a wait.</p>
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

	section {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	h2 {
		font-weight: 600;
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

	.notice {
		margin: 0;
		padding: 10px 14px;
		border-radius: var(--radius);
		background: var(--warnBg);
		color: var(--warnFg);
		font-size: 14px;
	}

	footer {
		padding: 16px 24px;
		border-top: 1px solid var(--line);
		font-size: 13px;
		color: var(--muted);
	}
</style>
