<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
</script>

<!-- Placeholder: the viewer (F2–F4, F7) replaces this page. -->
<main style="padding: 24px">
	<p>
		Viewer for RA <code>{page.url.searchParams.get('ra')}</code>, Dec
		<code>{page.url.searchParams.get('dec')}</code> is not built yet.
	</p>
	<a href={resolve('/')}>Back to search</a>
</main>
