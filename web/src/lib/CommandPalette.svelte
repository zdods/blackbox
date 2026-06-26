<script>
	// ⌘K / Ctrl+K command palette. Mounted once in the layout, gated by the
	// parent on `authed && !centered`. Reuses the shared hosts store, the theme
	// store (setTheme), goto for navigation, and the route-contextual action
	// registry ($lib/palette-actions.js). Hand-rolled fuzzy filter — no deps.
	import { createEventDispatcher, tick } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { hosts, hostsStatus } from '$lib/hosts.js';
	import { setTheme, THEMES } from '$lib/theme.js';
	import { paletteActions } from '$lib/palette-actions.js';
	import Face from '$lib/Face.svelte';
	import Kbd from '$lib/Kbd.svelte';

	export let open = false;

	const dispatch = createEventDispatcher();

	let query = '';
	let highlight = 0;
	let inputEl;
	let listEl;
	let prevFocus = null;

	// Build the flat, sectioned command list from live sources, then filter.
	function buildItems(allHosts, pageObj, actions) {
		const items = [];
		const onHost = pageObj?.params?.id;

		for (const h of allHosts) {
			items.push({
				group: 'hosts',
				id: `host:${h.id}`,
				label: h.label,
				sub: h.hosted_path ? `~/${String(h.hosted_path).replace(/^[~/]+/, '')}` : '',
				connected: h.connected,
				run: () => goto(`/daemons/${h.id}`)
			});
		}

		items.push({
			group: 'navigate',
			id: 'nav:dashboard',
			label: 'Dashboard',
			run: () => goto('/dashboard')
		});
		items.push({
			group: 'navigate',
			id: 'nav:account',
			label: 'Account settings',
			run: () => goto('/account')
		});
		if (onHost) {
			items.push({
				group: 'navigate',
				id: 'nav:host-files',
				label: "This host's files",
				run: () => goto(`/daemons/${onHost}`)
			});
		}

		for (const a of actions) {
			if (a.when && !a.when()) continue;
			items.push({ group: 'actions', id: a.id, label: a.label, sub: a.hint || '', run: a.run });
		}

		for (const t of THEMES) {
			items.push({
				group: 'appearance',
				id: `theme:${t.value}`,
				label: `Theme: ${t.label}`,
				run: () => setTheme(t.value)
			});
		}
		items.push({
			group: 'appearance',
			id: 'logout',
			label: 'Log out',
			run: () => dispatch('logout')
		});

		return items;
	}

	// Case-insensitive substring + subsequence (fuzzy) score. Lower = better.
	function score(text, q) {
		if (!q) return 0;
		const t = text.toLowerCase();
		const ql = q.toLowerCase();
		const idx = t.indexOf(ql);
		if (idx !== -1) return idx; // substring: rank by position
		// subsequence fallback
		let ti = 0;
		for (let qi = 0; qi < ql.length; qi += 1) {
			ti = t.indexOf(ql[qi], ti);
			if (ti === -1) return Infinity;
			ti += 1;
		}
		return 500 + t.length; // worse than any substring hit
	}

	const GROUP_LABELS = {
		hosts: 'hosts',
		navigate: 'navigate',
		actions: 'actions',
		appearance: 'appearance'
	};
	const GROUP_ORDER = ['hosts', 'navigate', 'actions', 'appearance'];

	$: allItems = buildItems($hosts, $page, $paletteActions);

	$: filtered = (() => {
		const q = query.trim();
		const scored = allItems
			.map((it) => ({ it, s: score(`${it.label} ${it.sub || ''}`, q) }))
			.filter((x) => x.s !== Infinity);
		// Hosts rank first on a label match; otherwise keep group order + score.
		scored.sort((a, b) => {
			const ga = GROUP_ORDER.indexOf(a.it.group);
			const gb = GROUP_ORDER.indexOf(b.it.group);
			if (q && a.s !== b.s) return a.s - b.s;
			if (ga !== gb) return ga - gb;
			return a.s - b.s;
		});
		return scored.map((x) => x.it);
	})();

	// Group the flat filtered list back into sections for rendering (preserving
	// the flat order so keyboard highlight maps 1:1 onto `filtered`).
	$: sections = (() => {
		const out = [];
		let cur = null;
		filtered.forEach((it, i) => {
			if (!cur || cur.group !== it.group) {
				cur = { group: it.group, label: GROUP_LABELS[it.group], rows: [] };
				out.push(cur);
			}
			cur.rows.push({ it, index: i });
		});
		return out;
	})();

	$: if (highlight >= filtered.length) highlight = Math.max(0, filtered.length - 1);

	$: if (open) onOpen();

	async function onOpen() {
		prevFocus = typeof document !== 'undefined' ? document.activeElement : null;
		query = '';
		highlight = 0;
		await tick();
		if (inputEl) inputEl.focus();
	}

	function close() {
		dispatch('close');
		if (prevFocus && typeof prevFocus.focus === 'function') prevFocus.focus();
	}

	function activate(item) {
		if (!item) return;
		close();
		item.run();
	}

	async function scrollHighlightIntoView() {
		await tick();
		if (!listEl) return;
		const el = listEl.querySelector(`[data-idx="${highlight}"]`);
		if (el) el.scrollIntoView({ block: 'nearest' });
	}

	function onKeydown(e) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			highlight = filtered.length ? (highlight + 1) % filtered.length : 0;
			scrollHighlightIntoView();
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			highlight = filtered.length ? (highlight - 1 + filtered.length) % filtered.length : 0;
			scrollHighlightIntoView();
		} else if (e.key === 'Home') {
			e.preventDefault();
			highlight = 0;
			scrollHighlightIntoView();
		} else if (e.key === 'End') {
			e.preventDefault();
			highlight = Math.max(0, filtered.length - 1);
			scrollHighlightIntoView();
		} else if (e.key === 'Enter') {
			e.preventDefault();
			activate(filtered[highlight]);
		} else if (e.key === 'Tab') {
			// The input is the only tab stop (result rows are arrow-driven and
			// tabindex=-1), so trap focus on it while the palette is open.
			e.preventDefault();
			if (inputEl) inputEl.focus();
		}
		// Esc is handled by the layout's Esc-router so the topmost overlay wins.
	}
</script>

{#if open}
	<!-- svelte-ignore a11y-click-events-have-key-events -->
	<!-- svelte-ignore a11y-no-static-element-interactions -->
	<div class="palette-scrim overlay-scrim" on:click={close}></div>
	<div class="palette" role="dialog" aria-modal="true" aria-label="command palette">
		<!-- svelte-ignore a11y-click-events-have-key-events -->
		<!-- svelte-ignore a11y-no-static-element-interactions -->
		<div class="palette__card overlay-surface" on:click|stopPropagation>
			<div class="palette__input-wrap">
				<span class="palette__face">
					<Face state={$hostsStatus.loading ? 'loading' : 'ok'} />
				</span>
				<!-- svelte-ignore a11y-autofocus -->
				<input
					bind:this={inputEl}
					bind:value={query}
					on:keydown={onKeydown}
					class="palette__input"
					type="text"
					placeholder="jump to host, navigate, run a command…"
					autocomplete="off"
					spellcheck="false"
					aria-controls="palette-list"
					aria-label="command palette search"
					aria-activedescendant={filtered.length ? `palette-opt-${highlight}` : undefined}
				/>
			</div>

			<div class="palette__list" id="palette-list" role="listbox" bind:this={listEl}>
				{#if filtered.length === 0}
					<p class="palette__empty"><Face state="error" /> nothing matches '{query}'</p>
				{:else}
					{#each sections as section (section.group)}
						<div class="palette__section-label microlabel">{section.label}</div>
						{#each section.rows as row (row.it.id)}
							<button
								type="button"
								class="palette__row"
								class:palette__row--active={row.index === highlight}
								role="option"
								id="palette-opt-{row.index}"
								tabindex="-1"
								aria-selected={row.index === highlight}
								data-idx={row.index}
								on:mouseenter={() => (highlight = row.index)}
								on:click={() => activate(row.it)}
							>
								{#if row.it.group === 'hosts'}
									<Face state={row.it.connected ? 'ok' : 'offline'} />
								{/if}
								<span class="palette__row-label">{row.it.label}</span>
								{#if row.it.sub}
									<span class="palette__row-sub">{row.it.sub}</span>
								{/if}
							</button>
						{/each}
					{/each}
				{/if}
			</div>

			<div class="palette__foot">
				<span><Kbd>↑↓</Kbd> navigate</span>
				<span><Kbd>↵</Kbd> select</span>
				<span><Kbd>esc</Kbd> close</span>
			</div>
		</div>
	</div>
{/if}

<style>
	.palette-scrim {
		z-index: var(--z-palette-scrim);
	}

	.palette {
		position: fixed;
		inset: 0;
		z-index: var(--z-palette);
		display: flex;
		justify-content: center;
		align-items: flex-start;
		padding: 18vh var(--space-md) var(--space-md);
		pointer-events: none;
	}

	.palette__card {
		pointer-events: auto;
		width: 100%;
		max-width: 34rem;
		display: flex;
		flex-direction: column;
		max-height: 70vh;
		overflow: hidden;
	}

	.palette__input-wrap {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-md);
		border-bottom: 1px solid var(--border);
	}

	.palette__face {
		flex-shrink: 0;
	}

	.palette__input {
		flex: 1;
		width: 100%;
		height: auto;
		padding: 0;
		border: none;
		background: transparent;
		color: var(--text);
		font-size: var(--fs-md);
	}

	.palette__input:focus,
	.palette__input:focus-visible {
		outline: none;
		border: none;
		box-shadow: none;
	}

	.palette__list {
		flex: 1;
		overflow-y: auto;
		padding: var(--space-sm);
	}

	.palette__section-label {
		padding: var(--space-sm) var(--space-sm) var(--space-xs);
	}

	.palette__row {
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
		color: var(--text);
		text-align: left;
	}

	.palette__row--active {
		background: var(--accent-soft);
	}

	.palette__row-label {
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.palette__row-sub {
		margin-left: auto;
		font-size: var(--fs-xs);
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex-shrink: 0;
		max-width: 50%;
	}

	.palette__empty {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-md);
		color: var(--text-muted);
	}

	.palette__foot {
		display: flex;
		gap: var(--space-md);
		padding: var(--space-sm) var(--space-md);
		border-top: 1px solid var(--border);
		font-size: var(--fs-xs);
		color: var(--text-faint);
	}

	.palette__foot span {
		display: inline-flex;
		align-items: center;
		gap: var(--space-xs);
	}

	@media (max-width: 640px) {
		.palette {
			padding: 0;
			align-items: stretch;
		}
		.palette__card {
			max-width: 100%;
			max-height: 100vh;
			border-radius: 0;
			padding-top: var(--safe-top);
		}
		.palette__row {
			min-height: var(--touch-min);
		}
	}
</style>
