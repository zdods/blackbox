<script>
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { getToken } from '$lib/auth.js';

  let username = '';
  let password = '';
  let error = '';
  let loading = false;
  let setupLoading = true;
  let registrationOpen = false;

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
    } catch (_) {
      goto('/login', { replaceState: true });
      return;
    }
    setupLoading = false;
  });

  async function handleSubmit(e) {
    e.preventDefault();
    error = '';
    loading = true;
    try {
      const res = await fetch('/api/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password })
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
  {:else}
    <h1 class="term-h1"><span class="kaomoji">[▪‿▪]</span> register</h1>
    <form on:submit={handleSubmit} class="term-form">
      <div class="form-row">
        <label for="username"><span class="prompt-prefix">$</span> username</label>
        <input id="username" type="text" bind:value={username} placeholder="pick-a-username" required />
      </div>
      <div class="form-row">
        <label for="password"><span class="prompt-prefix">$</span> password</label>
        <input id="password" type="password" bind:value={password} placeholder="••••••••" required />
      </div>
      {#if error}<p class="error">{error}</p>{/if}
      <button type="submit" class="primary" disabled={loading}>{loading ? '(´・ω・`) ...' : 'register'}</button>
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
</style>
