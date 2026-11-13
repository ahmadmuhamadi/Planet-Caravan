<script lang="ts">
	import { page } from '$app/state';
	import Viewer from '#lib/viewer/Viewer.svelte';
	import { parseViewState } from '#lib/view-url.js';
	import type { PageProps } from './$types';

	let { params }: PageProps = $props();
</script>

<!-- page.url changes on real navigations only, so each one opens a fresh viewer. -->
{#key page.url.href}
	<Viewer target={{ slug: params.slug }} initial={parseViewState(page.url.searchParams)} />
{/key}
