<script>
	import { onMount, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { isLoggedIn, setLoggedIn } from '$lib/auth.js';
	import { setRegisterCredentials, clearRegisterCredentials } from '$lib/register-state.js';
	import { passkeyRegister } from '$lib/passkey.js';
	import Face from '$lib/Face.svelte';

	let username = '';
	let password = '';
	let error = '';
	let loading = false;
	let setupLoading = true;
	let registrationOpen = false;
	let showPassword = false;
	// Which registration methods to offer (server-driven; both may be true).
	let passwordAuth = true;
	let passkeyAuth = false;

	let userInvalid = false;
	let passInvalid = false;
	let usernameInput;
	let pkUsername = '';
	let pkUserInvalid = false;
	let pkUsernameInput;

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
				passwordAuth = data.password_enabled === true;
				passkeyAuth = data.passkey_enabled === true;
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
		await tick();
		// Passkey is the lower-friction path; focus its field first when offered.
		if (passkeyAuth && pkUsernameInput) pkUsernameInput.focus();
		else if (passwordAuth && usernameInput) usernameInput.focus();
	});

	function handleContinue(e) {
		e.preventDefault();
		error = '';
		userInvalid = false;
		passInvalid = false;
		const u = username.trim();
		const p = password;
		if (!u) userInvalid = true;
		if (!p) passInvalid = true;
		if (!u || !p) {
			error = 'Username and password are required';
			return;
		}
		setRegisterCredentials(u, p);
		goto('/register/totp');
	}

	async function handlePasskeyRegister(e) {
		e.preventDefault();
		error = '';
		pkUserInvalid = false;
		const u = pkUsername.trim();
		if (!u) {
			pkUserInvalid = true;
			error = 'Choose a username';
			return;
		}
		loading = true;
		try {
			await passkeyRegister(u, 'Default passkey');
			setLoggedIn();
			goto('/dashboard');
		} catch (err) {
			error = err?.message || 'Could not create passkey';
		} finally {
			loading = false;
		}
	}
</script>

<div class="card auth-card">
	{#if setupLoading}
		<p class="muted" role="status" aria-live="polite"><Face state="loading" /> loading…</p>
	{:else}
		<header class="auth-head">
			<p class="microlabel auth-step">create account</p>
		</header>
		<h1 class="page-title">register</h1>
		<p class="page-sub">One-time setup — create the owner account.</p>

		{#if error}
			<p class="error" id="auth-error" role="alert"><Face state="error" /> {error}</p>
		{/if}

		{#if passkeyAuth}
			<form on:submit={handlePasskeyRegister} class="auth-form">
				<div class="form-row">
					<label class="field-label" for="pk-username">username</label>
					<input
						id="pk-username"
						name="username"
						type="text"
						autocomplete="username webauthn"
						autocapitalize="none"
						spellcheck="false"
						class:input--invalid={pkUserInvalid}
						aria-invalid={pkUserInvalid}
						bind:this={pkUsernameInput}
						bind:value={pkUsername}
						on:input={() => (pkUserInvalid = false)}
						placeholder="pick-a-username"
						required
					/>
				</div>
				<button type="submit" class="primary" disabled={loading || !pkUsername.trim()}>
					{loading ? 'waiting for passkey…' : 'create passkey'}
				</button>
			</form>
		{/if}

		{#if passkeyAuth && passwordAuth}
			<div class="auth-or" aria-hidden="true"><span>or</span></div>
		{/if}

		{#if passwordAuth}
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
						class:input--invalid={userInvalid}
						aria-invalid={userInvalid}
						bind:this={usernameInput}
						bind:value={username}
						on:input={() => (userInvalid = false)}
						placeholder="pick-a-username"
						required
					/>
				</div>
				<div class="form-row">
					<label class="field-label" for="password">password</label>
					<div class="auth-pass" class:auth-pass--invalid={passInvalid}>
						<input
							id="password"
							name="password"
							type={showPassword ? 'text' : 'password'}
							autocomplete="new-password"
							class="auth-pass__input"
							class:input--invalid={passInvalid}
							aria-invalid={passInvalid}
							bind:value={password}
							on:input={() => (passInvalid = false)}
							placeholder="••••••••"
							required
						/>
						<button
							type="button"
							class="auth-pass__toggle"
							aria-pressed={showPassword}
							aria-label={showPassword ? 'hide password' : 'show password'}
							on:click={() => (showPassword = !showPassword)}
						>
							{showPassword ? 'hide' : 'show'}
						</button>
					</div>
				</div>
				<button type="submit" class="primary" disabled={loading || !username.trim() || !password}>
					{loading ? 'continuing…' : 'continue'}
				</button>
			</form>
		{/if}
	{/if}
</div>

<style>
	.auth-card {
		width: 24rem;
		max-width: 100%;
		padding: var(--space-xl);
	}
	.auth-head {
		margin-bottom: var(--space-md);
	}

	/* "or" divider between the passkey and password registration options. */
	.auth-or {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		margin: var(--space-lg) 0;
		color: var(--text-faint);
		font-size: var(--fs-xs);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}
	.auth-or::before,
	.auth-or::after {
		content: '';
		flex: 1;
		height: 1px;
		background: var(--border);
	}
	.auth-step {
		margin: 0;
		color: var(--text-faint);
	}
	.auth-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-lg);
		margin-top: var(--space-lg);
	}

	.auth-form :global(.input--invalid) {
		border-color: var(--err);
	}
	.auth-form :global(.input--invalid:focus),
	.auth-form :global(.input--invalid:focus-visible) {
		border-color: var(--err);
		box-shadow: 0 0 0 3px var(--err-soft);
	}

	.auth-pass {
		position: relative;
		display: flex;
		align-items: center;
	}
	.auth-pass__input {
		padding-right: 4rem;
	}
	.auth-pass__toggle {
		position: absolute;
		right: var(--space-xs);
		top: 50%;
		transform: translateY(-50%);
		height: 2rem;
		min-width: 3rem;
		padding: 0 var(--space-sm);
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-xs);
		color: var(--text-muted);
		font-size: var(--fs-xs);
	}
	.auth-pass__toggle:hover:not(:disabled) {
		color: var(--text);
		border-color: var(--border-strong);
	}

	@media (max-width: 640px) {
		.auth-pass__toggle {
			height: calc(var(--touch-min) - 0.5rem);
			min-width: var(--touch-min);
		}
	}
</style>
