<!--
	Blink player (F4): the frames of one detector in date order, with play, pause and a
	slider. Frames are aligned server-side and drawn in one box, so nothing moves between
	frames unless the sky did. Every frame stays loaded; only the current one shows.
	Playback state lives in the Blink passed in.
-->
<script lang="ts">
	import type { View } from '#lib/api.js';
	import { formatAngle, formatBand, scaleBar } from '#lib/format.js';
	import { warningText } from '#lib/warnings.js';
	import type { Blink } from './blink.svelte.js';

	let { blink, epochs }: { blink: Blink; epochs: View['epochs'] } = $props();

	const id = $props.id();
	// The parent shows the player only when the detector has frames.
	const frames = $derived(blink.frames);
	const frame = $derived(blink.frame!);
	const band = $derived(formatBand(frame.wavelength_min_um, frame.wavelength_max_um));
	const epochNo = $derived(
		epochs.findIndex((e) => e.index === frame.epoch_index) + 1 || blink.index + 1
	);
	const epochText = $derived(`Epoch ${epochNo} of ${epochs.length}`);
	const residual = $derived(
		frame.alignment_residual_arcsec === null
			? 'not measured'
			: formatAngle(frame.alignment_residual_arcsec)
	);
	// All frames share one grid (aligned server-side), so the first frame sets the geometry.
	const geometry = $derived(frames[0]);
	const scale = $derived(scaleBar(geometry.width * geometry.pixel_scale_arcsec));
	const northUp = $derived(Math.abs(geometry.orientation_deg) < 0.5);
	const settledCount = $derived(frames.filter((f) => blink.settled.has(f.id)).length);
</script>

<figure class="stage" style:aspect-ratio="{geometry.width} / {geometry.height}">
	{#each frames as f, i (f.id)}
		<img
			src={f.url}
			alt={i === blink.index ? `Sky frame, ${epochText}, ${f.date}, ${band} micrometres` : ''}
			width={f.width}
			height={f.height}
			class:current={i === blink.index}
			onload={() => blink.settle(f.id, true)}
			onerror={() => blink.settle(f.id, false)}
		/>
	{/each}

	{#if blink.failed.has(frame.id)}
		<p class="missing">This frame could not be loaded.</p>
	{:else if !blink.ready}
		<p class="missing" role="status">Loading frames… {settledCount} of {frames.length}</p>
	{/if}

	<div class="scale mono" aria-label="Scale bar, {formatAngle(scale.arcsec)}">
		<!-- cqw: a share of the frame's width, wherever the bar sits in it. -->
		<div class="bar" style:width="{scale.fraction * 100}cqw"></div>
		<span class="chip">{formatAngle(scale.arcsec)}</span>
	</div>
	{#if northUp}
		<span class="chip compass mono">N up · E left</span>
	{/if}
	<!-- What the frame shows and its notices (F9); wraps instead of overlapping when narrow. -->
	<figcaption class="top">
		<span class="chip label mono">{frame.date} · {band} µm</span>
		{#if frame.warnings.length}
			<ul class="warnings" aria-label="Notices for this frame">
				{#each frame.warnings as code (code)}
					{@const text = warningText(code)}
					<li title={text.help}>{text.label}</li>
				{/each}
			</ul>
		{/if}
	</figcaption>
</figure>

<div class="controls">
	<button class="btn" type="button" disabled={frames.length < 2} onclick={() => blink.toggle()}>
		{blink.playing && frames.length > 1 ? 'Pause' : 'Play'}
	</button>
	<div class="scrub">
		<label class="caption" for="{id}-scrub">
			{epochText} · alignment residual {residual}
		</label>
		<input
			id="{id}-scrub"
			type="range"
			min="0"
			max={frames.length - 1}
			step="1"
			value={blink.index}
			aria-valuetext="{epochText}, {frame.date}"
			disabled={frames.length < 2}
			oninput={(e) => blink.seek(Number(e.currentTarget.value))}
		/>
	</div>
	{#if frames.length < epochs.length}
		<p class="caption">
			This detector has frames from {frames.length} of {epochs.length} epochs.
		</p>
	{/if}
</div>

<style>
	.stage {
		position: relative;
		width: 100%;
		max-width: 600px;
		margin: 0;
		border-radius: 12px;
		overflow: hidden;
		background: var(--stage);
		color: var(--onStage);
		container-type: inline-size;
	}
	img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		/* SPHEREx pixels are 6″: show them as square pixels, not a blur. */
		image-rendering: pixelated;
		visibility: hidden;
	}
	img.current {
		visibility: visible;
	}
	.missing {
		position: absolute;
		inset: 0;
		margin: 0;
		display: grid;
		place-items: center;
		font-size: 14px;
	}

	.chip {
		padding: 4px 8px;
		border-radius: 6px;
		background: var(--stageChip);
		font-size: 12px;
	}
	.top {
		position: absolute;
		left: 12px;
		right: 12px;
		top: 12px;
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		gap: 6px;
	}
	.label {
		padding: 6px 10px;
		border-radius: 8px;
		font-size: 13px;
	}
	.warnings {
		margin: 0 0 0 auto;
		padding: 0;
		list-style: none;
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		gap: 6px;
	}
	.warnings li {
		padding: 6px 10px;
		border-radius: 8px;
		background: var(--warnBg);
		color: var(--warnFg);
		font-size: 13px;
		font-weight: 600;
	}
	.scale {
		position: absolute;
		left: 12px;
		bottom: 12px;
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 4px;
	}
	.bar {
		height: 2px;
		background: var(--onStage);
	}
	.compass {
		position: absolute;
		right: 12px;
		bottom: 12px;
	}

	.controls {
		width: 100%;
		max-width: 640px;
		box-sizing: border-box;
		padding: 10px 16px;
		border-radius: 12px;
		background: var(--surface);
		border: 1px solid var(--line);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 16px;
	}
	.controls .btn {
		min-width: 88px;
	}
	.scrub {
		flex: 1 1 240px;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	input[type='range'] {
		width: 100%;
		height: 28px;
		margin: 0;
		accent-color: var(--accent);
	}
</style>
