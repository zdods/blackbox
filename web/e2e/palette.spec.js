// Command-palette coverage: verifies the palette surfaces the route-contextual
// actions for each screen (account, dashboard, host file browser), with the API
// mocked at the network layer. Runs across the configured viewports.
import { test, expect } from '@playwright/test';

const DAEMON = {
	id: 'd1',
	label: 'test-host',
	hosted_path: '/files',
	connected: true,
	disk_free: 1024 * 1024 * 1024,
	disk_total: 2 * 1024 * 1024 * 1024
};

const ENTRIES = [
	{ name: 'documents', is_dir: true, size: 0, mtime: '2026-01-01T00:00:00Z' },
	{ name: 'notes.txt', is_dir: false, size: 2048, mtime: '2026-02-02T10:30:00Z' }
];

async function authMock(page, accountOverrides = {}) {
	await page.addInitScript(() => localStorage.setItem('blackhaul_authed', 'true'));
	await page.route('**/api/**', (route) => route.fulfill({ json: {} }));
	await page.route('**/api/daemons', (route) => route.fulfill({ json: [DAEMON] }));
	await page.route('**/api/daemons/d1/files**', (route) => route.fulfill({ json: ENTRIES }));
	await page.route('**/api/passkeys', (route) => route.fulfill({ json: [] }));
	await page.route('**/api/account', (route) =>
		route.fulfill({
			json: {
				username: 'zach',
				email: '',
				has_password: true,
				password_enabled: true,
				passkey_enabled: true,
				...accountOverrides
			}
		})
	);
}

async function openPalette(page) {
	await page.getByRole('button', { name: 'open command palette' }).click();
	const palette = page.getByRole('dialog', { name: 'command palette' });
	await expect(palette).toBeVisible();
	return palette;
}

test('account screen exposes its actions in the palette', async ({ page }) => {
	await authMock(page);
	await page.goto('/account');
	await expect(page.getByRole('heading', { name: 'profile' })).toBeVisible();

	const palette = await openPalette(page);
	await expect(palette.getByRole('option', { name: 'Edit email' })).toBeVisible();
	await expect(palette.getByRole('option', { name: 'Change password' })).toBeVisible();
	await expect(palette.getByRole('option', { name: 'Add a passkey' })).toBeVisible();
	// Navigation entries are present too.
	await expect(palette.getByRole('option', { name: 'Account settings' })).toBeVisible();

	// Running "Edit email" closes the palette and focuses the email field.
	await palette.getByRole('option', { name: 'Edit email' }).click();
	await expect(page.locator('#email')).toBeFocused();
});

test('passkey-only account shows "Set password" instead of "Change password"', async ({ page }) => {
	await authMock(page, { has_password: false });
	await page.goto('/account');
	await expect(page.getByRole('heading', { name: 'profile' })).toBeVisible();

	const palette = await openPalette(page);
	await expect(palette.getByRole('option', { name: 'Set password' })).toBeVisible();
	await expect(palette.getByRole('option', { name: 'Change password' })).toHaveCount(0);
});

test('dashboard exposes host-management actions in the palette', async ({ page }) => {
	await authMock(page);
	await page.goto('/dashboard');
	await expect(page.getByRole('heading', { name: 'hosts', exact: true })).toBeVisible();

	const palette = await openPalette(page);
	await expect(palette.getByRole('option', { name: 'Add host' })).toBeVisible();
	await expect(palette.getByRole('option', { name: 'Refresh hosts' })).toBeVisible();
});

test('host file browser exposes file actions, including refresh and go-up', async ({ page }) => {
	await authMock(page);
	await page.goto('/daemons/d1');
	await expect(page.getByText('notes.txt')).toBeVisible();

	// At the root: Refresh files + Upload present; Go up hidden (already at root).
	let palette = await openPalette(page);
	await expect(palette.getByRole('option', { name: 'Refresh files' })).toBeVisible();
	await expect(palette.getByRole('option', { name: 'Upload files' })).toBeVisible();
	await expect(palette.getByRole('option', { name: 'Go up a directory' })).toHaveCount(0);
	await page.keyboard.press('Escape');
	await expect(palette).toBeHidden();

	// Enter a subdirectory (directory rows render with a trailing slash); now
	// Go up a directory is available (re-evaluated when the palette opens).
	await page.getByRole('button', { name: 'documents/', exact: true }).click();
	palette = await openPalette(page);
	await expect(palette.getByRole('option', { name: 'Go up a directory' })).toBeVisible();
});
