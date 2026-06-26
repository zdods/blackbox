// Shared transient toast. One store + one mounted <Toast/> (in +layout.svelte)
// replaces the per-screen toast state every route used to roll itself — the
// same single-instance approach as $lib/ConfirmDialog.svelte. Call showToast()
// from anywhere; the styling lives on the global .toast classes in app.css.
import { writable } from 'svelte/store';

export const toast = writable({ show: false, message: '', type: 'success' });

let timer = null;

// showToast surfaces a message for `duration` ms (type drives the .toast-* color).
export function showToast(message, type = 'success', duration = 3000) {
	if (timer) clearTimeout(timer);
	toast.set({ show: true, message, type });
	timer = setTimeout(() => {
		toast.update((t) => ({ ...t, show: false }));
		timer = null;
	}, duration);
}
