// Account settings screen: layout + the profile/password/passkey interactions,
// with the API mocked at the network layer (the bastion integration suite
// covers the real backend behavior). Runs across the configured viewports.
import { test, expect } from '@playwright/test';

// Mock the whole API surface the account screen + app shell touch. `account`
// is a mutable object so PATCH/GET round-trips reflect edits. Pass overrides to
// exercise the password-only / passkey-only / set-password variants.
async function mockAccount(page, overrides = {}) {
	const account = {
		username: 'zach',
		email: '',
		has_password: true,
		password_enabled: true,
		passkey_enabled: true,
		...overrides
	};
	const calls = { patch: 0, password: 0 };

	await page.addInitScript(() => localStorage.setItem('blackhaul_authed', 'true'));
	// Most-recently-registered route wins, so the catch-all is registered first.
	await page.route('**/api/**', (route) => route.fulfill({ json: {} }));
	await page.route('**/api/daemons', (route) => route.fulfill({ json: [] }));
	await page.route('**/api/passkeys', (route) => route.fulfill({ json: [] }));
	await page.route('**/api/account', (route) => {
		const req = route.request();
		if (req.method() === 'PATCH') {
			calls.patch += 1;
			const body = JSON.parse(req.postData() || '{}');
			account.email = body.email || '';
			return route.fulfill({ json: { email: account.email } });
		}
		return route.fulfill({ json: account });
	});
	await page.route('**/api/account/password', (route) => {
		calls.password += 1;
		return route.fulfill({ json: { status: 'ok' } });
	});
	// Keep Gravatar off the real network: any avatar request gets a 1x1 GIF.
	// A valid 1x1 transparent GIF so the avatar <img> loads (an invalid body
	// would fire on:error and fall back to the monogram).
	await page.route('**gravatar.com/**', (route) =>
		route.fulfill({
			contentType: 'image/gif',
			body: Buffer.from('R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7', 'base64')
		})
	);
	return { account, calls };
}

async function expectNoHorizontalScroll(page) {
	const overflow = await page.evaluate(
		() => document.documentElement.scrollWidth - document.documentElement.clientWidth
	);
	expect(overflow, 'page must not scroll horizontally').toBeLessThanOrEqual(0);
}

test('account screen shows profile, password and passkey sections', async ({ page }) => {
	await mockAccount(page);
	await page.goto('/account');

	await expect(page.getByRole('heading', { name: 'account', exact: true })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'profile' })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'password' })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'passkeys' })).toBeVisible();
	await expect(page.locator('#email')).toBeVisible();
	await expect(page.getByRole('button', { name: 'change password' })).toBeVisible();

	await expectNoHorizontalScroll(page);
	const viewport = page.viewportSize();
	await page.screenshot({ path: `e2e/screenshots/account-${viewport.width}.png`, fullPage: true });
});

test('saving an email PATCHes the account and updates the avatar', async ({ page }) => {
	const { calls } = await mockAccount(page);
	await page.goto('/account');

	// Save is disabled until the field differs from the saved value.
	const save = page.getByRole('button', { name: 'save', exact: true });
	await expect(save).toBeDisabled();

	await page.locator('#email').fill('zach@zdods.com');
	await expect(save).toBeEnabled();
	await save.click();

	await expect(page.locator('.toast')).toContainText('email saved');
	expect(calls.patch).toBe(1);
	// The avatar image now points at Gravatar (header + profile). Anchor the
	// host so the pattern can't match an attacker-controlled prefix/suffix.
	await expect(page.locator('.profile-head img')).toHaveAttribute(
		'src',
		/^https:\/\/www\.gravatar\.com\//
	);
});

test('mismatched new passwords are rejected client-side', async ({ page }) => {
	const { calls } = await mockAccount(page);
	await page.goto('/account');

	await page.locator('#current-password').fill('old-password-123');
	await page.locator('#new-password').fill('new-password-456');
	await page.locator('#confirm-password').fill('different-789');
	await page.getByRole('button', { name: 'change password' }).click();

	await expect(page.getByRole('alert')).toContainText('do not match');
	expect(calls.password, 'no request on client-side validation failure').toBe(0);
});

test('a valid password change posts and confirms', async ({ page }) => {
	const { calls } = await mockAccount(page);
	await page.goto('/account');

	await page.locator('#current-password').fill('old-password-123');
	await page.locator('#new-password').fill('new-password-456');
	await page.locator('#confirm-password').fill('new-password-456');
	await page.getByRole('button', { name: 'change password' }).click();

	await expect(page.locator('.toast')).toContainText('password changed');
	expect(calls.password).toBe(1);
	await expectNoHorizontalScroll(page);
});

test('passkey-only account is offered to set a password', async ({ page }) => {
	await mockAccount(page, { has_password: false });
	await page.goto('/account');

	// No current-password field, and the action reads "set password".
	await expect(page.locator('#current-password')).toBeHidden();
	await expect(page.getByRole('button', { name: 'set password' })).toBeVisible();
	await expect(page.getByText('Set a password to enable password sign-in')).toBeVisible();
});

test('password-disabled mode hides the password section', async ({ page }) => {
	await mockAccount(page, { password_enabled: false, has_password: false });
	await page.goto('/account');

	await expect(page.getByRole('heading', { name: 'profile' })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'password' })).toBeHidden();
	await expect(page.getByRole('heading', { name: 'passkeys' })).toBeVisible();
});

test('passkeys moved off the dashboard and onto the account screen', async ({ page }) => {
	await mockAccount(page);

	await page.goto('/dashboard');
	await expect(page.getByRole('heading', { name: 'hosts', exact: true })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'passkeys' })).toHaveCount(0);

	// The header avatar links to the account screen.
	await page.locator('.app-header__account').click();
	await page.waitForURL('**/account');
	await expect(page.getByRole('heading', { name: 'passkeys' })).toBeVisible();
});
