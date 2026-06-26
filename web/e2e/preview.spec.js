// Confirms the image branch of $lib/preview.js (isImageExt) still drives the
// preview modal to render an <img>, and that the modal closes. Text previews are
// covered by responsive.spec.js.
import { test, expect } from '@playwright/test';

const DAEMON = {
	id: 'd1',
	label: 'host',
	hosted_path: '/files',
	connected: true,
	disk_free: 1e9,
	disk_total: 2e9
};
const ENTRIES = [{ name: 'photo.png', is_dir: false, size: 1024, mtime: '2026-02-02T10:30:00Z' }];
// 1x1 transparent PNG.
const PNG = Buffer.from(
	'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==',
	'base64'
);

test('image file opens an image preview and closes', async ({ page }) => {
	await page.addInitScript(() => localStorage.setItem('blackhaul_authed', 'true'));
	await page.route('**/api/**', (r) => r.fulfill({ json: {} }));
	await page.route('**/api/daemons', (r) => r.fulfill({ json: [DAEMON] }));
	await page.route('**/api/daemons/d1/files**download=1**', (r) =>
		r.fulfill({ contentType: 'image/png', body: PNG })
	);
	await page.route('**/api/daemons/d1/files**', (r) => r.fulfill({ json: ENTRIES }));

	await page.goto('/daemons/d1');
	await page.getByRole('button', { name: 'photo.png', exact: true }).click();

	const modal = page.locator('.preview-modal');
	await expect(modal).toBeVisible();
	await expect(modal.locator('img')).toBeVisible();

	await page.getByRole('button', { name: 'close' }).click();
	await expect(modal).toBeHidden();
});
