<script>
	// Accessible custom checkbox: a REAL <input type=checkbox> visually replaced
	// by a styled box + inline-SVG check / dash. `indeterminate` is a DOM-only
	// property, so it is set imperatively via JS (it cannot be expressed in
	// markup). The on:change event forwards the native change so callers can read
	// modifier keys (shift/ctrl/meta) off it for range/toggle selection.
	export let checked = false;
	export let indeterminate = false;
	export let ariaLabel = undefined;

	let input;

	// indeterminate is not a reflected HTML attribute — keep the DOM node in sync
	// whenever the prop changes (and once the node is bound).
	$: if (input) input.indeterminate = indeterminate;
</script>

<!--
	The native input sits invisibly on top of the styled box, filling the whole
	44px hit area, so the real control handles focus, keyboard (space), pointer
	and assistive tech. We forward change/click rather than re-emitting so the
	original event (with shiftKey/ctrlKey/metaKey) reaches the caller.
-->
<label class="cb">
	<input
		bind:this={input}
		type="checkbox"
		class="cb__input"
		bind:checked
		aria-label={ariaLabel}
		aria-checked={indeterminate ? 'mixed' : undefined}
		on:change
		on:click
	/>
	<span class="cb__box" class:cb__box--checked={checked} class:cb__box--mixed={indeterminate} aria-hidden="true">
		{#if indeterminate}
			<svg class="cb__glyph" viewBox="0 0 16 16" width="16" height="16" fill="none">
				<path d="M3.5 8 H12.5" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
			</svg>
		{:else if checked}
			<svg class="cb__glyph" viewBox="0 0 16 16" width="16" height="16" fill="none">
				<path
					d="M3.5 8.5 L6.5 11.5 L12.5 4.5"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
				/>
			</svg>
		{/if}
	</span>
</label>

<style>
	.cb {
		position: relative;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		/* 44px mobile hit area; the visual box is centered inside. */
		min-width: var(--touch-min);
		min-height: var(--touch-min);
		cursor: pointer;
		flex-shrink: 0;
	}

	/* Real input covers the whole hit area but is visually invisible. It still
	   receives focus/keyboard/pointer so a11y + space-to-toggle work natively. */
	.cb__input {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		margin: 0;
		opacity: 0;
		cursor: pointer;
		z-index: 1;
	}

	.cb__box {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 18px;
		height: 18px;
		box-sizing: border-box;
		border: 1.5px solid var(--border-strong);
		border-radius: var(--radius-xs);
		background: var(--inset);
		color: var(--accent-contrast);
		pointer-events: none;
	}

	.cb__box--checked,
	.cb__box--mixed {
		border-color: var(--accent);
		background: var(--accent);
	}

	.cb__glyph {
		display: block;
	}

	/* Focus ring: mirror the canonical 2px accent / offset-2px ring on the box
	   when the real input is focused via keyboard. Instant, never animated. */
	.cb__input:focus-visible + .cb__box {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}

	@media (prefers-reduced-motion: no-preference) {
		.cb__box {
			transition:
				background var(--dur-instant) var(--ease-std),
				border-color var(--dur-instant) var(--ease-std);
		}
	}
</style>
