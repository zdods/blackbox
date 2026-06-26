// Exercises the file-browser upload path end to end through the UI, asserting
// the request shape the extracted $lib/upload.js produces — a single PUT for a
// small file, and chunked PUTs (shared upload_id, chunk_index/total_chunks) for
// a file larger than the 5 MB chunk size.
import { test, expect } from '@playwright/test';

const DAEMON = {
	id: 'd1',
	label: 'host',
	hosted_path: '/files',
	connected: true,
	disk_free: 1e9,
	disk_total: 2e9
};

async function mount(page, onPut) {
	await page.addInitScript(() => localStorage.setItem('blackhaul_authed', 'true'));
	await page.route('**/api/**', (r) => r.fulfill({ json: {} }));
	await page.route('**/api/daemons', (r) => r.fulfill({ json: [DAEMON] }));
	await page.route('**/api/daemons/d1/files**', (route) => {
		const req = route.request();
		if (req.method() === 'PUT') {
			onPut(new URL(req.url()));
			return route.fulfill({ status: 200, body: '' });
		}
		return route.fulfill({ json: [] }); // GET listing (empty)
	});
}

test('small file uploads as a single PUT', async ({ page }) => {
	const puts = [];
	await mount(page, (url) => puts.push(url));
	await page.goto('/daemons/d1');
	await expect(page.locator('.upload-row')).toBeVisible();

	await page.locator('input[type=file]').setInputFiles({
		name: 'note.txt',
		mimeType: 'text/plain',
		buffer: Buffer.from('hello world')
	});

	await expect.poll(() => puts.length).toBe(1);
	expect(puts[0].searchParams.get('path')).toBe('note.txt');
	expect(puts[0].searchParams.get('upload_id')).toBeNull(); // single-shot, not chunked
	await expect(page.locator('.error')).toHaveCount(0);
});

test('large file uploads in chunks with a shared upload_id', async ({ page }) => {
	const puts = [];
	await mount(page, (url) => puts.push(url));
	await page.goto('/daemons/d1');
	await expect(page.locator('.upload-row')).toBeVisible();

	// 6 MB → two chunks (5 MB + 1 MB) at the 5 MB CHUNK_SIZE.
	await page.locator('input[type=file]').setInputFiles({
		name: 'big.bin',
		mimeType: 'application/octet-stream',
		buffer: Buffer.alloc(6 * 1024 * 1024, 1)
	});

	await expect.poll(() => puts.length).toBe(2);
	const ids = new Set(puts.map((u) => u.searchParams.get('upload_id')));
	expect(ids.size).toBe(1); // one shared upload_id
	expect([...ids][0]).toBeTruthy();
	expect(puts.map((u) => u.searchParams.get('chunk_index'))).toEqual(['0', '1']);
	expect(puts.every((u) => u.searchParams.get('total_chunks') === '2')).toBe(true);
	await expect(page.locator('.error')).toHaveCount(0);
});
