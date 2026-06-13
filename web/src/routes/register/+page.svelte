<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { isLoggedIn } from '$lib/auth.js';
	import { setRegisterCredentials, clearRegisterCredentials } from '$lib/register-state.js';
	import Face from '$lib/Face.svelte';

	let username = '';
	let password = '';
	let error = '';
	let loading = false;
	let setupLoading = true;
	let registrationOpen = false;

	$: if (typeof window !== 'undefined' && isLoggedIn()) {
		goto('/dashboard');
	}

	onMount(async () => {
		clearRegisterCredentials();
		try {
			const res = await fetch('/api/setup');
			if (res.ok) {
				const data = await res.json();
				registrationOpen = data.registration_open === true;
				if (!registrationOpen) {
					goto('/login?registration=closed', { replaceState: true });
					return;
				}
			} else {
				goto('/login?registration=closed', { replaceState: true });
				return;
			}
		} catch (e) {
			console.error('setup check failed:', e);
			goto('/login?registration=closed', { replaceState: true });
			return;
		}
		setupLoading = false;
	});

	function handleContinue(e) {
		e.preventDefault();
		error = '';
		const u = username.trim();
		const p = password;
		if (!u || !p) {
			error = 'Username and password are required';
			return;
		}
		setRegisterCredentials(u, p);
		goto('/register/totp');
	}
</script>

<div class="card auth-card">
	{#if setupLoading}
		<p class="muted" role="status" aria-live="polite"><Face state="loading" /> loading…</p>
	{:else}
		<h1 class="page-title">register</h1>
		<p class="page-sub">One-time setup — create the owner account.</p>
		<form method="post" action="/register" on:submit={handleContinue} class="auth-form">
			<div class="form-row">
				<label class="field-label" for="username">username</label>
				<input
					id="username"
					name="username"
					type="text"
					autocomplete="username"
					autocapitalize="none"
					spellcheck="false"
					bind:value={username}
					placeholder="pick-a-username"
					required
				/>
			</div>
			<div class="form-row">
				<label class="field-label" for="password">password</label>
				<input
					id="password"
					name="password"
					type="password"
					autocomplete="new-password"
					bind:value={password}
					placeholder="••••••••"
					required
				/>
			</div>
			{#if error}<p class="error" role="alert"><Face state="error" /> {error}</p>{/if}
			<button type="submit" class="primary" disabled={loading || !username.trim() || !password}>
				{loading ? 'continuing…' : 'continue'}
			</button>
		</form>
	{/if}
</div>

<style>
	.auth-card {
		width: 24rem;
		max-width: 100%;
		padding: var(--space-xl);
	}
	.auth-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-lg);
		margin-top: var(--space-lg);
	}
</style>
