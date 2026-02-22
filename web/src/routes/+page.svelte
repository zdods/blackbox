<script>
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { getToken } from '$lib/auth.js';

  onMount(async () => {
    if (getToken()) {
      goto('/dashboard');
      return;
    }
    try {
      const res = await fetch('/api/setup');
      if (res.ok) {
        const data = await res.json();
        if (data.registration_open === true) {
          goto('/register');
          return;
        }
      }
    } catch (_) {}
    goto('/login');
  });
</script>

<div class="container"></div>
