<script>
  import { onMount, onDestroy } from 'svelte';
  import { goto } from '$app/navigation';
  import { getToken, clearToken, apiFetch } from '$lib/auth.js';

  const POLL_INTERVAL_MS = 8000; // refreshes agent list and disk space

  let agents = [];
  let loading = true;
  let error = '';
  let creating = false;
  let newLabel = '';
  let editingId = null;
  let editLabel = '';
  let deletingId = null;
  let toast = { show: false, message: '', type: 'success' };
  let toastTimeout = null;
  let pollInterval = null;

  onMount(() => {
    if (!getToken()) {
      goto('/login');
      return;
    }
    load();
    pollInterval = setInterval(loadQuiet, POLL_INTERVAL_MS);
  });

  onDestroy(() => {
    if (pollInterval) clearInterval(pollInterval);
    if (toastTimeout) clearTimeout(toastTimeout);
  });

  function showToast(message, type = 'success', duration = 3000) {
    if (toastTimeout) clearTimeout(toastTimeout);
    toast = { show: true, message, type };
    toastTimeout = setTimeout(() => {
      toast = { ...toast, show: false };
      toastTimeout = null;
    }, duration);
  }

  async function load() {
    loading = true;
    error = '';
    try {
      const res = await apiFetch('/api/agents');
      if (res.status === 401) {
        clearToken();
        goto('/login');
        return;
      }
      if (!res.ok) throw new Error(await res.text());
      agents = await res.json();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function loadQuiet() {
    if (!getToken()) return;
    try {
      const res = await apiFetch('/api/agents');
      if (res.status === 401) return;
      if (!res.ok) return;
      agents = await res.json();
    } catch (_) {}
  }

  async function createAgent(e) {
    e.preventDefault();
    if (!newLabel.trim()) return;
    creating = true;
    try {
      const res = await apiFetch('/api/agents', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ label: newLabel.trim() })
      });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      newLabel = '';
      await load();
      if (data.token) {
        try {
          await navigator.clipboard.writeText(data.token);
          showToast('token copied to clipboard', 'success', 3000);
        } catch (_) {
          showToast('copy failed — save token: ' + data.token, 'error', 8000);
        }
      }
    } catch (err) {
      error = err.message;
    } finally {
      creating = false;
    }
  }

  function logout() {
    clearToken();
    goto('/login');
  }

  async function saveRename(e, id) {
    e.preventDefault();
    if (!editLabel.trim()) return;
    try {
      const res = await apiFetch(`/api/agents/${id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ label: editLabel.trim() })
      });
      if (!res.ok) throw new Error(await res.text());
      editingId = null;
      editLabel = '';
      await load();
    } catch (err) {
      error = err.message;
    }
  }

  function formatBytes(n) {
    if (n == null || n < 0) return '—';
    const k = 1024;
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    let v = n;
    while (v >= k && i < units.length - 1) {
      v /= k;
      i += 1;
    }
    return (i === 0 ? v : v.toFixed(1)) + ' ' + units[i];
  }

  async function deleteAgent(agent) {
    if (!confirm(`Delete agent "${agent.label}"? This cannot be undone.`)) return;
    deletingId = agent.id;
    error = '';
    try {
      const res = await apiFetch(`/api/agents/${agent.id}`, { method: 'DELETE' });
      if (!res.ok) throw new Error(await res.text());
      await load();
    } catch (err) {
      error = err.message;
    } finally {
      deletingId = null;
    }
  }
</script>

<div class="container">
  <header class="dashboard-header">
    <h1 class="term-h1"><span class="kaomoji">[▪‿▪]</span>agents</h1>
    <button class="secondary" on:click={logout}>log out</button>
  </header>

  {#if loading}
    <p class="term-muted">loading...</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else}
    <div class="agent-list-wrap">
      <ul class="agent-list">
        {#each agents as agent}
          <li>
            {#if editingId === agent.id}
              <form class="agent-rename-form" on:submit={(e) => saveRename(e, agent.id)}>
                <input type="text" bind:value={editLabel} class="agent-rename-input" />
                <button type="submit" class="primary">save</button>
                <button type="button" class="secondary" on:click={() => { editingId = null; editLabel = ''; }}>cancel</button>
              </form>
            {:else}
              <a href="/agents/{agent.id}">{agent.label}</a>
              {#if agent.connected}<span class="badge">connected</span>{:else}<span class="badge off">offline</span>{/if}
              {#if agent.disk_free != null}
                <span class="disk-free" title={agent.disk_total != null ? formatBytes(agent.disk_free) + ' free of ' + formatBytes(agent.disk_total) : ''}>
                  {formatBytes(agent.disk_free)} free
                </span>
              {/if}
              <button type="button" class="link-button" on:click={() => { editingId = agent.id; editLabel = agent.label; }} title="rename">rename</button>
              <button type="button" class="link-button delete-btn" on:click={() => deleteAgent(agent)} disabled={deletingId !== null} title="delete">delete</button>
            {/if}
          </li>
        {/each}
      </ul>
      {#if agents.length === 0}
        <p class="term-muted">no agents yet. add one below.</p>
      {/if}
    </div>

    <h2 class="term-h2">add agent</h2>
    <form on:submit={createAgent} class="term-form">
      <div class="form-row">
        <label for="agent-label"><span class="prompt-prefix">$</span> label</label>
        <input id="agent-label" type="text" bind:value={newLabel} placeholder="e.g. my-mac" />
      </div>
      <button type="submit" class="primary" disabled={creating}>{creating ? '(´・ω・`) ...' : 'add agent'}</button>
    </form>
  {/if}
</div>

{#if toast.show}
  <div class="toast toast-{toast.type}" role="status">
    {toast.message}
  </div>
{/if}

<style>
  .dashboard-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--space-lg);
  }
  .agent-list-wrap {
    overflow-y: auto;
    min-height: 120px;
    max-height: min(50vh, 400px);
    margin-bottom: var(--space-lg);
    padding: var(--space-md);
    border: 1px solid var(--term-border);
    border-radius: 4px;
  }
  .agent-list {
    list-style: none;
    padding: 0;
    margin: 0;
  }
  .agent-list li {
    padding: var(--space-md) 0;
    display: flex;
    align-items: center;
    gap: var(--space-md);
    border-bottom: 1px solid var(--term-border);
    flex-wrap: wrap;
  }
  .agent-list li:last-child {
    border-bottom: none;
  }
  .agent-list a {
    color: var(--term-cyan);
    flex: 1;
    min-width: 0;
  }
  .agent-list a:hover {
    color: var(--term-green);
  }
  .link-button {
    background: none;
    border: none;
    color: var(--term-text-muted);
    cursor: pointer;
    font-size: 0.85rem;
    padding: var(--space-sm) var(--space-md);
  }
  .link-button:hover {
    color: var(--term-cyan);
  }
  .link-button.delete-btn:hover {
    color: var(--term-red);
  }
  .agent-rename-form {
    display: flex;
    align-items: center;
    gap: var(--space-sm);
    flex: 1;
    min-width: 0;
  }
  .agent-rename-input {
    flex: 1;
    min-width: 8rem;
  }
  .badge {
    font-size: 0.7rem;
    padding: var(--space-sm) var(--space-md);
    background: rgba(126, 231, 135, 0.15);
    color: var(--term-green);
    border: 1px solid var(--term-green);
    border-radius: 4px;
  }
  .badge.off {
    background: rgba(110, 118, 129, 0.2);
    color: var(--term-text-muted);
    border-color: var(--term-text-muted);
  }
  .disk-free {
    font-size: 0.8rem;
    color: var(--term-text-muted);
    min-width: 5rem;
  }
  .term-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-lg);
    max-width: 100%;
    width: 100%;
    margin-top: var(--space-md);
  }
  .term-form button[type="submit"] {
    width: 100%;
  }
  .form-row {
    margin-bottom: var(--space-sm);
    width: 100%;
  }
  .form-row label {
    display: block;
    font-size: 0.85rem;
    color: var(--term-text-muted);
    margin-bottom: var(--space-sm);
  }
  .term-muted {
    color: var(--term-text-muted);
    font-size: 0.9rem;
    margin-top: var(--space-lg);
  }

  .toast {
    position: fixed;
    bottom: var(--space-xl);
    left: 50%;
    transform: translateX(-50%) translateY(0);
    padding: var(--space-md) var(--space-lg);
    border-radius: 8px;
    font-size: 0.9rem;
    box-shadow: 0 4px 20px rgba(0, 0, 0, 0.35);
    z-index: 100;
    animation: toast-in 0.25s ease-out;
    max-width: min(90vw, 28rem);
    text-align: center;
  }
  .toast-success {
    background: var(--term-surface);
    border: 1px solid var(--term-green);
    color: var(--term-text-bright);
  }
  .toast-error {
    background: var(--term-surface);
    border: 1px solid var(--term-red);
    color: var(--term-text-bright);
  }
  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translateX(-50%) translateY(10px);
    }
    to {
      opacity: 1;
      transform: translateX(-50%) translateY(0);
    }
  }
</style>
