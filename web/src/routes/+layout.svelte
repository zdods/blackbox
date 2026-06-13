<script>
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { getToken, clearToken, apiFetch } from '$lib/auth.js';
	import Face from '$lib/Face.svelte';
	import ThemeSelect from '$lib/ThemeSelect.svelte';
	import '../app.css';

	$: centered = $page.url.pathname === '/login' || $page.url.pathname.startsWith('/register');
	// Re-check auth on every navigation (token lives in localStorage).
	$: authed = $page.url && typeof window !== 'undefined' && !!getToken();

	async function logout() {
		try {
			await apiFetch('/api/logout', { method: 'POST' });
		} catch (_) {}
		clearToken();
		goto('/login');
	}
</script>

<div class="app">
	<header class="app-header">
		<a class="app-header__brand" href={authed ? '/dashboard' : '/'}>
			<Face state="ok" />
			<span>blackhaul</span>
		</a>
		<div class="app-header__actions">
			<ThemeSelect />
			{#if authed}
				<button type="button" class="secondary logout-btn" on:click={logout}>log out</button>
			{/if}
		</div>
	</header>
	<main class="app-main" class:centered>
		<slot />
	</main>
</div>

<style>
	.logout-btn {
		height: 2rem;
		padding: 0 var(--space-md);
		font-size: 0.8rem;
		white-space: nowrap;
	}

	/* Tighten the header on small screens so the brand, theme picker and
	   logout button share a single row without the button text wrapping. */
	@media (max-width: 640px) {
		.logout-btn {
			padding: 0 var(--space-sm);
		}
	}
</style>
