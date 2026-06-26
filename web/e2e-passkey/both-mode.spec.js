import { test, expect } from '@playwright/test';

// In AUTH_MODE=both the register and login screens must offer BOTH the passkey
// and the password+TOTP options side by side. Run against a bastion started in
// "both" mode (E2E_BASE_URL points at it).
test.describe.configure({ mode: 'serial' });

async function addAuthenticator(page) {
	const client = await page.context().newCDPSession(page);
	await client.send('WebAuthn.enable');
	await client.send('WebAuthn.addVirtualAuthenticator', {
		options: {
			protocol: 'ctap2',
			transport: 'internal',
			hasResidentKey: true,
			hasUserVerification: true,
			isUserVerified: true,
			automaticPresenceSimulation: true
		}
	});
}

test('both mode shows passkey + password on register and login, and both work', async ({
	page
}) => {
	await addAuthenticator(page);

	// --- register screen offers both methods ---
	await page.goto('/');
	await page.waitForURL('**/register');
	await expect(page.locator('#pk-username')).toBeVisible(); // passkey form
	await expect(page.getByRole('button', { name: 'create passkey' })).toBeVisible();
	await expect(page.locator('#username')).toBeVisible(); // password form
	await expect(page.locator('#password')).toBeVisible();
	await expect(page.getByRole('button', { name: 'continue' })).toBeVisible();

	// register via the passkey path
	await page.locator('#pk-username').fill('zach');
	await page.getByRole('button', { name: 'create passkey' }).click();
	await page.waitForURL('**/dashboard');
	await expect(page.getByRole('heading', { name: 'hosts', exact: true })).toBeVisible();

	// --- logout, then the login screen offers both methods ---
	await page.locator('.logout-btn').first().click();
	await page.waitForURL('**/login');
	await expect(page.getByRole('button', { name: 'sign in with passkey' })).toBeVisible();
	await expect(page.locator('#username')).toBeVisible();
	await expect(page.locator('#password')).toBeVisible();
	await expect(page.getByRole('button', { name: 'log in' })).toBeVisible();

	// sign in via the passkey path
	await page.getByRole('button', { name: 'sign in with passkey' }).click();
	await page.waitForURL('**/dashboard');
	await expect(page.getByRole('heading', { name: 'hosts', exact: true })).toBeVisible();
});
