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
const THEME_ACCENTS = { light: '#0c8c5e', dark: '#3fd68c', nord: '#a3be8c' };

// Build the [▪‿▪] favicon recolored for the resolved theme. The background rect
// uses --bg and the face uses --accent, mirroring web/static/icons/icon.svg.
function faviconDataUri(resolved) {
	const bg = THEME_COLORS[resolved];
	const fg = THEME_ACCENTS[resolved];
	const svg =
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` +
		`<rect width="100" height="100" rx="20" fill="${bg}"/>` +
		`<g stroke="${fg}" stroke-width="6" fill="none">` +
		`<path d="M 24 28 H 16 V 72 H 24" stroke-linecap="square"/>` +
		`<path d="M 76 28 H 84 V 72 H 76" stroke-linecap="square"/>` +
		`<path d="M 44 58 Q 50 66 56 58" stroke-linecap="round"/></g>` +
		`<g fill="${fg}">` +
		`<rect x="28" y="40" width="12" height="12"/>` +
		`<rect x="60" y="40" width="12" height="12"/></g></svg>`;
	return 'data:image/svg+xml,' + encodeURIComponent(svg);
}

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
	const favicon = document.getElementById('favicon');
	if (favicon) favicon.setAttribute('href', faviconDataUri(resolved));
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
