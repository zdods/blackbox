<script>
	// Cursor-anchored popover menu (desktop) / bottom-sheet (mobile). Driven by
	// the file browser for right-click and the ⋯ kebab. The caller owns the
	// item list (labels already reflect "(N)" when a selection is active) — this
	// component only positions, renders, keyboard-drives, and dispatches `select`
	// with the chosen item's id. No deps; role=menu/menuitem; viewport-clamped;
	// closes on Esc / outside-click / scroll; restores focus to the trigger.
	import { createEventDispatcher, tick, onDestroy } from 'svelte';

	export let open = false;
	export let x = 0;
	export let y = 0;
	export let items = []; // [{ id, label, icon?, danger?, disabled?, divider? }]
	export let mobileSheet = false;

	const dispatch = createEventDispatcher();

	let menuEl;
	let prevFocus = null;
	let posX = 0;
	let posY = 0;
	let highlight = -1;

	// Indices of the real (non-divider, non-disabled) items, for arrow nav.
	$: actionable = items
		.map((it, i) => ({ it, i }))
		.filter(({ it }) => !it.divider && !it.disabled)
		.map(({ i }) => i);

	$: if (open) onOpen();
	$: if (!open) teardown();

	async function onOpen() {
		prevFocus = typeof document !== 'undefined' ? document.activeElement : null;
		highlight = actionable.length ? actionable[0] : -1;
		await tick();
		clampToViewport();
		if (menuEl) menuEl.focus();
		// Defer so the opening click doesn't immediately count as "outside".
		if (typeof window !== 'undefined') {
			window.addEventListener('scroll', onScroll, true);
			window.addEventListener('resize', onScroll, true);
		}
	}

	function teardown() {
		if (typeof window !== 'undefined') {
			window.removeEventListener('scroll', onScroll, true);
			window.removeEventListener('resize', onScroll, true);
		}
	}

	onDestroy(teardown);

	function clampToViewport() {
		// Mobile bottom-sheet ignores cursor coords (full-width, docked bottom).
		if (mobileSheet) return;
		if (typeof window === 'undefined') return;
		const pad = 8;
		const w = menuEl ? menuEl.offsetWidth : 0;
		const h = menuEl ? menuEl.offsetHeight : 0;
		const vw = window.innerWidth;
		const vh = window.innerHeight;
		posX = Math.max(pad, Math.min(x, vw - w - pad));
		posY = Math.max(pad, Math.min(y, vh - h - pad));
	}

	function close() {
		dispatch('close');
		if (prevFocus && typeof prevFocus.focus === 'function') prevFocus.focus();
	}

	function onScroll() {
		// A scroll/resize dismisses the cursor-anchored popover (the anchor would
		// otherwise drift). The mobile bottom-sheet is viewport-fixed, so scrolling
		// inside it must not close it.
		if (mobileSheet) return;
		close();
	}

	function selectItem(item) {
		if (!item || item.disabled || item.divider) return;
		dispatch('select', { id: item.id });
		close();
	}

	function moveHighlight(delta) {
		if (!actionable.length) return;
		const pos = actionable.indexOf(highlight);
		const next = (pos + delta + actionable.length) % actionable.length;
		highlight = actionable[next];
		focusHighlight();
	}

	async function focusHighlight() {
		await tick();
		if (!menuEl) return;
		const el = menuEl.querySelector(`[data-idx="${highlight}"]`);
		if (el) el.focus();
	}

	function onKeydown(e) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			moveHighlight(1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			moveHighlight(-1);
		} else if (e.key === 'Home') {
			e.preventDefault();
			if (actionable.length) {
				highlight = actionable[0];
				focusHighlight();
			}
		} else if (e.key === 'End') {
			e.preventDefault();
			if (actionable.length) {
				highlight = actionable[actionable.length - 1];
				focusHighlight();
			}
		} else if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			selectItem(items[highlight]);
		} else if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			close();
		} else if (e.key === 'Tab') {
			// Trap: a menu has no other tab stops — keep focus inside, Tab navigates.
			e.preventDefault();
			moveHighlight(e.shiftKey ? -1 : 1);
		}
	}
</script>

{#if open}
	<!-- svelte-ignore a11y-click-events-have-key-events -->
	<!-- svelte-ignore a11y-no-static-element-interactions -->
	<div
		class="ctx-scrim"
		class:ctx-scrim--sheet={mobileSheet}
		on:click={close}
		on:contextmenu|preventDefault={close}
	></div>

	<!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
	<div
		bind:this={menuEl}
		class="ctx"
		class:ctx--sheet={mobileSheet}
		role="menu"
		tabindex="-1"
		aria-orientation="vertical"
		style={mobileSheet ? '' : `left: ${posX}px; top: ${posY}px;`}
		on:keydown={onKeydown}
		on:click|stopPropagation
		on:contextmenu|stopPropagation|preventDefault
	>
		{#each items as item, i (item.id ?? `divider-${i}`)}
			{#if item.divider}
				<div class="ctx__divider" role="separator"></div>
			{:else}
				<button
					type="button"
					class="ctx__item"
					class:ctx__item--danger={item.danger}
					class:ctx__item--active={i === highlight}
					data-idx={i}
					role="menuitem"
					tabindex={i === highlight ? 0 : -1}
					disabled={item.disabled}
					on:click={() => selectItem(item)}
					on:mouseenter={() => (highlight = i)}
				>
					{#if item.icon}
						<span class="ctx__icon" aria-hidden="true">{item.icon}</span>
					{/if}
					<span class="ctx__label">{item.label}</span>
				</button>
			{/if}
		{/each}
	</div>
{/if}

<style>
	.ctx-scrim {
		position: fixed;
		inset: 0;
		z-index: var(--z-context-menu);
		background: transparent;
	}

	/* On mobile the sheet sits over a dimming backdrop. */
	.ctx-scrim--sheet {
		background: var(--backdrop);
	}

	.ctx {
		position: fixed;
		z-index: var(--z-context-menu);
		min-width: 12rem;
		max-width: min(18rem, calc(100vw - 16px));
		padding: var(--space-xs);
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		box-shadow: var(--shadow-pop);
		outline: none;
	}

	.ctx__item {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		width: 100%;
		height: auto;
		min-height: var(--row-height);
		padding: var(--space-xs) var(--space-sm);
		border: none;
		border-radius: var(--radius-sm);
		background: transparent;
		color: var(--text);
		font-size: var(--fs-sm);
		text-align: left;
		cursor: pointer;
	}

	.ctx__item:disabled {
		color: var(--text-faint);
		cursor: default;
	}

	.ctx__item--active:not(:disabled),
	.ctx__item:hover:not(:disabled) {
		background: var(--row-hover);
	}

	.ctx__item--danger:not(:disabled) {
		color: var(--err);
	}

	.ctx__item--danger.ctx__item--active:not(:disabled),
	.ctx__item--danger:hover:not(:disabled) {
		background: var(--err-soft);
	}

	.ctx__icon {
		flex-shrink: 0;
		width: var(--icon-col);
		text-align: center;
		color: var(--text-muted);
	}

	.ctx__item--danger .ctx__icon {
		color: inherit;
	}

	.ctx__label {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.ctx__divider {
		height: 1px;
		margin: var(--space-xs) var(--space-sm);
		background: var(--border);
	}

	/* ---- Mobile bottom-sheet ------------------------------------------ */
	.ctx--sheet {
		left: 0;
		right: 0;
		bottom: 0;
		top: auto;
		min-width: 0;
		max-width: none;
		padding: var(--space-sm) var(--space-sm) calc(var(--space-sm) + var(--safe-bottom));
		padding-left: max(var(--space-sm), var(--safe-left));
		padding-right: max(var(--space-sm), var(--safe-right));
		border-left: none;
		border-right: none;
		border-bottom: none;
		border-radius: var(--radius-lg) var(--radius-lg) 0 0;
		box-shadow: var(--shadow-overlay);
	}

	.ctx--sheet .ctx__item {
		min-height: var(--touch-min);
		font-size: var(--fs-md);
		padding: var(--space-sm) var(--space-md);
	}

	.ctx--sheet .ctx__divider {
		margin: var(--space-xs) var(--space-md);
	}

	@media (prefers-reduced-motion: no-preference) {
		.ctx {
			animation: ctx-pop var(--dur-fast) var(--ease-out);
		}
		.ctx--sheet {
			animation: ctx-sheet var(--dur-base) var(--ease-soft);
		}
		.ctx-scrim--sheet {
			animation: ctx-scrim-in var(--dur-base) var(--ease-std);
		}
	}

	@keyframes ctx-pop {
		from {
			opacity: 0;
			transform: translateY(calc(var(--slide-y) * -0.5)) scale(0.98);
		}
		to {
			opacity: 1;
			transform: translateY(0) scale(1);
		}
	}

	@keyframes ctx-sheet {
		from {
			transform: translateY(100%);
		}
		to {
			transform: translateY(0);
		}
	}

	@keyframes ctx-scrim-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}
</style>
