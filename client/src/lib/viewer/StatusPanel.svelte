<!-- What the viewer shows in place of the blink: loading, no coverage (F2), errors (F3). -->
<script lang="ts">
	import { resolve } from '$app/paths';
	import type { Phase } from './view-loader.svelte.js';

	let {
		phase,
		onretry
	}: {
		/** Any phase but a ready view with frames; a ready view here has none. */
		phase: Phase;
		onretry: () => void;
	} = $props();
</script>

<section class="panel">
	{#if phase.kind === 'loading'}
		<!-- Only the stage is announced, not every progress step. -->
		<p role="status">{phase.progress === null ? 'Opening the view…' : 'Building the blink…'}</p>
		{#if phase.progress !== null}
			<progress max="100" value={phase.progress} aria-label="Building the blink"></progress>
			<p class="mono">{phase.progress}%</p>
			<p class="caption">New views can take a few minutes. Featured targets open at once.</p>
		{/if}
	{:else if phase.kind === 'no-coverage'}
		<h2 role="status">No SPHEREx data here yet</h2>
		<p class="caption">
			SPHEREx has not observed this position in a public release. Try another target.
		</p>
		<a href={resolve('/')}>Back to search</a>
	{:else if phase.kind === 'error'}
		<h2>This view could not be opened</h2>
		<p role="alert">{phase.message}</p>
		<button class="btn" type="button" onclick={onretry}>Try again</button>
	{:else}
		<h2>This view has no frames</h2>
		<a href={resolve('/')}>Back to search</a>
	{/if}
</section>

<style>
	.panel {
		width: 100%;
		max-width: 600px;
		aspect-ratio: 1;
		box-sizing: border-box;
		padding: 24px;
		border-radius: 12px;
		background: var(--surface);
		border: 1px solid var(--line);
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 12px;
		text-align: center;
	}
	h2 {
		margin: 0;
		font-size: 20px;
	}
	p {
		margin: 0;
	}
	progress {
		width: min(320px, 100%);
		accent-color: var(--accent);
	}
</style>
