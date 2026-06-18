<script>
	// Host rail. Subscribes to the shared hosts store (read-only — the layout
	// owns start/stop) and renders one row per host with a live Face + status
	// dot. On mobile the same markup is positioned as an off-canvas drawer by
	// the parent layout; this component just renders the rail contents.
	import { createEventDispatcher } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { hosts, hostsStatus } from '$lib/hosts.js';
	import Face from '$lib/Face.svelte';
	import Skeleton from '$lib/Skeleton.svelte';

	export let drawerOpen = false; // mobile drawer visibility (parent-controlled)

	const dispatch = createEventDispatcher();

	$: activeId = $page.params.id ?? null;

	function formatFree(n) {
		if (n == null || n < 0) return '';
		const units = ['B', 'KB', 'MB', 'GB', 'TB'];
		let v = n;
		let i = 0;
		while (v >= 1024 && i < units.length - 1) {
			v /= 1024;
			i += 1;
		}
		return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]} free`;
	}

	function pickHost(id) {
		dispatch('navigate', { id });
		goto(`/daemons/${id}`);
	}

	function addHost() {
		dispatch('requestAddHost');
		goto('/dashboard');
	}

	function openPalette() {
		dispatch('openPalette');
	}
</script>

<nav class="sidebar" class:sidebar--open={drawerOpen} aria-label="hosts">
	<div class="sidebar__head">
		<span class="microlabel">hosts</span>
		<span class="sidebar__count num">{$hosts.length}</span>
	</div>

	<div class="sidebar__list">
		{#if $hostsStatus.loading && $hosts.length === 0}
			<div class="sidebar__loading">
				<Skeleton variant="host" count={3} />
			</div>
		{:else if $hosts.length === 0}
			<p class="sidebar__empty muted"><Face state="offline" /> no hosts yet</p>
		{:else}
			{#each $hosts as host (host.id)}
				<button
					type="button"
					class="host-row"
					class:host-row--active={String(host.id) === String(activeId)}
					on:click={() => pickHost(host.id)}
				>
					<span class="status-dot" class:on={host.connected}></span>
					<Face state={host.connected ? 'ok' : 'offline'} />
					<span class="host-row__body">
						<span class="host-row__label">{host.label}</span>
						{#if host.hosted_path}
							<span class="host-row__path">~/{host.hosted_path.replace(/^[~/]+/, '')}</span>
						{/if}
						<span class="host-row__meta">
							{#if host.connected}
								<span class="num">{formatFree(host.disk_free)}</span>
							{:else}
								offline
							{/if}
						</span>
					</span>
				</button>
			{/each}
		{/if}
	</div>

	<div class="sidebar__foot">
		<button type="button" class="sidebar__action" on:click={addHost}>
			<span class="sidebar__glyph" aria-hidden="true">+</span> add host
		</button>
		<button type="button" class="sidebar__action sidebar__action--kbd" on:click={openPalette}>
			<span class="sidebar__glyph" aria-hidden="true">⌘K</span> command…
		</button>
	</div>
</nav>

<style>
	.sidebar {
		display: flex;
		flex-direction: column;
		width: var(--sidebar-width);
		flex-shrink: 0;
		background: var(--rail-bg);
		border-right: 1px solid var(--border);
		padding: var(--space-md) 0 calc(var(--space-md) + var(--safe-bottom));
		position: sticky;
		top: var(--header-height);
		height: calc(100vh - var(--header-height));
		overflow-y: auto;
		z-index: var(--z-sidebar);
	}

	.sidebar__head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0 var(--space-md) var(--space-sm);
	}

	.sidebar__count {
		font-size: var(--fs-2xs);
		color: var(--text-faint);
	}

	.sidebar__list {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: 0 var(--space-sm);
	}

	.sidebar__loading,
	.sidebar__empty {
		padding: var(--space-sm);
	}

	.sidebar__empty {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.host-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		width: 100%;
		min-height: var(--rail-row-height);
		padding: var(--space-sm);
		border: none;
		border-left: 2px solid transparent;
		border-radius: var(--radius-sm);
		background: transparent;
		color: var(--text);
		text-align: left;
		cursor: pointer;
	}
	@media (prefers-reduced-motion: no-preference) {
		.host-row {
			transition:
				background var(--dur-instant) var(--ease-std),
				border-color var(--dur-instant) var(--ease-std);
		}
	}

	.host-row:hover {
		background: var(--row-hover);
	}

	.host-row--active {
		background: var(--rail-active);
		border-left-color: var(--accent);
	}

	.host-row__body {
		display: flex;
		flex-direction: column;
		gap: 1px;
		min-width: 0;
		flex: 1;
	}

	.host-row__label {
		font-weight: 600;
		font-size: var(--fs-sm);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.host-row__path,
	.host-row__meta {
		font-size: var(--fs-xs);
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sidebar__foot {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: var(--space-sm) var(--space-sm) 0;
		margin-top: var(--space-sm);
		border-top: 1px solid var(--border);
	}

	.sidebar__action {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		width: 100%;
		height: auto;
		min-height: var(--rail-row-height);
		padding: var(--space-sm);
		border: none;
		border-radius: var(--radius-sm);
		background: transparent;
		color: var(--text-muted);
		font-size: var(--fs-xs);
		text-align: left;
	}

	.sidebar__action:hover {
		background: var(--row-hover);
		color: var(--text);
		border-color: transparent;
	}

	.sidebar__glyph {
		flex-shrink: 0;
		min-width: 1.5rem;
		color: var(--text-faint);
	}

	@media (max-width: 640px) {
		.host-row,
		.sidebar__action {
			min-height: var(--touch-min);
		}
	}
</style>
