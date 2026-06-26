<script>
	import { theme, setTheme, THEMES } from '$lib/theme.js';

	// On mobile the picker collapses to a single glyph (the native <select> still
	// drives selection underneath). The glyph reflects the current theme.
	const GLYPHS = { system: '◐', light: '○', dark: '●', nord: '◆' };
	$: glyph = GLYPHS[$theme] || '◐';
</script>

<label class="theme-select">
	<span class="theme-select__label" aria-hidden="true">theme</span>
	<select
		aria-label="color theme"
		value={$theme}
		on:change={(e) => setTheme(e.currentTarget.value)}
	>
		{#each THEMES as t}
			<option value={t.value}>{t.label}</option>
		{/each}
	</select>
	<!-- Painted over the select on mobile (after it in the DOM so it stacks on
	     top); pointer-events: none keeps the native picker tappable. -->
	<span class="theme-select__glyph" aria-hidden="true">{glyph}</span>
</label>

<style>
	.theme-select {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
	}
	.theme-select__label {
		font-size: 0.7rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-faint);
	}
	/* The glyph only stands in for the picker on mobile (see the media query). */
	.theme-select__glyph {
		display: none;
	}
	select {
		appearance: none;
		font-family: var(--font-mono);
		font-size: 0.8rem;
		color: var(--text-muted);
		background: transparent;
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 0.25rem 1.4rem 0.25rem 0.6rem;
		cursor: pointer;
		background-image:
			linear-gradient(45deg, transparent 50%, currentColor 50%),
			linear-gradient(135deg, currentColor 50%, transparent 50%);
		background-position:
			right 0.7rem top 55%,
			right 0.45rem top 55%;
		background-size:
			0.25rem 0.25rem,
			0.25rem 0.25rem;
		background-repeat: no-repeat;
	}
	select:hover {
		color: var(--text);
		border-color: var(--border-strong);
	}
	select:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	@media (max-width: 640px) {
		.theme-select__label {
			display: none;
		}
		/* Collapse the picker to an icon: the <select> becomes a square tap
		   target with its label text and caret hidden, and the glyph is painted
		   on top (pointer-events: none so taps still reach the native picker). */
		.theme-select__glyph {
			display: grid;
			place-items: center;
			position: absolute;
			inset: 0;
			pointer-events: none;
			font-size: 0.95rem;
			color: var(--text-muted);
		}
		select {
			width: var(--touch-min);
			min-height: var(--touch-min);
			padding: 0;
			color: transparent;
			background-image: none;
			text-align: center;
		}
		.theme-select:hover .theme-select__glyph,
		select:focus-visible + .theme-select__glyph {
			color: var(--text);
		}
	}

	/* Coarse-pointer / mobile: the tappable control is the <select> itself, so
	   the touch minimum has to land on it (not just the wrapper). */
	@media (pointer: coarse) {
		.theme-select {
			min-height: var(--touch-min);
		}
		select {
			min-height: var(--touch-min);
		}
	}
</style>
