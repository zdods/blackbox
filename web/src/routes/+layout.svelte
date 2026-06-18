<script>
	import { page } from '$app/stores';
	import { goto, afterNavigate } from '$app/navigation';
	import { onDestroy, tick } from 'svelte';
	import { isLoggedIn, clearLoggedIn, apiFetch } from '$lib/auth.js';
	import { start as startHosts, stop as stopHosts } from '$lib/hosts.js';
	import Face from '$lib/Face.svelte';
	import ThemeSelect from '$lib/ThemeSelect.svelte';
	import Sidebar from '$lib/Sidebar.svelte';
	import CommandPalette from '$lib/CommandPalette.svelte';
	import ConfirmDialog from '$lib/ConfirmDialog.svelte';
	import '../app.css';

	// Auth pages (login / register / register/totp) and the landing redirector
	// get the centered card layout — no rail, no palette.
	$: centered = $page.url.pathname === '/login' || $page.url.pathname.startsWith('/register');
	// Re-check auth on every navigation (login flag in localStorage; the real
	// session is an httpOnly cookie).
	$: authed = $page.url && typeof window !== 'undefined' && !!isLoggedIn();
	// The rail/palette only exist for the authed app shell.
	$: shellActive = authed && !centered;

	let drawerOpen = false;
	let paletteOpen = false;
	let hamburgerEl;
	let sidebarEl;

	const FOCUSABLE =
		'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

	// When the mobile drawer opens, pull focus into it; Tab is trapped there
	// (see onWindowKeydown) until it closes, when focus returns to the hamburger.
	$: if (drawerOpen) focusDrawer();
	async function focusDrawer() {
		await tick();
		if (!sidebarEl) return;
		const first = sidebarEl.querySelector(FOCUSABLE);
		if (first) first.focus();
	}
	function trapDrawerFocus(e) {
		if (!sidebarEl) return;
		const focusable = sidebarEl.querySelectorAll(FOCUSABLE);
		if (focusable.length === 0) return;
		const first = focusable[0];
		const last = focusable[focusable.length - 1];
		const active = document.activeElement;
		if (!sidebarEl.contains(active)) {
			e.preventDefault();
			first.focus();
		} else if (e.shiftKey && active === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && active === last) {
			e.preventDefault();
			first.focus();
		}
	}

	// --- Hosts poll lifecycle: one shared poller, started when the shell is
	// active and stopped otherwise (covers logout + navigation to auth pages).
	let polling = false;
	$: syncPoll(shellActive);
	function syncPoll(active) {
		if (typeof window === 'undefined') return;
		if (active && !polling) {
			polling = true;
			startHosts();
		} else if (!active && polling) {
			polling = false;
			stopHosts();
		}
	}
	onDestroy(() => {
		if (polling) {
			polling = false;
			stopHosts();
		}
	});

	async function logout() {
		try {
			await apiFetch('/api/logout', { method: 'POST' });
		} catch (_) {}
		clearLoggedIn();
		goto('/login');
	}

	function openPalette() {
		paletteOpen = true;
		drawerOpen = false;
	}

	function toggleDrawer() {
		drawerOpen = !drawerOpen;
	}

	function closeDrawer() {
		drawerOpen = false;
	}

	// Close the drawer on navigation (e.g. host selection) and return focus.
	afterNavigate(() => {
		drawerOpen = false;
	});

	// Esc-router: a single document-level handler so the innermost overlay wins.
	// Precedence: palette > drawer > (preview/other overlays handle their own).
	// ⌘K / Ctrl+K toggles the palette, but not while typing in a field.
	function onWindowKeydown(e) {
		const k = e.key.toLowerCase();
		const meta = e.metaKey || e.ctrlKey;
		if (meta && k === 'k') {
			const t = e.target;
			const tag = t && t.tagName ? t.tagName.toLowerCase() : '';
			const typing = tag === 'input' || tag === 'textarea' || (t && t.isContentEditable);
			// Allow ⌘K to still open the palette unless focus is in a field AND it
			// isn't our own palette input.
			if (typing && !(t && t.classList && t.classList.contains('palette__input'))) return;
			if (!shellActive) return;
			e.preventDefault();
			paletteOpen = !paletteOpen;
			return;
		}
		if (e.key === 'Tab' && drawerOpen && !paletteOpen) {
			trapDrawerFocus(e);
			return;
		}
		if (e.key === 'Escape') {
			if (paletteOpen) {
				paletteOpen = false;
				e.stopPropagation();
				return;
			}
			if (drawerOpen) {
				closeDrawer();
				returnFocusToHamburger();
				e.stopPropagation();
			}
			// Otherwise let preview/other overlays handle Esc themselves.
		}
	}

	async function returnFocusToHamburger() {
		await tick();
		if (hamburgerEl) hamburgerEl.focus();
	}

	// Lock body scroll while the mobile drawer is open.
	$: if (typeof document !== 'undefined') {
		document.body.style.overflow = drawerOpen ? 'hidden' : '';
	}
</script>

<svelte:window on:keydown={onWindowKeydown} />

<div class="app">
	<header class="app-header">
		<div class="app-header__lead">
			{#if shellActive}
				<button
					type="button"
					class="app-header__hamburger"
					bind:this={hamburgerEl}
					aria-label="toggle hosts"
					aria-expanded={drawerOpen}
					aria-controls="app-sidebar"
					on:click={toggleDrawer}
				>
					<span aria-hidden="true">☰</span>
				</button>
			{/if}
			<a class="app-header__brand" href={authed ? '/dashboard' : '/'}>
				<Face state="ok" />
				<span>blackhaul</span>
			</a>
		</div>
		<div class="app-header__actions">
			{#if shellActive}
				<button type="button" class="app-header__cmdk" on:click={openPalette} aria-label="open command palette">
					<span class="app-header__cmdk-glyph" aria-hidden="true">⌘K</span>
					<span class="app-header__cmdk-text">search…</span>
				</button>
			{/if}
			<ThemeSelect />
			{#if authed}
				<button type="button" class="secondary logout-btn" on:click={logout}>log out</button>
			{/if}
		</div>
	</header>

	<div class="app-body">
		{#if shellActive}
			<!-- svelte-ignore a11y-click-events-have-key-events -->
			<!-- svelte-ignore a11y-no-static-element-interactions -->
			<div
				class="drawer-scrim"
				class:drawer-scrim--show={drawerOpen}
				on:click={() => {
					closeDrawer();
					returnFocusToHamburger();
				}}
			></div>
			<div
				id="app-sidebar"
				class="app-sidebar"
				class:app-sidebar--open={drawerOpen}
				bind:this={sidebarEl}
			>
				<Sidebar {drawerOpen} on:openPalette={openPalette} on:navigate={closeDrawer} />
			</div>
		{/if}

		<main class="app-main" class:centered>
			<slot />
		</main>
	</div>
</div>

{#if shellActive}
	<CommandPalette bind:open={paletteOpen} on:close={() => (paletteOpen = false)} on:logout={logout} />
{/if}

<!-- Single shared delete-confirmation dialog for every route (dashboard +
     file browser call confirmDelete() from $lib/ConfirmDialog.svelte). -->
<ConfirmDialog />

<style>
	.logout-btn {
		height: 2rem;
		padding: 0 var(--space-md);
		font-size: var(--fs-xs);
		white-space: nowrap;
	}

	.app-header__hamburger {
		display: none;
		height: 2.25rem;
		width: 2.25rem;
		padding: 0;
		align-items: center;
		justify-content: center;
		background: transparent;
		border: 1px solid var(--border);
		color: var(--text-muted);
		flex-shrink: 0;
	}

	.app-header__cmdk {
		display: inline-flex;
		align-items: center;
		gap: var(--space-sm);
		height: 2rem;
		padding: 0 var(--space-md) 0 var(--space-sm);
		font-size: var(--fs-xs);
		color: var(--text-muted);
		background: var(--inset);
		border: 1px solid var(--border);
	}

	.app-header__cmdk:hover {
		color: var(--text);
		border-color: var(--border-strong);
	}

	.app-header__cmdk-glyph {
		font-size: var(--fs-2xs);
		color: var(--text-faint);
	}

	/* Drawer scrim is inert (no flow, no pointer) until the drawer opens. */
	.drawer-scrim {
		display: none;
	}

	@media (max-width: 640px) {
		.app-header__hamburger {
			display: inline-flex;
			min-height: var(--touch-min);
			min-width: var(--touch-min);
		}

		.app-header__cmdk-text {
			display: none;
		}

		.app-header__cmdk {
			padding: 0 var(--space-sm);
			min-height: var(--touch-min);
			min-width: var(--touch-min);
			justify-content: center;
		}

		/* Sidebar leaves flow and becomes an off-canvas drawer. */
		.app-sidebar {
			position: fixed;
			top: var(--header-height);
			left: 0;
			bottom: 0;
			z-index: var(--z-drawer);
			transform: translateX(-100%);
			padding-left: var(--safe-left);
		}

		.app-sidebar--open {
			transform: translateX(0);
		}

		.drawer-scrim {
			display: block;
			position: fixed;
			inset: var(--header-height) 0 0 0;
			z-index: var(--z-drawer-scrim);
			background: var(--backdrop);
			opacity: 0;
			pointer-events: none;
		}

		.drawer-scrim--show {
			opacity: 1;
			pointer-events: auto;
		}

		@media (prefers-reduced-motion: no-preference) {
			.app-sidebar {
				transition: transform var(--dur-slow) var(--ease-soft);
			}
			.drawer-scrim {
				transition: opacity var(--dur-slow) var(--ease-soft);
			}
		}

		.logout-btn {
			padding: 0 var(--space-sm);
			min-height: var(--touch-min);
		}
	}
</style>
