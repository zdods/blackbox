<script>
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { getToken, clearToken, apiFetch } from '$lib/auth.js';

  const agentId = $page.params.id;
  let path = '';
  let entries = [];
  let loading = true;
  let error = '';
  let agentLabel = '';
  let uploadPath = '';
  let uploading = false;
  let selectedFileName = '';
  let uploadProgress = { current: 0, total: 0 };
  let deletingPath = '';
  let sortBy = 'name'; // 'name' | 'size' | 'mtime'
  let sortDir = 'asc';  // 'asc' | 'desc'

  $: pathSegments = path ? path.split('/').filter(Boolean) : [];
  $: sortedEntries = (() => {
    const list = [...entries];
    list.sort((a, b) => {
      let cmp = 0;
      if (sortBy === 'name') {
        const an = (a.name || '').toLowerCase();
        const bn = (b.name || '').toLowerCase();
        cmp = an.localeCompare(bn, undefined, { sensitivity: 'base' });
        // directories first when sorting by name
        if (a.is_dir !== b.is_dir) cmp = a.is_dir ? -1 : 1;
      } else if (sortBy === 'size') {
        const as = a.is_dir ? -1 : (a.size ?? 0);
        const bs = b.is_dir ? -1 : (b.size ?? 0);
        cmp = as - bs;
      } else if (sortBy === 'mtime') {
        const at = a.mtime || '';
        const bt = b.mtime || '';
        cmp = at.localeCompare(bt);
      }
      return sortDir === 'asc' ? cmp : -cmp;
    });
    return list;
  })();

  onMount(() => {
    if (!getToken()) {
      goto('/login');
      return;
    }
    load();
  });

  async function load() {
    loading = true;
    error = '';
    try {
      if (!agentLabel) {
        const listRes = await apiFetch('/api/agents');
        if (listRes.ok) {
          const list = await listRes.json();
          const a = list.find((x) => x.id === agentId);
          if (a) agentLabel = a.label;
        }
      }
      const q = path ? `?path=${encodeURIComponent(path)}` : '';
      const res = await apiFetch(`/api/agents/${agentId}/files${q}`);
      if (res.status === 401) {
        clearToken();
        goto('/login');
        return;
      }
      if (res.status === 503) {
        error = 'blackbox agent not connected';
        entries = [];
        loading = false;
        return;
      }
      if (!res.ok) throw new Error(await res.text());
      entries = await res.json();
    } catch (e) {
      error = e.message;
      entries = [];
    } finally {
      loading = false;
    }
  }

  function goToSegment(segment) {
    const idx = pathSegments.indexOf(segment);
    path = pathSegments.slice(0, idx + 1).join('/');
    load();
  }

  function goUp() {
    if (pathSegments.length === 0) return;
    path = pathSegments.slice(0, -1).join('/');
    load();
  }

  async function download(entry) {
    const fullPath = path ? `${path}/${entry.name}` : entry.name;
    const url = `/api/agents/${agentId}/files?path=${encodeURIComponent(fullPath)}&download=1`;
    const res = await apiFetch(url);
    if (!res.ok) return;
    const blob = await res.blob();
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = entry.name;
    a.click();
    URL.revokeObjectURL(a.href);
  }

  async function handleUpload(e) {
    const files = e.target.files;
    if (!files?.length) return;
    const total = files.length;
    uploading = true;
    uploadProgress = { current: 0, total };
    error = '';
    try {
      for (let i = 0; i < total; i++) {
        uploadProgress = { current: i + 1, total };
        selectedFileName = total > 1 ? `Uploading ${i + 1} of ${total}…` : files[i].name;
        const file = files[i];
        const targetPath = uploadPath ? `${uploadPath}/${file.name}` : file.name;
        const res = await apiFetch(`/api/agents/${agentId}/files?path=${encodeURIComponent(targetPath)}`, {
          method: 'PUT',
          body: file
        });
        if (!res.ok) {
          const msg = await res.text();
          throw new Error(msg ? `${file.name}: ${msg}` : `Upload failed for ${file.name}`);
        }
      }
      uploadPath = '';
      selectedFileName = '';
      e.target.value = '';
      load();
    } catch (err) {
      error = err.message || 'Upload failed';
    } finally {
      uploading = false;
      uploadProgress = { current: 0, total: 0 };
      selectedFileName = '';
      e.target.value = '';
    }
  }

  function setSort(col) {
    if (sortBy === col) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
    else { sortBy = col; sortDir = 'asc'; }
  }

  function formatSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const units = ['B', 'KB', 'MB', 'GB'];
    let i = 0;
    let n = bytes;
    while (n >= k && i < units.length - 1) {
      n /= k;
      i += 1;
    }
    return (i === 0 ? n : n.toFixed(1)) + ' ' + units[i];
  }

  async function deleteEntry(entry) {
    const fullPath = path ? `${path}/${entry.name}` : entry.name;
    if (!confirm(`Delete ${entry.is_dir ? 'directory' : 'file'} "${entry.name}"?`)) return;
    deletingPath = fullPath;
    error = '';
    try {
      const res = await apiFetch(`/api/agents/${agentId}/files?path=${encodeURIComponent(fullPath)}`, {
        method: 'DELETE'
      });
      if (!res.ok) throw new Error(await res.text());
      load();
    } catch (err) {
      error = err.message;
    } finally {
      deletingPath = '';
    }
  }
</script>

<div class="container">
  <p class="term-muted"><a href="/dashboard">← dashboard</a></p>
  <h1 class="term-h1"><span class="kaomoji">[▪‿▪]</span>files {#if agentLabel}<span class="path-label">({agentLabel})</span>{/if}</h1>

  <div class="breadcrumb">
    <button type="button" class="link" on:click={() => { path = ''; load(); }}>root</button>
    {#each pathSegments as segment}
      <span class="breadcrumb-sep">/</span>
      <button type="button" class="link" on:click={() => goToSegment(segment)}>{segment}</button>
    {/each}
  </div>

  {#if error}<p class="error">{error}</p>{/if}

  {#if loading}
    <p class="term-muted">loading...</p>
  {:else}
    <div class="file-list-wrap">
      <table class="file-list">
        <thead>
          <tr>
            <th scope="col" class="col-name sortable" class:sort-asc={sortBy === 'name' && sortDir === 'asc'} class:sort-desc={sortBy === 'name' && sortDir === 'desc'}>
              <button type="button" class="th-sort" on:click={() => setSort('name')}>name</button>
            </th>
            <th scope="col" class="col-size sortable" class:sort-asc={sortBy === 'size' && sortDir === 'asc'} class:sort-desc={sortBy === 'size' && sortDir === 'desc'}>
              <button type="button" class="th-sort" on:click={() => setSort('size')}>size</button>
            </th>
            <th scope="col" class="col-mtime sortable" class:sort-asc={sortBy === 'mtime' && sortDir === 'asc'} class:sort-desc={sortBy === 'mtime' && sortDir === 'desc'}>
              <button type="button" class="th-sort" on:click={() => setSort('mtime')}>modified</button>
            </th>
            <th scope="col" class="col-actions"></th>
          </tr>
        </thead>
        <tbody>
        {#if pathSegments.length > 0}
          <tr>
            <td colspan="4"><button type="button" class="link" on:click={goUp}>..</button></td>
          </tr>
        {/if}
        {#each sortedEntries as entry}
          <tr>
            <td class="col-name">
              {#if entry.is_dir}
                <button type="button" class="link" on:click={() => { path = path ? `${path}/${entry.name}` : entry.name; load(); }}>{entry.name}/</button>
              {:else}
                <button class="link" on:click={() => download(entry)}>{entry.name}</button>
              {/if}
            </td>
            <td class="col-size">{entry.is_dir ? '—' : formatSize(entry.size)}</td>
            <td class="col-mtime">{entry.mtime || '—'}</td>
            <td class="col-actions">
              <button type="button" class="link delete-btn" on:click={() => deleteEntry(entry)} disabled={deletingPath !== ''} title="delete">delete</button>
            </td>
          </tr>
        {/each}
        </tbody>
      </table>
    </div>

    <div class="upload">
      <div class="upload-row">
        <span class="upload-label">upload</span>
        <input type="text" bind:value={uploadPath} placeholder="optional subpath" class="upload-path" />
        <label class="upload-file-wrap">
          <input type="file" multiple on:change={handleUpload} disabled={uploading} class="upload-file-input" />
          <span class="upload-file-text">
            {#if uploading && uploadProgress.total > 0}
              uploading {uploadProgress.current} of {uploadProgress.total}…
            {:else}
              {selectedFileName || 'choose files…'}
            {/if}
          </span>
        </label>
      </div>
    </div>
  {/if}
</div>

<style>
  .path-label {
    color: var(--term-text-muted);
    font-weight: 500;
  }
  .breadcrumb {
    margin: var(--space-md) 0 var(--space-lg);
    font-size: 0.9rem;
    color: var(--term-text-muted);
  }
  .breadcrumb-sep {
    margin: 0 var(--space-sm);
  }
  .file-list-wrap {
    overflow-y: auto;
    min-height: 120px;
    max-height: min(50vh, 400px);
    margin-bottom: var(--space-lg);
    padding: var(--space-md);
    border: 1px solid var(--term-border);
    border-radius: 4px;
  }
  .file-list {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.9rem;
    margin: 0;
  }
  .file-list thead tr {
    background: var(--term-surface);
  }
  .file-list th {
    position: sticky;
    top: 0;
    z-index: 2;
    background: var(--term-surface);
    padding: var(--space-sm) var(--space-md);
    border-bottom: 1px solid var(--term-border);
    font-weight: 600;
    font-size: 0.8rem;
    color: var(--term-text-muted);
    letter-spacing: 0.03em;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.3);
  }
  .file-list td {
    padding: var(--space-sm) var(--space-md);
    border-bottom: 1px solid var(--term-border);
    vertical-align: middle;
  }
  .col-name {
    min-width: 0;
    text-align: left;
  }
  .file-list th.col-name,
  .file-list td.col-name {
    text-align: left;
  }
  .col-size {
    width: 7rem;
    color: var(--term-text-muted);
    text-align: right;
  }
  .file-list th.col-size,
  .file-list td.col-size {
    text-align: right;
  }
  .col-mtime {
    width: 12rem;
    color: var(--term-text-muted);
    font-size: 0.85rem;
    text-align: right;
  }
  .file-list th.col-mtime,
  .file-list td.col-mtime {
    text-align: right;
  }
  .col-actions {
    width: 5rem;
    text-align: center;
  }
  .file-list th.col-actions,
  .file-list td.col-actions {
    text-align: center;
  }
  .file-list th.sortable .th-sort {
    background: none;
    border: none;
    color: inherit;
    font: inherit;
    cursor: pointer;
    padding: 0;
    width: 100%;
    text-align: inherit;
  }
  .file-list th.sortable .th-sort:hover {
    color: var(--term-cyan);
  }
  .file-list th.sort-asc .th-sort::after {
    content: ' ↑';
    opacity: 0.8;
  }
  .file-list th.sort-desc .th-sort::after {
    content: ' ↓';
    opacity: 0.8;
  }
  .file-list td .link {
    display: inline-block;
  }
  .file-list .link {
    background: none;
    border: none;
    color: var(--term-cyan);
    cursor: pointer;
    padding: var(--space-sm) var(--space-md);
    text-align: left;
    font-family: var(--font-mono);
    font-size: inherit;
  }
  .file-list .link:hover {
    color: var(--term-green);
    text-decoration: underline;
  }
  .file-list .delete-btn {
    color: var(--term-red);
    font-size: 0.85rem;
  }
  .file-list .delete-btn:hover {
    color: var(--term-red);
    text-decoration: underline;
  }
  .upload {
    margin-top: var(--space-xl);
    padding-top: var(--space-lg);
    border-top: 1px solid var(--term-border);
    width: 100%;
  }
  .upload-row {
    display: flex;
    align-items: stretch;
    gap: var(--space-md);
    width: 100%;
  }
  .upload-label {
    display: flex;
    align-items: center;
    font-size: 0.9rem;
    color: var(--term-text-muted);
    flex-shrink: 0;
  }
  .upload-path {
    flex: 1;
    min-width: 0;
  }
  .upload-file-wrap {
    display: flex;
    align-items: center;
    flex: 0 0 auto;
    min-width: 12rem;
    min-height: var(--input-height);
    height: var(--input-height);
    background: var(--term-bg);
    border: 1px solid var(--term-border);
    border-radius: 4px;
    cursor: pointer;
    position: relative;
    box-sizing: border-box;
  }
  .upload-file-wrap:hover {
    border-color: var(--term-green);
  }
  .upload-file-input {
    position: absolute;
    inset: 0;
    opacity: 0;
    cursor: pointer;
    width: 100%;
    height: 100%;
    margin: 0;
    padding: 0;
  }
  .upload-file-text {
    padding: 0 var(--space-md);
    font-size: 13px;
    color: var(--term-text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    pointer-events: none;
  }
  .term-muted {
    color: var(--term-text-muted);
    font-size: 0.9rem;
    margin-bottom: var(--space-md);
  }
</style>
