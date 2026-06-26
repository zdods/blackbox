<script>
	import { onMount, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { isLoggedIn, apiFetch, redirectIfUnauthorized } from '$lib/auth.js';
	import { hosts, hostsStatus, refresh } from '$lib/hosts.js';
	import { registerActions } from '$lib/palette-actions.js';
	import { confirmDelete } from '$lib/ConfirmDialog.svelte';
	import { formatBytes } from '$lib/format.js';
	import { showToast } from '$lib/toast.js';
	import Face from '$lib/Face.svelte';
	import Skeleton from '$lib/Skeleton.svelte';

	// Host roster + live status come from the shared store (the layout owns the
	// 8s poll on this route — no local setInterval here, else we'd double-poll).
	// We only own the mutating actions (create / rename / delete) and a forced
	// refresh() afterward so the UI updates without waiting for the next tick.
	let error = '';
	let creating = false;
	let newLabel = '';
	let editingId = null;
	let editLabel = '';
	let deletingId = null;

	let labelInput; // add-host field, focused by the palette action / sidebar "+ add host"
	let renameInput;

	$: loading = $hostsStatus.loading && $hosts.length === 0;
	$: storeError = $hostsStatus.error;

	// Overview metrics — what makes this the management surface rather than a
	// redundant copy of the sidebar list.
	$: total = $hosts.length;
	$: onlineCount = $hosts.filter((h) => h.connected).length;
	$: offlineCount = total - onlineCount;
	$: aggFree = $hosts.reduce((s, h) => s + (h.disk_free > 0 ? h.disk_free : 0), 0);
	$: aggTotal = $hosts.reduce((s, h) => s + (h.disk_total > 0 ? h.disk_total : 0), 0);

	onMount(() => {
		if (!isLoggedIn()) {
			goto('/login');
			return;
		}
		// Register palette actions for this screen; unregister on destroy.
		const off = registerActions([
			{ id: 'dash-add-host', label: 'Add host…', run: focusAddHost },
			{ id: 'dash-refresh', label: 'Refresh hosts', run: () => refresh() }
		]);
		return off;
	});

	async function focusAddHost() {
		await tick();
		if (labelInput) {
			labelInput.focus();
			labelInput.scrollIntoView({ block: 'center' });
		}
	}

	async function createDaemon(e) {
		e.preventDefault();
		if (!newLabel.trim()) return;
		creating = true;
		error = '';
		try {
			const res = await apiFetch('/api/daemons', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ label: newLabel.trim() })
			});
			if (redirectIfUnauthorized(res)) return;
			if (!res.ok) throw new Error(await res.text());
			const data = await res.json();
			newLabel = '';
			await refresh();
			if (data.token) {
				try {
					await navigator.clipboard.writeText(data.token);
					showToast('token copied to clipboard', 'success', 3000);
				} catch (err) {
					console.error('clipboard write failed:', err);
					showToast('copy failed — save token: ' + data.token, 'error', 8000);
				}
			}
		} catch (err) {
			error = err.message;
		} finally {
			creating = false;
		}
	}

	function startRename(daemon) {
		editingId = daemon.id;
		editLabel = daemon.label;
		tick().then(() => {
			if (renameInput) {
				renameInput.focus();
				renameInput.select();
			}
		});
	}

	function cancelRename() {
		editingId = null;
		editLabel = '';
	}

	async function saveRename(e, id) {
		e.preventDefault();
		if (!editLabel.trim()) return;
		error = '';
		try {
			const res = await apiFetch(`/api/daemons/${id}`, {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ label: editLabel.trim() })
			});
			if (redirectIfUnauthorized(res)) return;
			if (!res.ok) throw new Error(await res.text());
			editingId = null;
			editLabel = '';
			await refresh();
		} catch (err) {
			error = err.message;
		}
	}

	// Fraction of disk used (0–1), or null when totals are unknown.
	function usedFraction(daemon) {
		if (daemon.disk_total == null || daemon.disk_total <= 0) return null;
		if (daemon.disk_free == null || daemon.disk_free < 0) return null;
		const used = daemon.disk_total - daemon.disk_free;
		return Math.min(1, Math.max(0, used / daemon.disk_total));
	}

	function usedPct(daemon) {
		const f = usedFraction(daemon);
		return f == null ? null : Math.round(f * 100);
	}

	async function deleteDaemon(daemon) {
		// Themed promise-based dialog instead of native confirm().
		const ok = await confirmDelete(`host "${daemon.label}"`, { danger: true });
		if (!ok) return;
		deletingId = daemon.id;
		error = '';
		try {
			const res = await apiFetch(`/api/daemons/${daemon.id}`, { method: 'DELETE' });
			if (redirectIfUnauthorized(res)) return;
			if (!res.ok) throw new Error(await res.text());
			await refresh();
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
	<button type="button" class="primary page-header__cta" on:click={focusAddHost}>
		<span aria-hidden="true">+</span> add host
	</button>
</header>

{#if loading}
	<p class="sr-status muted" role="status" aria-live="polite">
		<Face state="loading" /> loading hosts…
	</p>
	<ul class="host-list" aria-hidden="true">
		<li class="card host-card host-card--skeleton">
			<Skeleton variant="line" count={2} />
		</li>
		<li class="card host-card host-card--skeleton">
			<Skeleton variant="line" count={2} />
		</li>
	</ul>
{:else}
	{#if error || storeError}
		<p class="error" role="alert"><Face state="error" /> {error || storeError}</p>
	{/if}

	{#if total > 0}
		<!-- Overview strip: the at-a-glance roster summary that makes the dashboard
		     a management surface rather than a copy of the sidebar list. -->
		<section class="overview" aria-label="fleet overview">
			<div class="overview__stat">
				<span class="overview__num num">{total}</span>
				<span class="overview__label microlabel">host{total === 1 ? '' : 's'}</span>
			</div>
			<div class="overview__stat">
				<span class="overview__num num overview__num--ok">{onlineCount}</span>
				<span class="overview__label microlabel">online</span>
			</div>
			<div class="overview__stat">
				<span class="overview__num num overview__num--off">{offlineCount}</span>
				<span class="overview__label microlabel">offline</span>
			</div>
			{#if aggTotal > 0}
				<div class="overview__stat overview__stat--disk">
					<span class="overview__num num">{formatBytes(aggFree)}</span>
					<span class="overview__label microlabel">free of {formatBytes(aggTotal)}</span>
				</div>
			{/if}
		</section>

		<ul class="host-list">
			{#each $hosts as daemon (daemon.id)}
				<li class="card host-card" class:host-card--editing={editingId === daemon.id}>
					{#if editingId === daemon.id}
						<form class="host-rename-form" on:submit={(e) => saveRename(e, daemon.id)}>
							<input
								type="text"
								name="host-rename"
								autocomplete="off"
								bind:value={editLabel}
								bind:this={renameInput}
								class="host-rename-input"
								aria-label="new name for {daemon.label}"
								on:keydown={(e) => e.key === 'Escape' && cancelRename()}
							/>
							<div class="host-rename-actions">
								<button type="submit" class="primary" disabled={!editLabel.trim()}>save</button>
								<button type="button" class="secondary" on:click={cancelRename}>cancel</button>
							</div>
						</form>
					{:else}
						<div class="host-card__main">
							<span class="status-dot" class:on={daemon.connected} aria-hidden="true"></span>
							<Face state={daemon.connected ? 'ok' : 'offline'} />
							<div class="host-card__id">
								<a class="host-card__label" href="/daemons/{daemon.id}">{daemon.label}</a>
								{#if daemon.hosted_path}
									<span class="host-card__path num"
										>~/{daemon.hosted_path.replace(/^[~/]+/, '')}</span
									>
								{/if}
							</div>
							<div class="host-card__actions">
								<button
									type="button"
									class="quiet"
									on:click={() => startRename(daemon)}
									aria-label="rename {daemon.label}">rename</button
								>
								<button
									type="button"
									class="quiet danger"
									on:click={() => deleteDaemon(daemon)}
									disabled={deletingId !== null}
									aria-label="delete {daemon.label}"
									>{deletingId === daemon.id ? 'deleting…' : 'delete'}</button
								>
							</div>
						</div>

						<div class="host-card__status">
							{#if daemon.connected}
								<span class="badge">connected</span>
							{:else}
								<span class="badge off"><Face state="offline" /> offline</span>
							{/if}

							{#if usedFraction(daemon) != null}
								<div
									class="disk"
									title="{formatBytes(daemon.disk_free)} free of {formatBytes(daemon.disk_total)}"
								>
									<div
										class="disk__bar"
										role="progressbar"
										aria-valuemin="0"
										aria-valuemax="100"
										aria-valuenow={usedPct(daemon)}
										aria-label="disk usage for {daemon.label}"
									>
										<span
											class="disk__fill"
											class:disk__fill--high={usedFraction(daemon) >= 0.9}
											class:disk__fill--warn={usedFraction(daemon) >= 0.75 &&
												usedFraction(daemon) < 0.9}
											style="width: {usedPct(daemon)}%"
										></span>
									</div>
									<span class="disk__meta num"
										>{formatBytes(daemon.disk_free)} free · {usedPct(daemon)}% used</span
									>
								</div>
							{:else if daemon.connected}
								<span class="disk__meta muted">disk usage unavailable</span>
							{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	{:else}
		<!-- Strong empty state with onboarding guidance. -->
		<div class="card empty-state">
			<p class="empty-state__face"><Face state="offline" /></p>
			<h2 class="empty-state__title">no hosts yet</h2>
			<p class="empty-state__body muted">
				A <strong>host</strong> is a machine running the blackhaul daemon. Add one below to get a connection
				token, then run the daemon on that machine to bring it online.
			</p>
			<ol class="empty-state__steps">
				<li>Give the host a label and click <strong>add host</strong>.</li>
				<li>The token is copied to your clipboard.</li>
				<li>Run the daemon with that token — it appears here and in the sidebar once connected.</li>
			</ol>
			<button type="button" class="primary" on:click={focusAddHost}>
				<span aria-hidden="true">+</span> add your first host
			</button>
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
					bind:this={labelInput}
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

<style>
	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: var(--space-md);
		margin-bottom: var(--space-xl);
	}
	.page-header__cta {
		flex-shrink: 0;
	}

	.sr-status {
		margin: 0 0 var(--space-md);
	}

	/* --- Overview strip ------------------------------------------------ */
	.overview {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-sm);
		margin-bottom: var(--space-lg);
	}
	.overview__stat {
		flex: 1 1 7rem;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		padding: var(--space-md);
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
	}
	.overview__stat--disk {
		flex: 2 1 12rem;
	}
	.overview__num {
		font-size: var(--fs-lg);
		font-weight: 600;
		line-height: var(--lh-tight);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.overview__num--ok {
		color: var(--ok);
	}
	.overview__num--off {
		color: var(--text-faint);
	}
	.overview__label {
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* --- Host cards ---------------------------------------------------- */
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
		flex-direction: column;
		gap: var(--space-sm);
		padding: var(--space-md) var(--space-lg);
	}
	.host-card--skeleton {
		gap: var(--space-xs);
	}
	.host-card__main {
		display: flex;
		align-items: center;
		gap: var(--space-md);
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
		font-size: var(--fs-md);
		text-decoration: none;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.host-card__label:hover {
		color: var(--accent);
	}
	.host-card__path {
		font-size: var(--fs-xs);
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.host-card__actions {
		display: flex;
		gap: var(--space-xs);
		flex-shrink: 0;
	}

	.host-card__status {
		display: flex;
		align-items: center;
		gap: var(--space-md);
		padding-left: calc(8px + var(--space-md) + 1ch);
		flex-wrap: wrap;
	}

	/* --- Disk usage bar ------------------------------------------------ */
	.disk {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		flex: 1;
		min-width: 12rem;
	}
	.disk__bar {
		flex: 1;
		min-width: 5rem;
		height: 0.5rem;
		background: var(--inset);
		border: 1px solid var(--border);
		border-radius: var(--radius-pill);
		overflow: hidden;
	}
	.disk__fill {
		display: block;
		height: 100%;
		background: var(--ok);
		border-radius: var(--radius-pill);
	}
	.disk__fill--warn {
		background: var(--warn);
	}
	.disk__fill--high {
		background: var(--err);
	}
	@media (prefers-reduced-motion: no-preference) {
		.disk__fill {
			transition:
				width var(--dur-base) var(--ease-out),
				background var(--dur-base) var(--ease-std);
		}
	}
	.disk__meta {
		font-size: var(--fs-xs);
		color: var(--text-muted);
		flex-shrink: 0;
	}

	/* --- Inline rename ------------------------------------------------- */
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
	.host-rename-actions {
		display: flex;
		gap: var(--space-sm);
		flex-shrink: 0;
	}

	/* --- Empty state --------------------------------------------------- */
	.empty-state {
		padding: var(--space-2xl) var(--space-xl);
		text-align: center;
		margin-bottom: var(--space-xl);
		border-style: dashed;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-md);
	}
	.empty-state__face {
		margin: 0;
		font-size: var(--fs-xl);
	}
	.empty-state__title {
		margin: 0;
		font-size: var(--fs-lg);
		font-weight: 600;
	}
	.empty-state__body {
		margin: 0;
		max-width: 34rem;
		line-height: var(--lh-body);
	}
	.empty-state__steps {
		text-align: left;
		margin: 0;
		padding-left: 1.25rem;
		max-width: 34rem;
		color: var(--text-muted);
		font-size: var(--fs-sm);
		line-height: var(--lh-body);
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	/* --- Add host ------------------------------------------------------ */
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
		font-size: var(--fs-xs);
	}

	@media (max-width: 640px) {
		.page-header {
			flex-wrap: wrap;
		}
		.page-header__cta {
			min-height: var(--touch-min);
		}
		/* Even 2x2 stat grid instead of the lopsided 2+1+1 the flex-wrap
		   produced at narrow widths. The disk tile spans nothing special — its
		   label just wraps rather than ellipsizing so the totals stay readable. */
		.overview {
			display: grid;
			grid-template-columns: 1fr 1fr;
			gap: var(--space-sm);
		}
		.overview__stat--disk .overview__label {
			white-space: normal;
		}
		.host-card {
			padding: var(--space-md);
		}
		.host-card__main {
			flex-wrap: wrap;
		}
		/* Left-align the trailing actions under the host name instead of letting
		   them float right against an empty gutter. */
		.host-card__actions {
			width: 100%;
			justify-content: flex-start;
			gap: var(--space-sm);
		}
		.host-card__actions .quiet {
			min-height: var(--touch-min);
			padding: var(--space-sm) var(--space-md);
		}
		.host-card__status {
			padding-left: 0;
		}
		/* The disk row must not hold a fixed floor on phones, or the
		   non-shrinking meta text pushes the card past the viewport. Let the
		   bar take the full row and the meta wrap beneath it. */
		.disk {
			min-width: 0;
			flex-wrap: wrap;
		}
		.disk__bar {
			flex-basis: 100%;
		}
		.disk__meta {
			flex-shrink: 1;
			min-width: 0;
		}
		.host-rename-actions .primary,
		.host-rename-actions .secondary {
			min-height: var(--touch-min);
		}
		.add-host-row {
			flex-direction: column;
		}
		.add-host-row button {
			min-height: var(--touch-min);
		}
	}
</style>
