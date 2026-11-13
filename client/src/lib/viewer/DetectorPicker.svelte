<!-- Picks which SPHEREx detector (wavelength band) the blink shows. -->
<script lang="ts">
	import { DETECTORS } from '#lib/view-url.js';

	let {
		bands,
		selected,
		onselect
	}: {
		/** Band text ("1.10–1.65") per detector that has frames. */
		bands: ReadonlyMap<number, string>;
		selected: number;
		onselect: (detector: number) => void;
	} = $props();

	const id = $props.id();
</script>

<div class="picker" role="group" aria-labelledby="{id}-label">
	<div id="{id}-label" class="caption">
		Detector · <span class="mono">{bands.get(selected)} µm</span>
	</div>
	<div class="grid">
		{#each DETECTORS as d (d)}
			{@const band = bands.get(d)}
			<button
				class="toggle"
				type="button"
				aria-pressed={d === selected}
				aria-label={band
					? `Detector ${d}, ${band} micrometres`
					: `Detector ${d}, no frames in this view`}
				disabled={!band}
				onclick={() => onselect(d)}
			>
				{d}
			</button>
		{/each}
	</div>
</div>

<style>
	.picker {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(6, minmax(0, 1fr));
		gap: 6px;
	}
</style>
