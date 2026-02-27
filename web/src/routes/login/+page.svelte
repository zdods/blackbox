<script>
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { setToken, getToken } from '$lib/auth.js';

  let username = '';
  let password = '';
  let error = '';
  let loading = false;
  let registrationOpen = true;
  let setupLoading = true;
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
      if (!res.ok) {
        error = await res.text() || res.statusText || 'Login failed';
        return;
      }
      const data = await res.json();
      if (data.token) setToken(data.token);
      goto('/dashboard');
    } finally {
      loading = false;
    }
  }
</script>

<div class="container login-container">
  <h1 class="term-h1"><span class="kaomoji">[▪‿▪]</span> log in</h1>
  <form on:submit={handleSubmit} class="term-form">
    <div class="form-row">
      <label for="username"><span class="prompt-prefix">$</span> username</label>
      <input id="username" type="text" bind:value={username} placeholder="your-username" required />
    </div>
    <div class="form-row">
      <label for="password"><span class="prompt-prefix">$</span> password</label>
      <input id="password" type="password" bind:value={password} placeholder="••••••••" required />
    </div>
    {#if error}<p class="error">{error}</p>{/if}
    <button type="submit" class="primary" disabled={loading || !username.trim() || !password}>{loading ? '(´・ω・`) ...' : 'log in'}</button>
  </form>
  {#if !setupLoading && registrationOpen}
    <p class="term-muted"><a href="/register">register</a> (one-time setup)</p>
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
</style>
