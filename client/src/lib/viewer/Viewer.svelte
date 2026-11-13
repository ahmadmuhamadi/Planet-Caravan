<!--
	The viewer page (Phase 2): loads the view for a target and shows its blink.
	State comes from the link once, at mount; the route remounts this component on
	every real navigation. From then on the address bar follows the view on screen (F7).
-->
<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { prefersReducedMotion } from 'svelte/motion';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import SearchForm from '#lib/components/SearchForm.svelte';
	import { formatBand } from '#lib/format.js';
	import { viewHref, type Target, type ViewState } from '#lib/view-url.js';
	import { warningText } from '#lib/warnings.js';
	import { Blink } from './blink.svelte.js';
	import BlinkPlayer from './BlinkPlayer.svelte';
	import DetectorPicker from './DetectorPicker.svelte';
	import Notice from './Notice.svelte';
	import ShareLink from './ShareLink.svelte';
	import StatusPanel from './StatusPanel.svelte';
	import TargetSummary from './TargetSummary.svelte';
	import { ViewLoader } from './view-loader.svelte.js';

	let { target, initial }: { target: Target; initial: ViewState } = $props();

	// The link is read once; the route remounts this component on navigation.
	const link = untrack(() => ({ target, initial }));
	const loader = new ViewLoader(link.target, link.initial);
	const blink = new Blink(() => loader.view, {
		detector: link.initial.detector,
		frame: link.initial.frame,
		autoplay: !prefersReducedMotion.current
	});

	onMount(() => {
		loader.load();
		return () => loader.stop();
	});

	const view = $derived(loader.view);
	const position = $derived(loader.position);
	const targetName = 'name' in link.target ? link.target.name : undefined;
	const name = $derived(
		loader.featured?.name ??
			targetName ??
			view?.target.name ??
			(position ? `${position.ra.toFixed(3)} ${position.dec.toFixed(3)}` : '')
	);
	const searchText = $derived(
		loader.featured?.name ??
			targetName ??
			('ra' in link.target ? `${link.target.ra} ${link.target.dec}` : '')
	);
	const bands = $derived(
		new Map(
			[...blink.byDetector].map(([d, [f]]) => [
				d,
				formatBand(f.wavelength_min_um, f.wavelength_max_um)
			])
		)
	);

	const linkState = $derived<ViewState>({
		fov: view?.fov_arcmin ?? link.initial.fov,
		mode: view?.mode ?? link.initial.mode,
		detector: blink.detector,
		// While playing, a link plays from the start; once paused it holds the frame shown.
		frame: blink.playing || !blink.frames.length ? null : blink.index + 1,
		viewId: view?.id ?? link.initial.viewId
	});
	const href = $derived(viewHref(link.target, linkState));
	const latestHref = $derived(viewHref(link.target, { ...linkState, frame: null, viewId: null }));

	// Keep the address bar on the view shown, without adding history entries.
	// Debounced: dragging the slider would otherwise replace the URL on every step.
	$effect(() => {
		if (!view) return;
		const next = href;
		const timer = setTimeout(() => {
			if (next !== location.pathname + location.search) {
				// eslint-disable-next-line svelte/no-navigation-without-resolve -- viewHref resolves
				goto(next, { shallow: true, replace: true });
			}
		}, 250);
		return () => clearTimeout(timer);
	});
</script>

<svelte:head>
	<title>{name ? `${name} · Search · Planet Caravan` : 'Search · Planet Caravan'}</title>
</svelte:head>

<div class="viewer">
	<aside>
		<a class="brand" href={resolve('/')}>Planet Caravan</a>
		<SearchForm compact value={searchText} />

		{#if position}
			<TargetSummary
				{name}
				type={loader.featured?.type}
				ra={position.ra}
				dec={position.dec}
				fov={linkState.fov}
				epochs={view?.epochs.length ?? null}
				releases={view?.release_set?.map((r) => r.name) ?? null}
			/>
		{/if}

		{#if blink.detector !== null}
			<DetectorPicker {bands} selected={blink.detector} onselect={(d) => blink.selectDetector(d)} />
		{/if}
	</aside>

	<main>
		{#if view && blink.frames.length}
			<div class="column">
				<ShareLink url={new URL(href, location.origin).href} onbeforecopy={() => blink.pause()} />
				{#if view.newer_data_available}
					<Notice tone="info" title="Newer data available.">
						<span>A data release came out after this view was made.</span>
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- viewHref resolves -->
						<a href={latestHref}>Open with the newest data</a>
					</Notice>
				{/if}
				{#each view.warnings as code (code)}
					{@const text = warningText(code)}
					<Notice title="{text.label}.">{text.help}</Notice>
				{/each}
			</div>
			<BlinkPlayer {blink} epochs={view.epochs} />
		{:else}
			<StatusPanel phase={loader.phase} onretry={() => loader.load()} />
		{/if}
	</main>
</div>

<style>
	.viewer {
		min-height: 100vh;
		display: flex;
		flex-wrap: wrap;
	}

	aside {
		flex: 1 1 360px;
		box-sizing: border-box;
		padding: 20px 24px;
		background: var(--surface);
		border-right: 1px solid var(--line);
		display: flex;
		flex-direction: column;
		gap: 20px;
	}
	.brand {
		align-self: flex-start;
		padding: 10px 0;
		font-weight: 600;
		font-size: 17px;
		color: var(--text);
	}

	main {
		flex: 999 1 560px;
		min-width: 0;
		box-sizing: border-box;
		padding: 20px 24px;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 14px;
	}
	.column {
		width: 100%;
		max-width: 640px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
</style>
