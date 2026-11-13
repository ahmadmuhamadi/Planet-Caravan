<!-- The link to the current view, with a copy button (F7). -->
<script lang="ts">
	let {
		url,
		onbeforecopy
	}: {
		url: string;
		/** Runs before copying, e.g. to pause so the link holds the frame on screen. */
		onbeforecopy?: () => void;
	} = $props();

	const id = $props.id();
	let input: HTMLInputElement;
	let copied = $state<'ok' | 'manual' | null>(null);
	let timer: ReturnType<typeof setTimeout> | undefined;

	$effect(() => () => clearTimeout(timer));

	async function copy() {
		onbeforecopy?.();
		clearTimeout(timer);
		try {
			await navigator.clipboard.writeText(url);
			copied = 'ok';
			timer = setTimeout(() => (copied = null), 2500);
		} catch {
			// No clipboard access (permissions, insecure origin): let the user copy it.
			input.focus();
			input.select();
			copied = 'manual';
		}
	}
</script>

<div class="share">
	<label class="caption" for="{id}-link">Link to this view</label>
	<div class="row">
		<input
			bind:this={input}
			id="{id}-link"
			class="input mono"
			type="text"
			readonly
			value={url}
			onfocus={(e) => e.currentTarget.select()}
		/>
		<button class="btn" type="button" onclick={copy}>
			{copied === 'ok' ? 'Copied' : 'Copy link'}
		</button>
	</div>
	<p class="caption" role="status">
		{#if copied === 'ok'}
			Link copied to the clipboard.
		{:else if copied === 'manual'}
			The link is selected. Press Ctrl+C or Cmd+C to copy it.
		{:else}
			The same link is in the browser address bar.
		{/if}
	</p>
</div>

<style>
	.share {
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
		flex: 1 1 260px;
		min-width: 0;
		padding: 0 12px;
		font-size: 13px;
	}
	.btn {
		min-width: 112px;
	}
</style>
