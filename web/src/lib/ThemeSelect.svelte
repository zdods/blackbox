<script>
	import { theme, setTheme, THEMES } from '$lib/theme.js';
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
</label>

<style>
	.theme-select {
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
		/* Trim the picker so the brand wordmark keeps its room on small phones. */
		select {
			padding: 0.25rem 1.2rem 0.25rem 0.5rem;
		}
	}

	/* Coarse-pointer / mobile: give the picker a ≥44px hit target without
	   widening its visual footprint, so it meets the new touch minimum. */
	@media (pointer: coarse) {
		.theme-select {
			min-height: var(--touch-min);
		}
	}
</style>
