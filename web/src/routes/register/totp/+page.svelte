<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { getToken } from '$lib/auth.js';
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

	$: if (typeof window !== 'undefined' && getToken()) {
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
	});

	async function copySecret() {
		if (!secret) return;
		try {
			await navigator.clipboard.writeText(secret);
		} catch (e) {
			console.error('clipboard write failed:', e);
		}
	}

	async function handleSubmit(e) {
		e.preventDefault();
		error = '';
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
		<h1 class="page-title">set up 2FA</h1>
		<p class="page-sub">Scan the QR code with your authenticator app, or copy the secret.</p>
		{#if qrDataUrl}
			<div class="qr-wrap">
				<img src={qrDataUrl} alt="TOTP QR code" width="200" height="200" />
			</div>
		{/if}
		{#if secret}
			<div class="secret-row">
				<code class="secret-text">{secret}</code>
				<button
					type="button"
					class="quiet"
					on:click={copySecret}
					aria-label="copy secret to clipboard">copy</button
				>
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
					bind:value={totpCode}
					placeholder="000000"
					maxlength="8"
				/>
			</div>
			{#if error}<p class="error" role="alert"><Face state="error" /> {error}</p>{/if}
			<button type="submit" class="primary" disabled={loading || !totpCode.trim()}>
				{loading ? 'verifying…' : 'verify and register'}
			</button>
			<a href="/register" class="back-link">back</a>
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
	.qr-wrap {
		margin: var(--space-lg) 0 var(--space-md);
	}
	.qr-wrap img {
		display: block;
		border-radius: var(--radius-sm);
		border: 1px solid var(--border);
		background: #fff;
	}
	.secret-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		margin-bottom: var(--space-md);
	}
	.secret-text {
		font-size: 0.8rem;
		word-break: break-all;
		color: var(--text-muted);
	}
	.back-link {
		font-size: 0.8rem;
		color: var(--text-muted);
		text-decoration: none;
		align-self: flex-start;
	}
	.back-link:hover {
		color: var(--accent);
	}
</style>
