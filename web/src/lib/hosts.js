// Shared hosts store + single 8s poller.
//
// Consolidates what used to be the dashboard's local setInterval AND the
// file-browser's one-off /api/daemons label lookup into ONE source of truth so
// the sidebar, dashboard, and command palette all read the same live host list.
//
// Lifecycle (ref-counted): the poll starts on the first active subscriber when
// isLoggedIn() is true and stops when the last active subscriber leaves or on
// logout. Every fetch is guarded by isLoggedIn(); a 401 clears the auth flag
// and redirects to /login.
import { writable } from 'svelte/store';
import { isLoggedIn, apiFetch, redirectIfUnauthorized } from '$lib/auth.js';

const POLL_INTERVAL_MS = 8000;

// Internal writable: array of
// { id, label, hosted_path, connected, disk_free, disk_total }.
const _hosts = writable([]);
// Lifecycle flags so consumers can show skeletons / empty states correctly.
// loading is true until the first successful (or failed) fetch resolves.
const _status = writable({ loading: true, error: '' });

let activeCount = 0;
let intervalId = null;
let inFlight = false;

async function fetchOnce() {
	if (!isLoggedIn()) {
		stop();
		return;
	}
	if (inFlight) return;
	inFlight = true;
	try {
		const res = await apiFetch('/api/daemons');
		if (res.status === 401) {
			stop(); // halt the poll loop before the shared helper navigates away
			redirectIfUnauthorized(res);
			return;
		}
		if (!res.ok) throw new Error(await res.text());
		const data = await res.json();
		_hosts.set(Array.isArray(data) ? data : []);
		_status.set({ loading: false, error: '' });
	} catch (e) {
		_status.set({ loading: false, error: e?.message || 'failed to load hosts' });
	} finally {
		inFlight = false;
	}
}

// Begin polling (idempotent). Increments the active-subscriber refcount.
// Call from onMount; pair with stop() in onDestroy.
export function start() {
	activeCount += 1;
	if (intervalId) return;
	if (!isLoggedIn()) return;
	// Immediate fetch so first paint isn't stuck on stale/empty data, then poll.
	fetchOnce();
	intervalId = setInterval(fetchOnce, POLL_INTERVAL_MS);
}

// Decrement the refcount; clears the interval when no active subscribers remain
// (or when called unconditionally on logout via the internal stop path).
export function stop() {
	if (activeCount > 0) activeCount -= 1;
	if (activeCount > 0) return;
	if (intervalId) {
		clearInterval(intervalId);
		intervalId = null;
	}
}

// Force an out-of-band refresh (e.g. right after creating/renaming/deleting a
// host) without waiting for the next poll tick. No-op if logged out.
export function refresh() {
	return fetchOnce();
}

// Read-only stores for components.
export const hosts = { subscribe: _hosts.subscribe };
export const hostsStatus = { subscribe: _status.subscribe };

// Look up a single host by id from the current snapshot (sync). Returns
// undefined if not loaded yet — callers should fall back to the id string.
export function getHost(id) {
	let found;
	const unsub = _hosts.subscribe((list) => {
		found = list.find((h) => String(h.id) === String(id));
	});
	unsub();
	return found;
}
