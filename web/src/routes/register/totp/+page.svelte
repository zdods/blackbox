<script>
	import { onMount, onDestroy, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { isLoggedIn } from '$lib/auth.js';
	import { getRegisterCredentials, clearRegisterCredentials } from '$lib/register-state.js';
	import Face from '$lib/Face.svelte';
	import QRCode from 'qrcode';

	let totpCode = '';
	let regUsername = '';
	let error = '';
	let loading = false;
	let setupLoading = true;
	let setupError = '';
	let setupId = '';
	let secret = '';
	let qrDataUrl = '';
	let copied = false;
	let copyTimeout;

	let codeInvalid = false;
	let totpInput;

	$: if (typeof window !== 'undefined' && isLoggedIn()) {
		goto('/dashboard');
	}

	onMount(async () => {
		const credentials = getRegisterCredentials();
		if (!credentials) {
			goto('/register', { replaceState: true });
			return;
		}
		regUsername = credentials.username;

		try {
			const res = await fetch('/api/setup');
			if (res.ok) {
				const data = await res.json();
				if (data.registration_open !== true) {
					clearRegisterCredentials();
					goto('/login?registration=closed', { replaceState: true });
					return;
				}
			} else {
				clearRegisterCredentials();
				goto('/login?registration=closed', { replaceState: true });
				return;
			}

			const setupRes = await fetch('/api/register/totp-setup', { method: 'POST' });
			if (!setupRes.ok) {
				const data = await setupRes.json().catch(() => ({}));
				setupError = data.error || 'Failed to load 2FA setup';
				setupLoading = false;
				return;
			}
			const setupData = await setupRes.json();
			setupId = setupData.setup_id || '';
			secret = setupData.secret || '';
			const uri = setupData.provisioning_uri || '';
			if (uri) {
				qrDataUrl = await QRCode.toDataURL(uri, { width: 200, margin: 1 });
			}
		} catch (e) {
			setupError = e.message || 'Setup failed';
		}
		setupLoading = false;
		await tick();
		if (totpInput) totpInput.focus();
	});

	onDestroy(() => {
		clearTimeout(copyTimeout);
	});

	async function copySecret() {
		if (!secret) return;
		try {
			await navigator.clipboard.writeText(secret);
			copied = true;
			clearTimeout(copyTimeout);
			copyTimeout = setTimeout(() => (copied = false), 2000);
		} catch (e) {
			console.error('clipboard write failed:', e);
		}
	}

	async function handleSubmit(e) {
		e.preventDefault();
		error = '';
		codeInvalid = false;
		const credentials = getRegisterCredentials();
		if (!credentials) {
			goto('/register', { replaceState: true });
			return;
		}
		const { username: u, password: p } = credentials;
		if (!u || !p) {
			clearRegisterCredentials();
			goto('/register', { replaceState: true });
			return;
		}
		if (!totpCode.trim()) {
			codeInvalid = true;
			error = 'Enter the code from your authenticator app';
			return;
		}

		loading = true;
		try {
			const res = await fetch('/api/register', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					username: u,
					password: p,
					totp_code: totpCode.replace(/\s/g, ''),
					setup_id: setupId
				})
			});
			const data = await res.json().catch(() => ({}));
			if (!res.ok) {
				error = data.error || res.statusText || 'Registration failed';
				codeInvalid = true;
				if (res.status === 403) {
					clearRegisterCredentials();
					goto('/login?registration=closed', { replaceState: true });
					return;
				}
				return;
			}
			clearRegisterCredentials();
			goto('/login');
		} finally {
			loading = false;
		}
	}
</script>

<div class="card auth-card">
	{#if setupLoading}
		<p class="muted" role="status" aria-live="polite"><Face state="loading" /> loading…</p>
	{:else if setupError}
		<p class="error" role="alert"><Face state="error" /> {setupError}</p>
		<p class="muted"><a href="/register">retry</a></p>
	{:else}
		<header class="auth-head">
			<a class="auth-back" href="/register">
				<span class="auth-back__glyph" aria-hidden="true">←</span> back
			</a>
			<p class="microlabel auth-step">step 2 of 2 · enable 2FA</p>
		</header>
		<h1 class="page-title">set up 2FA</h1>
		<p class="page-sub">Scan the QR code with your authenticator app, or copy the secret.</p>
		{#if qrDataUrl}
			<div class="qr-wrap">
				<img class="qr-wrap__img" src={qrDataUrl} alt="TOTP QR code" width="200" height="200" />
			</div>
		{/if}
		{#if secret}
			<div class="secret">
				<p class="microlabel secret__label">setup key</p>
				<div class="secret__row">
					<code class="secret__text num">{secret}</code>
					<button
						type="button"
						class="secret__copy"
						class:secret__copy--done={copied}
						on:click={copySecret}
						aria-label="copy secret to clipboard"
					>
						<span aria-hidden="true">{copied ? '✓' : '⧉'}</span>
						{copied ? 'copied' : 'copy'}
					</button>
				</div>
			</div>
		{/if}
		<form method="post" action="/register/totp" on:submit={handleSubmit} class="auth-form">
			<!-- Hidden username keeps password managers oriented in the multi-step flow -->
			<input
				type="text"
				name="username"
				autocomplete="username"
				value={regUsername}
				readonly
				hidden
				tabindex="-1"
				aria-hidden="true"
			/>
			<div class="form-row">
				<label class="field-label" for="totp">authenticator code</label>
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
				{loading ? 'verifying…' : 'verify and register'}
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
		padding: var(--space-xs) var(--space-sm);
		background: var(--inset);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--text-muted);
		font-size: var(--fs-xs);
		text-decoration: none;
	}
	.auth-back:hover {
		color: var(--text);
		border-color: var(--border-strong);
	}
	.auth-back__glyph {
		line-height: 1;
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
	.auth-input--code {
		letter-spacing: 0.25em;
	}

	/* Responsive QR: scales down on narrow viewports, never overflows. */
	.qr-wrap {
		display: flex;
		justify-content: center;
		margin: var(--space-lg) 0 var(--space-md);
	}
	.qr-wrap__img {
		display: block;
		width: 200px;
		max-width: 60vw;
		height: auto;
		aspect-ratio: 1 / 1;
		padding: var(--space-sm);
		border-radius: var(--radius-sm);
		border: 1px solid var(--border);
		background: #fff;
	}

	.secret {
		margin-bottom: var(--space-md);
	}
	.secret__label {
		margin: 0 0 var(--space-sm);
		color: var(--text-faint);
	}
	.secret__row {
		display: flex;
		align-items: stretch;
		gap: var(--space-sm);
		background: var(--inset);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		padding: var(--space-sm) var(--space-md);
	}
	.secret__text {
		flex: 1;
		min-width: 0;
		align-self: center;
		font-size: var(--fs-xs);
		word-break: break-all;
		color: var(--text);
	}
	.secret__copy {
		display: inline-flex;
		align-items: center;
		gap: var(--space-xs);
		flex-shrink: 0;
		height: auto;
		padding: var(--space-sm) var(--space-md);
		background: var(--surface);
		border: 1px solid var(--border-strong);
		border-radius: var(--radius-sm);
		color: var(--text);
		font-size: var(--fs-xs);
		font-weight: 600;
	}
	.secret__copy:hover:not(:disabled) {
		border-color: var(--accent);
		color: var(--accent);
	}
	.secret__copy--done {
		border-color: var(--ok);
		color: var(--ok);
	}

	@media (max-width: 640px) {
		.auth-back {
			min-height: var(--touch-min);
			padding: 0 var(--space-md);
		}
		.secret__copy {
			min-height: var(--touch-min);
		}
	}
</style>
