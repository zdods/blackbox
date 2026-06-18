// Route-contextual action registry for the command palette.
//
// Screens register actions on mount and deregister on destroy. The palette
// (mounted once in the layout) subscribes to `paletteActions` to populate its
// ACTIONS section. Each action: { id, label, run, group?, when?, hint? }.
//
// Usage from a screen:
//   import { registerActions } from '$lib/palette-actions.js';
//   onMount(() => registerActions([
//     { id: 'add-host', label: 'Add host…', run: () => focusAddHost() }
//   ])); // returns an unregister fn — call it in onDestroy
import { writable } from 'svelte/store';

const _actions = writable([]);

// Read-only store for the palette.
export const paletteActions = { subscribe: _actions.subscribe };

// Register a batch of actions; returns an unregister function. Idempotent on id
// (re-registering the same id replaces the prior entry).
export function registerActions(actions) {
	const list = Array.isArray(actions) ? actions : [actions];
	const ids = new Set(list.map((a) => a.id));
	_actions.update((cur) => [...cur.filter((a) => !ids.has(a.id)), ...list]);
	return () => {
		_actions.update((cur) => cur.filter((a) => !ids.has(a.id)));
	};
}
