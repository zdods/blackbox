// The mobile host drawer must toggle from its own hamburger — a second tap
// dismisses it, not only a tap on the scrim. (Regression: the header used to go
// inert while the drawer was open, which disabled the hamburger.)
import { test, expect } from '@playwright/test';

async function shell(page) {
	await page.addInitScript(() => localStorage.setItem('blackhaul_authed', 'true'));
	await page.route('**/api/**', (r) => r.fulfill({ json: {} }));
	await page.route('**/api/daemons', (r) => r.fulfill({ json: [] }));
	await page.goto('/dashboard');
	await expect(page.getByRole('heading', { name: 'hosts', exact: true })).toBeVisible();
}

test('hamburger toggles the drawer open and closed', async ({ page }) => {
	test.skip(page.viewportSize().width >= 640, 'mobile-only');
	await shell(page);
	const hamburger = page.getByRole('button', { name: 'toggle hosts' });
	const sidebar = page.locator('#app-sidebar');

	await hamburger.click();
	await expect(sidebar).toHaveClass(/app-sidebar--open/);
	await expect(hamburger).toHaveAttribute('aria-expanded', 'true');

	// Second tap on the hamburger dismisses it (the actual fix).
	await hamburger.click();
	await expect(sidebar).not.toHaveClass(/app-sidebar--open/);
	await expect(hamburger).toHaveAttribute('aria-expanded', 'false');
});

test('scrim and Escape still dismiss the drawer', async ({ page }) => {
	test.skip(page.viewportSize().width >= 640, 'mobile-only');
	await shell(page);
	const hamburger = page.getByRole('button', { name: 'toggle hosts' });
	const sidebar = page.locator('#app-sidebar');

	await hamburger.click();
	await expect(sidebar).toHaveClass(/app-sidebar--open/);
	// Scrim: click the far right edge, away from the left-anchored drawer panel.
	await page
		.locator('.drawer-scrim')
		.click({ position: { x: page.viewportSize().width - 5, y: 300 } });
	await expect(sidebar).not.toHaveClass(/app-sidebar--open/);

	await hamburger.click();
	await expect(sidebar).toHaveClass(/app-sidebar--open/);
	await page.keyboard.press('Escape');
	await expect(sidebar).not.toHaveClass(/app-sidebar--open/);
});
