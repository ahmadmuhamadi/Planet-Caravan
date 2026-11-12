<script lang="ts">
	import { resolve } from '$app/paths';
	import type { PageProps } from './$types';

	let { params }: PageProps = $props();
</script>

<!-- Placeholder: the viewer (F2–F4, F7) replaces this page. -->
<main style="padding: 24px">
	<p>Viewer for <code>{params.slug}</code> is not built yet.</p>
	<a href={resolve('/')}>Back to search</a>
</main>
