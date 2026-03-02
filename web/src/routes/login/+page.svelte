<script>
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { setToken, getToken } from '$lib/auth.js';

  let username = '';
  let password = '';
  let totpCode = '';
  let error = '';
  let loading = false;
  let registrationOpen = true;
  let setupLoading = true;
  let loginToken = '';
  let step = 'password'; // 'password' | 'totp'

  $: if (typeof window !== 'undefined' && getToken()) {
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
    } catch (_) {}
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
      if (data.token) setToken(data.token);
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
      if (data.token) setToken(data.token);
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

<div class="container login-container">
  <h1 class="term-h1"><span class="kaomoji">[▪‿▪]</span> log in</h1>
  {#if step === 'totp'}
    <p class="term-muted">Enter the code from your authenticator app.</p>
    <form on:submit={handleTotpSubmit} class="term-form">
      <div class="form-row">
        <label for="totp"><span class="prompt-prefix">$</span> 2FA code</label>
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
      {#if error}<p class="error">{error}</p>{/if}
      <button type="submit" class="primary" disabled={loading || !totpCode.trim()}>
        {loading ? '(´・ω・`) ...' : 'verify'}
      </button>
      <button type="button" class="link-button" on:click={backToPassword}>back</button>
    </form>
  {:else}
    <form method="post" action="/login" on:submit={handleSubmit} class="term-form">
      <div class="form-row">
        <label for="username"><span class="prompt-prefix">$</span> username</label>
        <input id="username" name="login" type="text" autocomplete="username" bind:value={username} placeholder="your-username" required />
      </div>
      <div class="form-row">
        <label for="password"><span class="prompt-prefix">$</span> password</label>
        <input id="password" name="password" type="password" autocomplete="current-password" bind:value={password} placeholder="••••••••" required />
      </div>
      {#if error}<p class="error">{error}</p>{/if}
      <button type="submit" class="primary" disabled={loading || !username.trim() || !password}>{loading ? '(´・ω・`) ...' : 'log in'}</button>
    </form>
    {#if !setupLoading && registrationOpen}
      <p class="term-muted"><a href="/register">register</a> (one-time setup)</p>
    {/if}
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
  .link-button {
    background: none;
    border: none;
    color: var(--term-text-muted);
    cursor: pointer;
    font-size: 0.85rem;
    padding: var(--space-sm) 0;
  }
  .link-button:hover {
    color: var(--term-cyan);
  }
</style>
