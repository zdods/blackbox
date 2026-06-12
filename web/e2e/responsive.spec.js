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
	await page.addInitScript(() => localStorage.setItem('blackhaul_token', 'e2e'));
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
	await expect(page.getByText('test-host')).toBeVisible();
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
	if (isMobile(viewport)) {
		// The modified column collapses on small screens.
		await expect(page.locator('th.col-mtime')).toBeHidden();
	} else {
		await expect(page.locator('th.col-mtime')).toBeVisible();
		await expect(page.getByText('2026-02-02T10:30:00Z')).toBeVisible();
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
