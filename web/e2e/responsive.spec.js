// Validates the console layout at desktop and mobile viewports against the
// real build, with the API mocked at the network layer (layout only — the
// bastion integration suite covers behavior).
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
	{ name: 'notes.txt', is_dir: false, size: 2048, mtime: '2026-02-02T10:30:00Z' },
	{
		name: 'a-very-long-filename-that-should-not-break-the-mobile-layout-at-all.tar.gz',
		is_dir: false,
		size: 123456789,
		mtime: '2026-03-03T08:00:00Z'
	}
];

// Mock the API and mark the client as logged in. Playwright gives the most
// recently registered route precedence, so the catch-all goes first.
async function loginAndMock(page) {
	await page.addInitScript(() => localStorage.setItem('blackhaul_authed', 'true'));
	await page.route('**/api/**', (route) => route.fulfill({ json: {} }));
	await page.route('**/api/daemons', (route) => route.fulfill({ json: [DAEMON] }));
	await page.route('**/api/daemons/d1/files**', (route) => route.fulfill({ json: ENTRIES }));
}

async function expectNoHorizontalScroll(page) {
	const overflow = await page.evaluate(
		() => document.documentElement.scrollWidth - document.documentElement.clientWidth
	);
	expect(overflow, 'page must not scroll horizontally').toBeLessThanOrEqual(0);
}

function isMobile(viewport) {
	return viewport.width < 640;
}

test('login page fits the viewport', async ({ page }) => {
	await page.goto('/login');
	await expect(page.locator('form')).toBeVisible();
	await expectNoHorizontalScroll(page);
	// The form never exceeds the viewport.
	const form = await page.locator('form').boundingBox();
	const viewport = page.viewportSize();
	expect(form.width).toBeLessThanOrEqual(viewport.width);
	await page.screenshot({ path: `e2e/screenshots/login-${viewport.width}.png`, fullPage: true });
});

test('register page fits the viewport', async ({ page }) => {
	await page.route('**/api/setup', (route) => route.fulfill({ json: { registration_open: true } }));
	await page.goto('/register');
	await expectNoHorizontalScroll(page);
});

test('dashboard renders the daemon list without overflow', async ({ page }) => {
	await loginAndMock(page);
	await page.goto('/dashboard');
	// The host label now renders in both the sidebar rail and the dashboard
	// card (shared roster), so scope to the dashboard card's label. It keeps
	// overflow:hidden + ellipsis, so it must still render without overflowing.
	const hostLabel = page.locator('a.host-card__label', { hasText: 'test-host' });
	await expect(hostLabel).toBeVisible();
	await expectNoHorizontalScroll(page);
	const viewport = page.viewportSize();
	await page.screenshot({
		path: `e2e/screenshots/dashboard-${viewport.width}.png`,
		fullPage: true
	});
});

test('file browser adapts columns to the viewport', async ({ page }) => {
	await loginAndMock(page);
	await page.goto('/daemons/d1');
	await expect(page.locator('table.file-list')).toBeVisible();
	await expect(page.getByText('notes.txt')).toBeVisible();

	const viewport = page.viewportSize();
	// The modified date for notes.txt — rendered in the table's mtime column on
	// desktop, and inline as a card meta field on mobile (the same td.col-mtime).
	// The visible text is a friendly date ("Feb 2, 2026"); the raw ISO lives in
	// the title attribute, which we match on (locale-independent).
	const mtimeCell = page.locator('td.col-mtime[title="2026-02-02T10:30:00Z"]');
	if (isMobile(viewport)) {
		// The modified-column header collapses on small screens, but the date
		// itself survives as the mobile card's inline meta field.
		await expect(page.locator('th.col-mtime')).toBeHidden();
		await expect(mtimeCell).toBeVisible();
	} else {
		// On desktop the dedicated modified column is shown with its date.
		await expect(page.locator('th.col-mtime')).toBeVisible();
		await expect(mtimeCell).toBeVisible();
	}

	// Long filenames must not force horizontal scrolling.
	await expect(page.getByText(/a-very-long-filename/)).toBeVisible();
	await expectNoHorizontalScroll(page);
	await page.screenshot({ path: `e2e/screenshots/files-${viewport.width}.png`, fullPage: true });
});

test('upload controls stay within the viewport', async ({ page }) => {
	await loginAndMock(page);
	await page.goto('/daemons/d1');
	await expect(page.locator('.upload-row')).toBeVisible();
	const viewport = page.viewportSize();
	const row = await page.locator('.upload-row').boundingBox();
	expect(row.x + row.width).toBeLessThanOrEqual(viewport.width + 1);
	await expectNoHorizontalScroll(page);
});

test('preview modal fits the viewport', async ({ page }) => {
	await loginAndMock(page);
	// notes.txt is previewable; its download fetch returns text content.
	await page.route('**/api/daemons/d1/files**download=1**', (route) =>
		route.fulfill({ body: 'hello preview', contentType: 'text/plain' })
	);
	await page.goto('/daemons/d1');
	await page.getByRole('button', { name: 'notes.txt', exact: true }).click();
	const modal = page.locator('.preview-modal');
	await expect(modal).toBeVisible();
	const box = await modal.boundingBox();
	const viewport = page.viewportSize();
	expect(box.width).toBeLessThanOrEqual(viewport.width);
	await expectNoHorizontalScroll(page);
	const name = `preview-${viewport.width}.png`;
	await page.screenshot({ path: `e2e/screenshots/${name}` });
});
