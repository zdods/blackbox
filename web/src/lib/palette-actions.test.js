import { describe, it, expect } from 'vitest';
import { get } from 'svelte/store';
import { registerActions, paletteActions } from './palette-actions.js';

function ids() {
	return get(paletteActions).map((a) => a.id);
}

describe('registerActions', () => {
	it('adds actions and returns a working unregister fn', () => {
		const off = registerActions([{ id: 'a', label: 'A', run() {} }]);
		expect(ids()).toContain('a');
		off();
		expect(ids()).not.toContain('a');
	});

	it('accepts a single action (not just an array)', () => {
		const off = registerActions({ id: 'solo', label: 'Solo', run() {} });
		expect(ids()).toContain('solo');
		off();
	});

	it('replaces a prior entry with the same id (idempotent on id)', () => {
		const off1 = registerActions([{ id: 'dup', label: 'first', run() {} }]);
		const off2 = registerActions([{ id: 'dup', label: 'second', run() {} }]);
		const entries = get(paletteActions).filter((a) => a.id === 'dup');
		expect(entries).toHaveLength(1);
		expect(entries[0].label).toBe('second');
		off1();
		off2();
		expect(ids()).not.toContain('dup');
	});
});
