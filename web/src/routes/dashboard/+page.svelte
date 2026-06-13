<script>
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { getToken, clearToken, apiFetch } from '$lib/auth.js';
	import Face from '$lib/Face.svelte';

	const POLL_INTERVAL_MS = 8000; // refreshes daemon list and disk space

	let daemons = [];
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
			const res = await apiFetch('/api/daemons');
			if (res.status === 401) {
				clearToken();
				goto('/login');
				return;
			}
			if (!res.ok) throw new Error(await res.text());
			daemons = await res.json();
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	async function loadQuiet() {
		if (!getToken()) return;
		try {
			const res = await apiFetch('/api/daemons');
			if (res.status === 401) return;
			if (!res.ok) return;
			daemons = await res.json();
		} catch (e) {
			console.error('poll failed:', e);
		}
	}

	async function createDaemon(e) {
		e.preventDefault();
		if (!newLabel.trim()) return;
		creating = true;
		try {
			const res = await apiFetch('/api/daemons', {
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
				} catch (e) {
					console.error('clipboard write failed:', e);
					showToast('copy failed — save token: ' + data.token, 'error', 8000);
				}
			}
		} catch (err) {
			error = err.message;
		} finally {
			creating = false;
		}
	}

	async function saveRename(e, id) {
		e.preventDefault();
		if (!editLabel.trim()) return;
		try {
			const res = await apiFetch(`/api/daemons/${id}`, {
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

	async function deleteDaemon(daemon) {
		if (!confirm(`Delete host "${daemon.label}"? This cannot be undone.`)) return;
		deletingId = daemon.id;
		error = '';
		try {
			const res = await apiFetch(`/api/daemons/${daemon.id}`, { method: 'DELETE' });
			if (!res.ok) throw new Error(await res.text());
			await load();
		} catch (err) {
			error = err.message;
		} finally {
			deletingId = null;
		}
	}
</script>

<header class="page-header">
	<div>
		<h1 class="page-title">hosts</h1>
		<p class="page-sub">Machines running the blackhaul daemon.</p>
	</div>
</header>

{#if loading}
	<p class="muted" role="status" aria-live="polite"><Face state="loading" /> loading…</p>
{:else if error}
	<p class="error" role="alert"><Face state="error" /> {error}</p>
{:else}
	<ul class="host-list">
		{#each daemons as daemon}
			<li class="card host-card">
				{#if editingId === daemon.id}
					<form class="host-rename-form" on:submit={(e) => saveRename(e, daemon.id)}>
						<input
							type="text"
							name="host-rename"
							autocomplete="off"
							bind:value={editLabel}
							class="host-rename-input"
							aria-label="new name for {daemon.label}"
						/>
						<button type="submit" class="primary" disabled={!editLabel.trim()}>save</button>
						<button
							type="button"
							class="secondary"
							on:click={() => {
								editingId = null;
								editLabel = '';
							}}>cancel</button
						>
					</form>
				{:else}
					<span class="status-dot" class:on={daemon.connected} aria-hidden="true"></span>
					<div class="host-card__id">
						<a class="host-card__label" href="/daemons/{daemon.id}">{daemon.label}</a>
						<span class="host-card__meta">
							{#if daemon.connected}
								connected{#if daemon.disk_free != null}{' · '}<span
										title={daemon.disk_total != null
											? formatBytes(daemon.disk_free) + ' free of ' + formatBytes(daemon.disk_total)
											: ''}>{formatBytes(daemon.disk_free)} free</span
									>{/if}
							{:else}
								<Face state="offline" /> offline
							{/if}
						</span>
					</div>
					<div class="host-card__actions">
						<button
							type="button"
							class="quiet"
							on:click={() => {
								editingId = daemon.id;
								editLabel = daemon.label;
							}}
							aria-label="rename {daemon.label}">rename</button
						>
						<button
							type="button"
							class="quiet danger"
							on:click={() => deleteDaemon(daemon)}
							disabled={deletingId !== null}
							aria-label="delete {daemon.label}">delete</button
						>
					</div>
				{/if}
			</li>
		{/each}
	</ul>
	{#if daemons.length === 0}
		<div class="card empty-state">
			<p class="muted"><Face state="offline" /> no hosts yet — add one below.</p>
		</div>
	{/if}

	<section class="add-host">
		<h2 class="microlabel">add host</h2>
		<form on:submit={createDaemon} class="add-host-form card">
			<div class="add-host-row">
				<input
					id="daemon-label"
					name="host-label"
					type="text"
					autocomplete="off"
					bind:value={newLabel}
					placeholder="label, e.g. my-mac"
					aria-label="host label"
				/>
				<button type="submit" class="primary" disabled={creating || !newLabel.trim()}
					>{creating ? 'adding…' : 'add host'}</button
				>
			</div>
			<p class="muted add-host-hint">
				The daemon token is copied to your clipboard when the host is created.
			</p>
		</form>
	</section>
{/if}

{#if toast.show}
	<div class="toast toast-{toast.type}" role="status" aria-live="polite">
		{toast.message}
	</div>
{/if}

<style>
	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		margin-bottom: var(--space-xl);
	}
	.host-list {
		list-style: none;
		padding: 0;
		margin: 0 0 var(--space-xl);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}
	.host-card {
		display: flex;
		align-items: center;
		gap: var(--space-md);
		padding: var(--space-md) var(--space-lg);
	}
	.host-card__id {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}
	.host-card__label {
		font-weight: 600;
		font-size: 0.95rem;
		text-decoration: none;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.host-card__label:hover {
		color: var(--accent);
	}
	.host-card__meta {
		font-size: 0.8rem;
		color: var(--text-muted);
	}
	.host-card__actions {
		display: flex;
		gap: var(--space-xs);
		flex-shrink: 0;
	}
	.host-rename-form {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		flex: 1;
		min-width: 0;
		flex-wrap: wrap;
	}
	.host-rename-input {
		flex: 1;
		min-width: 8rem;
	}
	.empty-state {
		padding: var(--space-xl);
		text-align: center;
		margin-bottom: var(--space-xl);
		border-style: dashed;
	}
	.empty-state .muted {
		margin: 0;
	}
	.add-host {
		margin-top: var(--space-2xl);
	}
	.add-host-form {
		margin-top: var(--space-md);
		padding: var(--space-lg);
	}
	.add-host-row {
		display: flex;
		gap: var(--space-md);
	}
	.add-host-row input {
		flex: 1;
		min-width: 0;
	}
	.add-host-row button {
		flex-shrink: 0;
	}
	.add-host-hint {
		margin: var(--space-md) 0 0;
		font-size: 0.8rem;
	}

	@media (max-width: 640px) {
		.host-card {
			padding: var(--space-md);
			flex-wrap: wrap;
		}
		.host-card__actions {
			width: 100%;
			justify-content: flex-end;
			gap: var(--space-sm);
		}
		/* Comfortable touch targets for the host actions on mobile. */
		.host-card__actions .quiet {
			min-height: 2.25rem;
			padding: var(--space-sm) var(--space-sm);
		}
		.add-host-row {
			flex-direction: column;
		}
	}
</style>
