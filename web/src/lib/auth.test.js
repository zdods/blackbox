import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as nav from '$app/navigation';
import { isLoggedIn, setLoggedIn, clearLoggedIn, redirectIfUnauthorized } from './auth.js';

beforeEach(() => {
	localStorage.clear();
	vi.restoreAllMocks();
});

describe('login flag (localStorage)', () => {
	it('round-trips set → is → clear', () => {
		expect(isLoggedIn()).toBe(false);
		setLoggedIn();
		expect(isLoggedIn()).toBe(true);
		clearLoggedIn();
		expect(isLoggedIn()).toBe(false);
	});
});

describe('redirectIfUnauthorized', () => {
	it('on 401: clears the flag, navigates to /login, returns true', () => {
		const goto = vi.spyOn(nav, 'goto').mockImplementation(() => {});
		setLoggedIn();
		const acted = redirectIfUnauthorized({ status: 401 });
		expect(acted).toBe(true);
		expect(isLoggedIn()).toBe(false);
		expect(goto).toHaveBeenCalledWith('/login');
	});

	it('on non-401: does nothing and returns false', () => {
		const goto = vi.spyOn(nav, 'goto').mockImplementation(() => {});
		setLoggedIn();
		const acted = redirectIfUnauthorized({ status: 200 });
		expect(acted).toBe(false);
		expect(isLoggedIn()).toBe(true);
		expect(goto).not.toHaveBeenCalled();
	});
});
