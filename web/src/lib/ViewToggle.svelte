<script>
	// Segmented list/grid control. Persists the choice to localStorage so the
	// file browser remembers the preferred view across reloads. Default is
	// 'list' (e2e contract). Inline SVG icons keep it CSP-safe and theme-reactive
	// via currentColor.
	import { onMount, createEventDispatcher } from 'svelte';

	export let value = 'list'; // 'list' | 'grid'

	const STORAGE_KEY = 'blackhaul_fileview';
	const dispatch = createEventDispatcher();

	// On mount, hydrate from localStorage if a valid value was saved. We do this
	// in onMount (not at module init) so SSG/no-window builds don't touch storage.
	onMount(() => {
		try {
			const saved = localStorage.getItem(STORAGE_KEY);
			if ((saved === 'list' || saved === 'grid') && saved !== value) {
				value = saved;
				dispatch('change', value);
			}
		} catch {
			// storage unavailable (private mode etc.) — ignore, keep default.
		}
	});

	function select(next) {
		if (next === value) return;
		value = next;
		try {
			localStorage.setItem(STORAGE_KEY, value);
		} catch {
			// ignore persistence failures
		}
		dispatch('change', value);
	}
</script>

<div class="view-toggle" role="group" aria-label="File view">
	<button
		type="button"
		class="view-toggle__btn"
		class:view-toggle__btn--active={value === 'list'}
		aria-pressed={value === 'list'}
		aria-label="List view"
		title="List view"
		on:click={() => select('list')}
	>
		<svg
			class="view-toggle__icon"
			viewBox="0 0 16 16"
			width="16"
			height="16"
			fill="none"
			stroke="currentColor"
			stroke-width="1.5"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			<line x1="5.5" y1="3.5" x2="14" y2="3.5" />
			<line x1="5.5" y1="8" x2="14" y2="8" />
			<line x1="5.5" y1="12.5" x2="14" y2="12.5" />
			<line x1="2" y1="3.5" x2="2.5" y2="3.5" />
			<line x1="2" y1="8" x2="2.5" y2="8" />
			<line x1="2" y1="12.5" x2="2.5" y2="12.5" />
		</svg>
	</button>
	<button
		type="button"
		class="view-toggle__btn"
		class:view-toggle__btn--active={value === 'grid'}
		aria-pressed={value === 'grid'}
		aria-label="Grid view"
		title="Grid view"
		on:click={() => select('grid')}
	>
		<svg
			class="view-toggle__icon"
			viewBox="0 0 16 16"
			width="16"
			height="16"
			fill="none"
			stroke="currentColor"
			stroke-width="1.5"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			<rect x="2" y="2" width="5" height="5" rx="1" />
			<rect x="9" y="2" width="5" height="5" rx="1" />
			<rect x="2" y="9" width="5" height="5" rx="1" />
			<rect x="9" y="9" width="5" height="5" rx="1" />
		</svg>
	</button>
</div>

<style>
	.view-toggle {
		display: inline-flex;
		align-items: center;
		gap: 2px;
		padding: 2px;
		background: var(--inset);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
	}

	.view-toggle__btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 1.75rem;
		height: 1.75rem;
		padding: 0;
		border: none;
		border-radius: var(--radius-xs);
		background: transparent;
		color: var(--text-muted);
		cursor: pointer;
	}

	.view-toggle__icon {
		display: block;
	}

	.view-toggle__btn:hover {
		color: var(--text);
	}

	.view-toggle__btn--active {
		background: var(--surface);
		color: var(--accent);
		box-shadow: var(--shadow-1);
	}

	.view-toggle__btn:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}

	@media (prefers-reduced-motion: no-preference) {
		.view-toggle__btn {
			transition:
				color var(--dur-instant) var(--ease-std),
				background var(--dur-instant) var(--ease-std);
		}
	}

	@media (pointer: coarse) {
		.view-toggle__btn {
			width: var(--touch-min);
			height: var(--touch-min);
		}
	}
</style>
