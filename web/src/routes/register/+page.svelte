<script>
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { getToken } from '$lib/auth.js';
  import { setRegisterCredentials, clearRegisterCredentials } from '$lib/register-state.js';

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

<div class="container login-container">
  {#if setupLoading}
    <p class="term-muted">loading...</p>
  {:else}
    <h1 class="term-h1"><span class="kaomoji">[▪‿▪]</span> register</h1>
    <form on:submit={handleContinue} class="term-form">
      <div class="form-row">
        <label for="username"><span class="prompt-prefix">$</span> username</label>
        <input id="username" name="username" type="text" autocomplete="username" bind:value={username} placeholder="pick-a-username" required />
      </div>
      <div class="form-row">
        <label for="password"><span class="prompt-prefix">$</span> password</label>
        <input id="password" name="password" type="password" autocomplete="new-password" bind:value={password} placeholder="••••••••" required />
      </div>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
      <button type="submit" class="primary" disabled={loading || !username.trim() || !password}>
        {loading ? '(´・ω・`) ...' : 'continue'}
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
</style>
