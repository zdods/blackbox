<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { isLoggedIn, setLoggedIn } from '$lib/auth.js';
	import Face from '$lib/Face.svelte';

	let username = '';
	let password = '';
	let totpCode = '';
	let error = '';
	let loading = false;
	let registrationOpen = true;
	let setupLoading = true;
	let loginToken = '';
	let step = 'password'; // 'password' | 'totp'

	$: if (typeof window !== 'undefined' && isLoggedIn()) {
		goto('/dashboard');
	}

	onMount(async () => {
		try {
			const res = await fetch('/api/setup');
			if (res.ok) {
				const data = await res.json();
				registrationOpen = data.registration_open === true;
				if (registrationOpen) {
					goto('/register');
					return;
				}
			}
		} catch (e) {
			console.error('setup check failed:', e);
		}
		setupLoading = false;
	});

	async function handleSubmit(e) {
		e.preventDefault();
		error = '';
		loading = true;
		try {
			const res = await fetch('/api/login', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ username, password })
			});
			const data = await res.json().catch(() => ({}));
			if (!res.ok) {
				error = data.error || res.statusText || 'Login failed';
				return;
			}
			if (data.requires_totp === true && data.login_token) {
				loginToken = data.login_token;
				step = 'totp';
				totpCode = '';
				return;
			}
			// Server sets httpOnly session cookie; store a flag for client-side auth checks.
			setLoggedIn();
			goto('/dashboard');
		} finally {
			loading = false;
		}
	}

	async function handleTotpSubmit(e) {
		e.preventDefault();
		error = '';
		loading = true;
		try {
			const res = await fetch('/api/login/totp', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					login_token: loginToken,
					code: totpCode.replace(/\s/g, '')
				})
			});
			const data = await res.json().catch(() => ({}));
			if (!res.ok) {
				error = data.error || res.statusText || 'Verification failed';
				return;
			}
			// Server sets httpOnly session cookie; store a flag for client-side auth checks.
			setLoggedIn();
			goto('/dashboard');
		} finally {
			loading = false;
		}
	}

	function backToPassword() {
		step = 'password';
		loginToken = '';
		totpCode = '';
		error = '';
	}
</script>

<div class="card auth-card">
	{#if step === 'totp'}
		<h1 class="page-title">two-factor</h1>
		<p class="page-sub">Enter the code from your authenticator app.</p>
		<form method="post" action="/login" on:submit={handleTotpSubmit} class="auth-form">
			<!-- Hidden username keeps password managers oriented in the multi-step flow -->
			<input
				type="text"
				name="username"
				autocomplete="username"
				value={username}
				readonly
				hidden
				tabindex="-1"
				aria-hidden="true"
			/>
			<div class="form-row">
				<label class="field-label" for="totp">2FA code</label>
				<input
					id="totp"
					name="totp"
					type="text"
					inputmode="numeric"
					autocomplete="one-time-code"
					bind:value={totpCode}
					placeholder="000000"
					maxlength="8"
				/>
			</div>
			{#if error}<p class="error" role="alert"><Face state="error" /> {error}</p>{/if}
			<button type="submit" class="primary" disabled={loading || !totpCode.trim()}>
				{loading ? 'verifying…' : 'verify'}
			</button>
			<button type="button" class="quiet" on:click={backToPassword}>back</button>
		</form>
	{:else}
		<h1 class="page-title">log in</h1>
		<p class="page-sub">Welcome back.</p>
		<form method="post" action="/login" on:submit={handleSubmit} class="auth-form">
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
					placeholder="your-username"
					required
				/>
			</div>
			<div class="form-row">
				<label class="field-label" for="password">password</label>
				<input
					id="password"
					name="password"
					type="password"
					autocomplete="current-password"
					bind:value={password}
					placeholder="••••••••"
					required
				/>
			</div>
			{#if error}<p class="error" role="alert"><Face state="error" /> {error}</p>{/if}
			<button type="submit" class="primary" disabled={loading || !username.trim() || !password}
				>{loading ? 'logging in…' : 'log in'}</button
			>
		</form>
		{#if !setupLoading && registrationOpen}
			<p class="muted auth-footnote"><a href="/register">register</a> (one-time setup)</p>
		{/if}
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
	.auth-footnote {
		margin: var(--space-lg) 0 0;
	}
</style>
