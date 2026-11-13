<script lang="ts">
	import { formatDec, formatFov, formatRA } from '#lib/format.js';

	let {
		name,
		type = null,
		ra,
		dec,
		fov,
		epochs = null,
		releases = null
	}: {
		name: string;
		type?: string | null;
		ra: number;
		dec: number;
		fov: number;
		/** Total epoch count, once the view is known. */
		epochs?: number | null;
		/** Release names the view was built from, once known. */
		releases?: string[] | null;
	} = $props();
</script>

<div class="heading">
	<h1>{name}</h1>
	{#if type}<p>{type}</p>{/if}
</div>

<dl>
	<dt>RA</dt>
	<dd class="mono">{formatRA(ra)}</dd>
	<dt>Dec</dt>
	<dd class="mono">{formatDec(dec)}</dd>
	<dt>Field</dt>
	<dd class="mono">{formatFov(fov)}</dd>
	{#if epochs !== null}
		<dt>Epochs</dt>
		<dd class="mono">{epochs}</dd>
	{/if}
	{#if releases?.length}
		<dt>Releases</dt>
		<dd class="mono">{releases.join(', ')}</dd>
	{/if}
</dl>

<style>
	.heading {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	h1 {
		margin: 0;
		font-size: 26px;
		font-weight: 700;
		line-height: 1.2;
		overflow-wrap: anywhere;
	}
	.heading p {
		margin: 0;
		color: var(--muted);
	}
	dl {
		margin: 0;
		display: grid;
		grid-template-columns: 88px 1fr;
		font-size: 14px;
		border-top: 1px solid var(--line);
	}
	dt,
	dd {
		margin: 0;
		padding: 9px 0;
		border-bottom: 1px solid var(--line);
	}
	dt {
		color: var(--muted);
	}
</style>
