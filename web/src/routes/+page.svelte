<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { isLoggedIn } from '$lib/auth.js';
	import Face from '$lib/Face.svelte';

	onMount(async () => {
		if (isLoggedIn()) {
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

<div class="splash" data-testid="landing-splash">
	<p class="splash__status" role="status" aria-live="polite">
		<Face state="loading" />
		<span class="splash__text">loading…</span>
	</p>
</div>

<style>
	.splash {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: var(--space-md);
		min-height: 40vh;
		padding: var(--space-2xl) var(--space-md);
		text-align: center;
	}

	.splash__status {
		display: inline-flex;
		align-items: center;
		gap: var(--space-sm);
		margin: 0;
		font-size: var(--fs-md);
		color: var(--text-muted);
	}

	.splash__text {
		letter-spacing: var(--tracking-tight);
	}
</style>
