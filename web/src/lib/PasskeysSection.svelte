<script>
	import { onMount } from 'svelte';
	import { apiFetch, redirectIfUnauthorized } from '$lib/auth.js';
	import { passkeyEnroll, browserSupportsWebAuthn } from '$lib/passkey.js';
	import { confirmDelete } from '$lib/ConfirmDialog.svelte';
	import { formatDate } from '$lib/format.js';
	import Face from '$lib/Face.svelte';

	let passkeys = [];
	let loading = true;
	let error = '';
	let adding = false;
	let removingId = null;
	let newName = '';
	let supported = true;

	async function load() {
		error = '';
		try {
			const res = await apiFetch('/api/passkeys');
			if (redirectIfUnauthorized(res)) return;
			if (!res.ok) throw new Error(await res.text());
			passkeys = await res.json();
		} catch (e) {
			error = e?.message || 'Could not load passkeys';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		supported = browserSupportsWebAuthn();
		load();
	});

	async function add(e) {
		e?.preventDefault();
		error = '';
		adding = true;
		try {
			await passkeyEnroll(newName.trim() || 'Passkey');
			newName = '';
			await load();
		} catch (err) {
			error = err?.message || 'Could not add passkey';
		} finally {
			adding = false;
		}
	}

	// Exposed for the account screen's command-palette "Add a passkey" action,
	// which triggers the same enrollment ceremony as the in-card button. No-op
	// while a ceremony is already in flight or the browser lacks WebAuthn.
	export function enroll() {
		if (supported && !adding) add();
	}

	async function remove(passkey) {
		const ok = await confirmDelete(`passkey "${passkey.name || 'unnamed'}"`, { danger: true });
		if (!ok) return;
		removingId = passkey.id;
		error = '';
		try {
			const res = await apiFetch(`/api/passkeys/${passkey.id}`, { method: 'DELETE' });
			if (redirectIfUnauthorized(res)) return;
			if (res.status === 409) {
				const data = await res.json().catch(() => ({}));
				throw new Error(data.error || 'Cannot remove the last passkey');
			}
			if (!res.ok && res.status !== 204) throw new Error(await res.text());
			await load();
		} catch (err) {
			error = err?.message || 'Could not remove passkey';
		} finally {
			removingId = null;
		}
	}
</script>

<section class="passkeys">
	<h2 class="microlabel">passkeys</h2>
	<div class="card passkeys__card">
		{#if !supported}
			<p class="muted">This browser does not support passkeys.</p>
		{:else}
			{#if error}
				<p class="error" role="alert"><Face state="error" /> {error}</p>
			{/if}

			{#if loading}
				<p class="muted" role="status" aria-live="polite"><Face state="loading" /> loading…</p>
			{:else if passkeys.length === 0}
				<p class="muted">No passkeys yet. Add one so you can sign in without a password.</p>
			{:else}
				<ul class="passkeys__list">
					{#each passkeys as pk (pk.id)}
						<li class="passkeys__item">
							<div class="passkeys__meta">
								<span class="passkeys__name">{pk.name || 'unnamed passkey'}</span>
								<span class="passkeys__date num">added {formatDate(pk.created_at)}</span>
							</div>
							<button
								type="button"
								class="quiet danger"
								on:click={() => remove(pk)}
								disabled={removingId !== null}
								aria-label="remove {pk.name || 'passkey'}"
							>
								{removingId === pk.id ? 'removing…' : 'remove'}
							</button>
						</li>
					{/each}
				</ul>
			{/if}

			<form class="passkeys__add" on:submit={add}>
				<input
					type="text"
					name="passkey-name"
					autocomplete="off"
					bind:value={newName}
					placeholder="passkey name, e.g. MacBook"
					aria-label="new passkey name"
				/>
				<button type="submit" class="primary" disabled={adding}>
					{adding ? 'waiting for passkey…' : '+ add a passkey'}
				</button>
			</form>
		{/if}
	</div>
</section>

<style>
	.passkeys {
		margin-top: var(--space-2xl);
	}
	.passkeys__card {
		margin-top: var(--space-md);
		padding: var(--space-lg);
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}
	.passkeys__list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}
	.passkeys__item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-md);
		padding: var(--space-sm) var(--space-md);
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
	}
	.passkeys__meta {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
		min-width: 0;
	}
	.passkeys__name {
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.passkeys__date {
		font-size: var(--fs-xs);
		color: var(--text-muted);
	}
	.passkeys__add {
		display: flex;
		gap: var(--space-md);
	}
	.passkeys__add input {
		flex: 1;
		min-width: 0;
	}
	.passkeys__add button {
		flex-shrink: 0;
	}

	@media (max-width: 640px) {
		.passkeys__add {
			flex-direction: column;
		}
		.passkeys__add button {
			min-height: var(--touch-min);
		}
	}
</style>
