<script lang="ts">
	import { page } from '$app/state';
	import StartPage from '#lib/components/StartPage.svelte';
	import Viewer from '#lib/viewer/Viewer.svelte';
	import { parseCoords, parseViewState } from '#lib/view-url.js';

	const params = $derived(page.url.searchParams);
	const target = $derived(parseCoords(params));
	// A bare /search is the search page; a link with a bad position says so.
	const badLink = $derived(params.has('ra') || params.has('dec'));
</script>

<!-- page.url changes on real navigations only, so each one opens a fresh viewer. -->
{#key page.url.href}
	{#if target}
		<Viewer {target} initial={parseViewState(params)} />
	{:else}
		<StartPage
			title="Search"
			notice={badLink
				? 'This link has no valid position: RA must be from 0 to 360 and Dec from −90 to 90 degrees.'
				: ''}
		/>
	{/if}
{/key}
