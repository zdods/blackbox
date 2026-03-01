<script>
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { getToken } from '$lib/auth.js';
  import QRCode from 'qrcode';

  let username = '';
  let password = '';
  let totpCode = '';
  let error = '';
  let loading = false;
  let setupLoading = true;
  let registrationOpen = false;
  let setupId = '';
  let secret = '';
  let qrDataUrl = '';
  let setupError = '';

  $: if (typeof window !== 'undefined' && getToken()) {
    goto('/dashboard');
  }

  onMount(async () => {
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
    } catch (_) {}
  }

  async function handleSubmit(e) {
    e.preventDefault();
    error = '';
    loading = true;
    try {
      const res = await fetch('/api/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          username: username.trim(),
          password,
          totp_code: totpCode.replace(/\s/g, ''),
          setup_id: setupId
        })
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        error = data.error || res.statusText || 'Registration failed';
        if (res.status === 403) {
          goto('/login?registration=closed', { replaceState: true });
          return;
        }
        return;
      }
      goto('/login');
    } finally {
      loading = false;
    }
  }
</script>

<div class="container login-container">
  {#if setupLoading}
    <p class="term-muted">loading...</p>
  {:else if setupError}
    <p class="error">{setupError}</p>
    <p class="term-muted"><a href="/register">retry</a></p>
  {:else}
    <h1 class="term-h1"><span class="kaomoji">[▪‿▪]</span> register</h1>
    <p class="term-muted">Scan the QR code with your authenticator app, or copy the secret.</p>
    {#if qrDataUrl}
      <div class="qr-wrap">
        <img src={qrDataUrl} alt="TOTP QR code" width="200" height="200" />
      </div>
    {/if}
    {#if secret}
      <div class="secret-row">
        <code class="secret-text">{secret}</code>
        <button type="button" class="link-button" on:click={copySecret}>Copy</button>
      </div>
    {/if}
    <form on:submit={handleSubmit} class="term-form">
      <div class="form-row">
        <label for="username"><span class="prompt-prefix">$</span> username</label>
        <input id="username" type="text" bind:value={username} placeholder="pick-a-username" required />
      </div>
      <div class="form-row">
        <label for="password"><span class="prompt-prefix">$</span> password</label>
        <input id="password" type="password" bind:value={password} placeholder="••••••••" required />
      </div>
      <div class="form-row">
        <label for="totp"><span class="prompt-prefix">$</span> authenticator code</label>
        <input
          id="totp"
          type="text"
          inputmode="numeric"
          autocomplete="one-time-code"
          bind:value={totpCode}
          placeholder="000000"
          maxlength="8"
        />
      </div>
      {#if error}<p class="error">{error}</p>{/if}
      <button type="submit" class="primary" disabled={loading || !username.trim() || !password || !totpCode.trim()}>
        {loading ? '(´・ω・`) ...' : 'register'}
      </button>
    </form>
  {/if}
</div>

<style>
  .login-container {
    width: fit-content;
    max-width: 100%;
  }
  .term-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-lg);
    width: 22rem;
  }
  .form-row label {
    display: block;
    font-size: 0.85rem;
    color: var(--term-text-muted);
    margin-bottom: var(--space-sm);
  }
  .term-muted {
    margin-top: var(--space-xl);
    font-size: 0.85rem;
    color: var(--term-text-muted);
  }
  .qr-wrap {
    margin: var(--space-md) 0;
  }
  .qr-wrap img {
    display: block;
  }
  .secret-row {
    display: flex;
    align-items: center;
    gap: var(--space-sm);
    margin-bottom: var(--space-md);
  }
  .secret-text {
    font-size: 0.85rem;
    word-break: break-all;
    color: var(--term-text-muted);
  }
</style>
