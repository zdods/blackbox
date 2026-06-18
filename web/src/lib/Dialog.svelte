<script>
	// Themed modal primitive replacing native confirm(). role=dialog + aria-modal,
	// Esc/backdrop close, focus trap, focus restore to opener, --ease-soft enter via
	// the shared .overlay-surface animation. Slots: title, default (body), actions.
	// Mirrors CommandPalette's overlay-scrim/overlay-surface scaffolding.
	import { createEventDispatcher, tick } from 'svelte';

	export let open = false;
	export let title = '';
	export let danger = false;

	const dispatch = createEventDispatcher();

	let surfaceEl;
	let prevFocus = null;
	const titleId = `dialog-title-${Math.random().toString(36).slice(2, 9)}`;

	$: if (open) onOpen();

	async function onOpen() {
		prevFocus = typeof document !== 'undefined' ? document.activeElement : null;
		await tick();
		focusFirst();
	}

	// Move focus into the dialog when it opens — first focusable, else the surface.
	function focusFirst() {
		if (!surfaceEl) return;
		const target = surfaceEl.querySelector(FOCUSABLE);
		if (target && typeof target.focus === 'function') target.focus();
		else surfaceEl.focus();
	}

	const FOCUSABLE =
		'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

	function focusables() {
		if (!surfaceEl) return [];
		return Array.from(surfaceEl.querySelectorAll(FOCUSABLE)).filter(
			(el) => el.offsetParent !== null || el === document.activeElement
		);
	}

	// Cancel = dismissal (Esc, backdrop, close button); also emits close.
	function cancel() {
		dispatch('cancel');
		closeAndRestore();
	}

	function confirm() {
		dispatch('confirm');
		closeAndRestore();
	}

	function closeAndRestore() {
		dispatch('close');
		if (prevFocus && typeof prevFocus.focus === 'function') prevFocus.focus();
	}

	function onKeydown(e) {
		if (!open) return;
		if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			cancel();
			return;
		}
		if (e.key === 'Tab') {
			const items = focusables();
			if (items.length === 0) {
				e.preventDefault();
				return;
			}
			const first = items[0];
			const last = items[items.length - 1];
			const active = document.activeElement;
			if (e.shiftKey && (active === first || !surfaceEl.contains(active))) {
				e.preventDefault();
				last.focus();
			} else if (!e.shiftKey && active === last) {
				e.preventDefault();
				first.focus();
			}
		}
	}
</script>

<svelte:window on:keydown|capture={onKeydown} />

{#if open}
	<!-- svelte-ignore a11y-click-events-have-key-events -->
	<!-- svelte-ignore a11y-no-static-element-interactions -->
	<div class="dialog-scrim overlay-scrim" on:click={cancel}></div>
	<div class="dialog">
		<!-- svelte-ignore a11y-click-events-have-key-events -->
		<!-- svelte-ignore a11y-no-static-element-interactions -->
		<div
			class="dialog__surface overlay-surface"
			class:dialog__surface--danger={danger}
			role="dialog"
			aria-modal="true"
			aria-labelledby={titleId}
			tabindex="-1"
			bind:this={surfaceEl}
			on:click|stopPropagation
		>
			<div class="dialog__head">
				<h2 class="dialog__title" id={titleId}>
					<slot name="title">{title}</slot>
				</h2>
				<button
					type="button"
					class="dialog__close"
					aria-label="close dialog"
					on:click={cancel}>✕</button
				>
			</div>

			<div class="dialog__body">
				<slot />
			</div>

			<div class="dialog__actions">
				<slot name="actions">
					<button type="button" on:click={cancel}>cancel</button>
					<button
						type="button"
						class="dialog__confirm"
						class:primary={!danger}
						class:danger={danger}
						on:click={confirm}>{danger ? 'delete' : 'confirm'}</button
					>
				</slot>
			</div>
		</div>
	</div>
{/if}

<style>
	.dialog-scrim {
		z-index: var(--z-modal-scrim);
	}

	.dialog {
		position: fixed;
		inset: 0;
		z-index: var(--z-modal);
		display: flex;
		justify-content: center;
		align-items: center;
		padding: var(--space-md);
		padding-bottom: max(var(--space-md), var(--safe-bottom));
		pointer-events: none;
	}

	.dialog__surface {
		pointer-events: auto;
		width: 100%;
		max-width: 28rem;
		display: flex;
		flex-direction: column;
		max-height: calc(100vh - 2 * var(--space-md));
		overflow: hidden;
	}

	.dialog__surface--danger {
		border-color: var(--err);
	}

	.dialog__head {
		display: flex;
		align-items: flex-start;
		gap: var(--space-sm);
		padding: var(--space-md) var(--space-md) 0;
	}

	.dialog__title {
		flex: 1;
		margin: 0;
		font-size: var(--fs-md);
		font-weight: 600;
		line-height: var(--lh-tight);
		letter-spacing: var(--tracking-tight);
		color: var(--text);
		overflow-wrap: anywhere;
	}

	.dialog__surface--danger .dialog__title {
		color: var(--err);
	}

	.dialog__close {
		flex-shrink: 0;
		height: auto;
		width: auto;
		padding: var(--space-xs) var(--space-sm);
		border: none;
		background: transparent;
		color: var(--text-faint);
		font-size: var(--fs-sm);
		line-height: 1;
	}

	.dialog__close:hover {
		color: var(--text);
		border-color: transparent;
	}

	.dialog__body {
		padding: var(--space-md);
		overflow-y: auto;
		color: var(--text-muted);
		font-size: var(--fs-sm);
		line-height: var(--lh-body);
	}

	.dialog__body :global(p) {
		margin: 0 0 var(--space-sm);
	}

	.dialog__body :global(p:last-child) {
		margin-bottom: 0;
	}

	.dialog__actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-sm);
		padding: 0 var(--space-md) var(--space-md);
	}

	.dialog__confirm.danger {
		background: var(--err);
		border-color: var(--err);
		color: var(--accent-contrast);
		font-weight: 600;
	}

	.dialog__confirm.danger:hover:not(:disabled) {
		filter: brightness(1.08);
		border-color: var(--err);
	}

	@media (max-width: 640px) {
		.dialog {
			align-items: flex-end;
			padding: var(--space-sm);
			padding-bottom: max(var(--space-sm), var(--safe-bottom));
		}
		.dialog__surface {
			max-width: 100%;
		}
		.dialog__close {
			min-width: var(--touch-min);
			min-height: var(--touch-min);
		}
	}
</style>
