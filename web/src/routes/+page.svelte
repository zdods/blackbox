<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { getToken } from '$lib/auth.js';
	import Face from '$lib/Face.svelte';

	onMount(async () => {
		if (getToken()) {
			goto('/dashboard');
			return;
		}
		try {
			const res = await fetch('/api/setup');
			if (res.ok) {
				const data = await res.json();
				if (data.registration_open === true) {
					goto('/register');
					return;
				}
			}
		} catch (e) {
			console.error('setup check failed:', e);
		}
		goto('/login');
	});
</script>

<p class="muted splash" role="status" aria-live="polite"><Face state="loading" /> loading…</p>

<style>
	.splash {
		text-align: center;
		margin-top: var(--space-2xl);
	}
</style>
