import { writable } from 'svelte/store';

// Theme choice persisted in localStorage; 'system' resolves to light/dark
// via prefers-color-scheme. app.html applies the resolved theme before
// first paint so there is no flash — this module takes over after hydration.
const KEY = 'blackhaul-theme';

export const THEMES = [
	{ value: 'system', label: 'system' },
	{ value: 'light', label: 'light' },
	{ value: 'dark', label: 'dark' },
	{ value: 'nord', label: 'nord' }
];

const THEME_COLORS = { light: '#f7f7f5', dark: '#0c0d0f', nord: '#2e3440' };

function storedChoice() {
	if (typeof window === 'undefined') return 'system';
	const t = localStorage.getItem(KEY);
	return THEMES.some((x) => x.value === t) ? t : 'system';
}

function resolve(choice) {
	if (choice !== 'system') return choice;
	if (typeof window === 'undefined') return 'dark';
	return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function apply(choice) {
	if (typeof document === 'undefined') return;
	const resolved = resolve(choice);
	document.documentElement.setAttribute('data-theme', resolved);
	document.documentElement.style.colorScheme = resolved === 'light' ? 'light' : 'dark';
	const meta = document.querySelector('meta[name="theme-color"]');
	if (meta) meta.setAttribute('content', THEME_COLORS[resolved]);
}

export const theme = writable(storedChoice());

export function setTheme(choice) {
	if (typeof window !== 'undefined') localStorage.setItem(KEY, choice);
	theme.set(choice);
	apply(choice);
}

// Follow OS theme changes while the choice is 'system'.
if (typeof window !== 'undefined') {
	window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
		if (storedChoice() === 'system') apply('system');
	});
}
