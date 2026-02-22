<script>
  import { page } from '$app/stores';
  import '../app.css';

  $: narrowWindow = $page.url.pathname === '/login' || $page.url.pathname === '/register';
</script>

<div class="terminal" class:narrow={narrowWindow}>
  <header class="terminal-header">
    <span class="terminal-dot red"></span>
    <span class="terminal-dot yellow"></span>
    <span class="terminal-dot green"></span>
    <span class="terminal-title">blackbox-console</span>
  </header>
  <slot />
</div>
