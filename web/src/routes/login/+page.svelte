<script>
	import { onMount, tick } from 'svelte';
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
	let showPassword = false;

	// Field-level invalid flags (cleared on edit / step change).
	let userInvalid = false;
	let passInvalid = false;
	let codeInvalid = false;

	// Focus management between steps.
	let usernameInput;
	let totpInput;

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
		await tick();
		if (usernameInput) usernameInput.focus();
	});

	async function handleSubmit(e) {
		e.preventDefault();
		error = '';
		userInvalid = false;
		passInvalid = false;
		if (!username.trim()) userInvalid = true;
		if (!password) passInvalid = true;
		if (userInvalid || passInvalid) {
			error = 'Username and password are required';
			return;
		}
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
				userInvalid = true;
				passInvalid = true;
				return;
			}
			if (data.requires_totp === true && data.login_token) {
				loginToken = data.login_token;
				step = 'totp';
				totpCode = '';
				codeInvalid = false;
				await tick();
				if (totpInput) totpInput.focus();
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
		codeInvalid = false;
		if (!totpCode.trim()) {
			codeInvalid = true;
			error = 'Enter the code from your authenticator app';
			return;
		}
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
				codeInvalid = true;
				return;
			}
			// Server sets httpOnly session cookie; store a flag for client-side auth checks.
			setLoggedIn();
			goto('/dashboard');
		} finally {
			loading = false;
		}
	}

	async function backToPassword() {
		step = 'password';
		loginToken = '';
		totpCode = '';
		error = '';
		codeInvalid = false;
		showPassword = false;
		await tick();
		if (usernameInput) usernameInput.focus();
	}
</script>

<div class="card auth-card">
	{#if step === 'totp'}
		<header class="auth-head">
			<button type="button" class="auth-back" on:click={backToPassword}>
				<span class="auth-back__glyph" aria-hidden="true">←</span> back
			</button>
			<p class="microlabel auth-step">step 2 of 2 · two-factor</p>
		</header>
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
					class="num auth-input--code"
					class:input--invalid={codeInvalid}
					aria-invalid={codeInvalid}
					aria-describedby={error ? 'auth-error' : undefined}
					bind:this={totpInput}
					bind:value={totpCode}
					on:input={() => (codeInvalid = false)}
					placeholder="000000"
					maxlength="8"
				/>
			</div>
			{#if error}
				<p class="error" id="auth-error" role="alert"><Face state="error" /> {error}</p>
			{/if}
			<button type="submit" class="primary" disabled={loading || !totpCode.trim()}>
				{loading ? 'verifying…' : 'verify'}
			</button>
		</form>
	{:else}
		<header class="auth-head">
			<p class="microlabel auth-step">step 1 of 2 · sign in</p>
		</header>
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
					class:input--invalid={userInvalid}
					aria-invalid={userInvalid}
					bind:this={usernameInput}
					bind:value={username}
					on:input={() => (userInvalid = false)}
					placeholder="your-username"
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
						autocomplete="current-password"
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
			{#if error}
				<p class="error" id="auth-error" role="alert"><Face state="error" /> {error}</p>
			{/if}
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
	.auth-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-md);
		margin-bottom: var(--space-md);
		min-height: 1.5rem;
	}
	.auth-step {
		margin: 0;
		color: var(--text-faint);
	}
	.auth-back {
		display: inline-flex;
		align-items: center;
		gap: var(--space-xs);
		height: auto;
		padding: var(--space-xs) var(--space-sm);
		background: var(--inset);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--text-muted);
		font-size: var(--fs-xs);
	}
	.auth-back:hover:not(:disabled) {
		color: var(--text);
		border-color: var(--border-strong);
	}
	.auth-back__glyph {
		font-size: 1em;
		line-height: 1;
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

	/* Field-level error state: accent the border with --err. */
	.auth-form :global(.input--invalid) {
		border-color: var(--err);
	}
	.auth-form :global(.input--invalid:focus),
	.auth-form :global(.input--invalid:focus-visible) {
		border-color: var(--err);
		box-shadow: 0 0 0 3px var(--err-soft);
	}

	.auth-input--code {
		letter-spacing: 0.25em;
	}

	/* Show/hide password toggle. */
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
		.auth-back {
			min-height: var(--touch-min);
			padding: 0 var(--space-md);
		}
		.auth-pass__toggle {
			height: calc(var(--touch-min) - 0.5rem);
			min-width: var(--touch-min);
		}
	}
</style>
