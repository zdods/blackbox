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
	<!-- Shown only on mobile, where the select goes fully transparent and this
	     glyph is the visible control (the select stays on top as the tap/keyboard
	     target). -->
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
		/* Collapse to an icon. The wrapper becomes the visible, bordered control
		   and shows the glyph; the native <select> is laid over it fully
		   transparent. We use opacity:0 (not color:transparent) because mobile
		   browsers — iOS Safari especially — still paint a select's value text
		   when only the color is transparent, which would clash with the glyph.
		   opacity:0 hides the value text and arrow on every engine while keeping
		   the element tappable and focusable. */
		.theme-select {
			width: var(--touch-min);
			height: var(--touch-min);
			justify-content: center;
			border: 1px solid var(--border);
			border-radius: 6px;
		}
		.theme-select:hover {
			border-color: var(--border-strong);
		}
		.theme-select:focus-within {
			outline: 2px solid var(--accent);
			outline-offset: 2px;
		}
		select {
			position: absolute;
			inset: 0;
			width: 100%;
			height: 100%;
			min-height: 0;
			padding: 0;
			border: none;
			opacity: 0;
		}
		.theme-select__glyph {
			display: block;
			font-size: 0.95rem;
			line-height: 1;
			color: var(--text-muted);
			pointer-events: none;
		}
		.theme-select:hover .theme-select__glyph {
			color: var(--text);
		}
	}

	/* Coarse-pointer (touch) at any width: keep the control at the touch minimum
	   so the tap target is big enough even on tablets that miss the width query. */
	@media (pointer: coarse) {
		.theme-select {
			min-height: var(--touch-min);
		}
		select {
			min-height: var(--touch-min);
		}
	}
</style>
