<script>
	// Bulk-action bar shown while a multi-selection is active. The parent owns the
	// real work: `on:download` runs the staggered anchor-reuse loop, `on:delete`
	// opens ConfirmDialog then runs the DELETE loop, and `on:clear` empties the
	// selection. This component only renders the sticky bar + dispatches intent —
	// it never fetches, never blobs, never touches the network. It mounts inside
	// the `{#if selected.size}` guard at the call site, so `count` is always >= 1
	// here, but we still gate sticky/visible affordances on count > 0 defensively.
	import { createEventDispatcher } from 'svelte';
	import Face from '$lib/Face.svelte';

	export let count;

	const dispatch = createEventDispatcher();
</script>

{#if count > 0}
	<div class="selbar" role="region" aria-label="Selection actions">
		<span class="selbar__count">
			<Face state="ok" />
			<span class="selbar__num num">{count}</span>
			<span>selected</span>
		</span>

		<span class="selbar__sep" aria-hidden="true">·</span>

		<button type="button" class="selbar__btn" on:click={() => dispatch('download')}>
			<svg
				class="selbar__icon"
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
				<path d="M8 2.5 V10" />
				<path d="M4.5 6.5 L8 10 L11.5 6.5" />
				<path d="M3 13 H13" />
			</svg>
			<span>download <span class="num">({count})</span></span>
		</button>

		<button
			type="button"
			class="selbar__btn selbar__btn--danger"
			on:click={() => dispatch('delete')}
		>
			<svg
				class="selbar__icon"
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
				<path d="M3 4.5 H13" />
				<path d="M6.5 4.5 V3 H9.5 V4.5" />
				<path d="M4.5 4.5 L5 13 H11 L11.5 4.5" />
				<path d="M6.5 7 V11" />
				<path d="M9.5 7 V11" />
			</svg>
			<span>delete <span class="num">({count})</span></span>
		</button>

		<button
			type="button"
			class="selbar__btn selbar__btn--quiet"
			on:click={() => dispatch('clear')}
			aria-label="Clear selection"
		>
			<svg
				class="selbar__icon"
				viewBox="0 0 16 16"
				width="16"
				height="16"
				fill="none"
				stroke="currentColor"
				stroke-width="1.5"
				stroke-linecap="round"
				aria-hidden="true"
			>
				<path d="M4 4 L12 12" />
				<path d="M12 4 L4 12" />
			</svg>
			<span>clear</span>
		</button>
	</div>
{/if}

<style>
	/* Sticks to the bottom of the file-browser pane on desktop; docks to the
	   bottom of the viewport (above the home-indicator safe area) on mobile. */
	.selbar {
		position: sticky;
		bottom: var(--space-md);
		z-index: var(--z-sidebar);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-xs) var(--space-sm);
		margin-top: var(--space-md);
		padding: var(--space-sm) var(--space-md);
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		box-shadow: var(--shadow-pop);
		font-size: var(--fs-sm);
	}

	.selbar__count {
		display: inline-flex;
		align-items: center;
		gap: var(--space-xs);
		font-weight: 500;
		color: var(--text);
	}

	.selbar__num {
		font-weight: 600;
		color: var(--accent);
	}

	.selbar__sep {
		color: var(--text-faint);
	}

	.selbar__btn {
		display: inline-flex;
		align-items: center;
		gap: var(--space-xs);
		padding: var(--space-xs) var(--space-sm);
		border: 1px solid transparent;
		border-radius: var(--radius-sm);
		background: transparent;
		color: var(--text-muted);
		font-size: var(--fs-sm);
		cursor: pointer;
	}

	.selbar__btn:hover {
		color: var(--text);
		background: var(--inset);
	}

	.selbar__btn--danger:hover {
		color: var(--err);
		background: var(--inset);
	}

	.selbar__btn--quiet {
		margin-left: auto;
		color: var(--text-faint);
	}

	.selbar__btn--quiet:hover {
		color: var(--text);
	}

	.selbar__btn:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}

	.selbar__icon {
		display: block;
		flex-shrink: 0;
	}

	@media (prefers-reduced-motion: no-preference) {
		.selbar {
			animation: selbar-in var(--dur-base) var(--ease-out);
		}
		.selbar__btn {
			transition:
				color var(--dur-instant) var(--ease-std),
				background var(--dur-instant) var(--ease-std);
		}
	}

	@keyframes selbar-in {
		from {
			opacity: 0;
			transform: translateY(var(--slide-y));
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	/* Mobile: dock to the very bottom of the viewport, full-bleed, above the
	   safe-area inset. Larger tap targets for coarse pointers. */
	@media (max-width: 640px) {
		.selbar {
			position: fixed;
			left: var(--safe-left);
			right: var(--safe-right);
			bottom: 0;
			margin-top: 0;
			padding-bottom: calc(var(--space-sm) + var(--safe-bottom));
			border-radius: var(--radius-lg) var(--radius-lg) 0 0;
			box-shadow: var(--shadow-overlay);
		}
	}

	@media (pointer: coarse) {
		.selbar__btn {
			min-height: var(--touch-min);
		}
	}
</style>
