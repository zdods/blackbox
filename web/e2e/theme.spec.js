// Theme switcher: works on every viewport, and on mobile it collapses to an
// icon-only control whose native <select> is fully transparent (opacity:0) so
// the value text can't bleed through the glyph.
import { test, expect } from '@playwright/test';

async function shell(page) {
	await page.addInitScript(() => localStorage.setItem('blackhaul_authed', 'true'));
	await page.route('**/api/**', (r) => r.fulfill({ json: {} }));
	await page.route('**/api/daemons', (r) => r.fulfill({ json: [] }));
	await page.goto('/dashboard');
	await expect(page.getByRole('heading', { name: 'hosts', exact: true })).toBeVisible();
}

test('changing the theme updates data-theme', async ({ page }) => {
	await shell(page);
	const select = page.getByRole('combobox', { name: 'color theme' });
	await select.selectOption('nord');
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'nord');
	await select.selectOption('light');
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
});

test('mobile shows an icon with a transparent select (no text bleed)', async ({ page }) => {
	test.skip(page.viewportSize().width >= 640, 'mobile-only');
	await shell(page);
	await expect(page.locator('.theme-select__glyph')).toBeVisible();
	const opacity = await page
		.locator('.theme-select select')
		.evaluate((el) => getComputedStyle(el).opacity);
	expect(opacity).toBe('0');
	await page.getByRole('combobox', { name: 'color theme' }).selectOption('dark');
	await expect(page.locator('.theme-select__glyph')).toHaveText('●');
});

test('desktop shows the labelled select (glyph hidden)', async ({ page }) => {
	test.skip(page.viewportSize().width < 640, 'desktop-only');
	await shell(page);
	await expect(page.locator('.theme-select__label')).toBeVisible();
	await expect(page.locator('.theme-select__glyph')).toBeHidden();
	const opacity = await page
		.locator('.theme-select select')
		.evaluate((el) => getComputedStyle(el).opacity);
	expect(opacity).toBe('1');
});
