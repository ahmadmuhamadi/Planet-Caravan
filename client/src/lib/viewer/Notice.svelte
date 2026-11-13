<!-- A short notice next to the thing it applies to (F8, F9). -->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		title,
		tone = 'warn',
		children
	}: { title: string; tone?: 'warn' | 'info'; children?: Snippet } = $props();
</script>

<div class="notice {tone}">
	<strong>{title}</strong>
	{@render children?.()}
</div>

<style>
	.notice {
		padding: 10px 14px;
		border-radius: var(--radius);
		font-size: 14px;
		display: flex;
		flex-wrap: wrap;
		column-gap: 8px;
	}
	.warn {
		background: var(--warnBg);
		color: var(--warnFg);
	}
	.info {
		background: var(--infoBg);
		color: var(--infoFg);
	}
	.notice :global(a) {
		color: inherit;
		font-weight: 600;
	}
</style>
