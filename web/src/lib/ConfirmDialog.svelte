<script context="module">
	// Promise-based replacement for native confirm() at every delete call site
	// (dashboard delete-host, file delete, bulk delete). The canonical call
	// pattern is:
	//
	//   import { confirmDelete } from '$lib/ConfirmDialog.svelte';
	//   if (await confirmDelete(`host "${d.label}"`, { danger: true })) { … }
	//
	// `confirmDelete` resolves to a Promise<boolean>. It drives a single shared
	// dialog instance mounted once in +layout.svelte (so it covers every route
	// without per-screen wiring). The mounted component registers itself here on
	// mount; call sites import only the `confirmDelete` helper below.

	let activeInstance = null; // the currently-registered component instance

	export function registerConfirmDialog(instance) {
		activeInstance = instance;
		return () => {
			if (activeInstance === instance) activeInstance = null;
		};
	}

	// Module-level helper — the canonical call site API. Returns false (never
	// auto-confirms a delete) if no dialog is mounted (SSR / no DOM).
	export async function confirmDelete(label, opts = {}) {
		if (!activeInstance) return false;
		const message = `Delete ${label}?`;
		return activeInstance.confirmDelete(message, opts);
	}
</script>

<script>
	import { onMount, onDestroy, tick } from 'svelte';
	import Face from '$lib/Face.svelte';

	// --- Contract props ------------------------------------------------
	export let message = '';
	export let danger = false;
	export let confirmLabel = 'delete';

	// --- Internal open/resolve state ----------------------------------
	let open = false;
	let resolver = null; // resolves the outstanding Promise<boolean>
	let dialogEl;
	let confirmBtn;
	let prevFocus = null;

	// Per-open overrides (so the same instance serves every call site).
	let msg = '';
	let isDanger = false;
	let label = '';

	$: msg = message;
	$: isDanger = danger;
	$: label = confirmLabel;

	// Imperative API — instance method matching the contract's
	// confirmDelete(label, { danger }) -> Promise<boolean>. `confirmText`
	// optionally overrides the button label for non-delete confirms.
	export function confirmDelete(promptMessage, opts = {}) {
		return show(promptMessage, opts);
	}

	function show(promptMessage, opts = {}) {
		// Resolve any previously-open prompt as cancelled before replacing it.
		if (resolver) {
			resolver(false);
			resolver = null;
		}
		msg = promptMessage != null ? promptMessage : message;
		isDanger = opts.danger != null ? opts.danger : danger;
		label = opts.confirmLabel != null ? opts.confirmLabel : confirmLabel;
		prevFocus = typeof document !== 'undefined' ? document.activeElement : null;
		open = true;
		focusConfirm();
		return new Promise((resolve) => {
			resolver = resolve;
		});
	}

	async function focusConfirm() {
		await tick();
		if (confirmBtn) confirmBtn.focus();
	}

	function settle(result) {
		open = false;
		if (resolver) {
			resolver(result);
			resolver = null;
		}
		if (prevFocus && typeof prevFocus.focus === 'function') prevFocus.focus();
		prevFocus = null;
	}

	function confirm() {
		settle(true);
	}

	function cancel() {
		settle(false);
	}

	// Esc closes (cancels); focus trap keeps Tab within the dialog.
	function onKeydown(e) {
		if (!open) return;
		if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			cancel();
			return;
		}
		if (e.key === 'Tab') trapFocus(e);
	}

	function trapFocus(e) {
		if (!dialogEl) return;
		const focusable = dialogEl.querySelectorAll(
			'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
		);
		if (focusable.length === 0) return;
		const first = focusable[0];
		const last = focusable[focusable.length - 1];
		const activeEl = document.activeElement;
		if (e.shiftKey && activeEl === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && activeEl === last) {
			e.preventDefault();
			first.focus();
		}
	}

	let unregister = null;
	onMount(() => {
		unregister = registerConfirmDialog({ confirmDelete });
	});
	onDestroy(() => {
		if (unregister) unregister();
		// Resolve any dangling prompt so awaiters never hang on teardown.
		if (resolver) {
			resolver(false);
			resolver = null;
		}
	});
</script>

<svelte:window on:keydown={onKeydown} />

{#if open}
	<!-- svelte-ignore a11y-click-events-have-key-events -->
	<!-- svelte-ignore a11y-no-static-element-interactions -->
	<div class="confirm-scrim overlay-scrim" on:click={cancel}></div>
	<div
		class="confirm"
		role="dialog"
		aria-modal="true"
		aria-labelledby="confirm-title"
		bind:this={dialogEl}
	>
		<!-- svelte-ignore a11y-click-events-have-key-events -->
		<!-- svelte-ignore a11y-no-static-element-interactions -->
		<div class="confirm__card overlay-surface" on:click|stopPropagation>
			<p class="confirm__title" id="confirm-title">
				<Face state={isDanger ? 'error' : 'ok'} />
				<span class="confirm__message">{msg}</span>
			</p>

			<div class="confirm__body">
				<!-- Optional extra body, e.g. sample names for bulk delete. -->
				<slot />
				{#if isDanger}
					<p class="confirm__warn">this cannot be undone.</p>
				{/if}
			</div>

			<div class="confirm__actions">
				<button type="button" class="secondary" on:click={cancel}>cancel</button>
				<button
					type="button"
					class="primary"
					class:confirm__confirm--danger={isDanger}
					bind:this={confirmBtn}
					on:click={confirm}
				>
					{label}
				</button>
			</div>
		</div>
	</div>
{/if}

<style>
	.confirm-scrim {
		z-index: var(--z-modal-scrim);
	}

	.confirm {
		position: fixed;
		inset: 0;
		z-index: var(--z-modal);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-lg);
		padding-top: max(var(--space-lg), var(--safe-top));
		padding-bottom: max(var(--space-lg), var(--safe-bottom));
		padding-left: max(var(--space-lg), var(--safe-left));
		padding-right: max(var(--space-lg), var(--safe-right));
	}

	.confirm__card {
		width: 100%;
		max-width: 28rem;
		padding: var(--space-lg);
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	.confirm__title {
		display: flex;
		align-items: baseline;
		gap: var(--space-sm);
		margin: 0;
		font-size: var(--fs-md);
		font-weight: 600;
		line-height: var(--lh-tight);
	}

	.confirm__message {
		overflow-wrap: anywhere;
	}

	.confirm__body {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		font-size: var(--fs-sm);
		color: var(--text-muted);
		line-height: var(--lh-body);
	}

	.confirm__warn {
		margin: 0;
		color: var(--err);
		font-size: var(--fs-xs);
	}

	.confirm__actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-sm);
		margin-top: var(--space-xs);
	}

	.confirm__confirm--danger {
		background: var(--err);
		border-color: var(--err);
		color: #fff;
	}

	.confirm__confirm--danger:hover:not(:disabled) {
		filter: brightness(1.08);
		border-color: var(--err);
	}

	@media (max-width: 640px) {
		.confirm__actions {
			flex-direction: column-reverse;
		}
		.confirm__actions button {
			width: 100%;
		}
	}
</style>
