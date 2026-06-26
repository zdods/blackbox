import { test, expect } from '@playwright/test';

// Drive the whole passkey lifecycle against a real bastion in AUTH_MODE=passkey
// using a CDP virtual authenticator (resident keys + auto user verification), so
// navigator.credentials.create()/get() succeed without human interaction.
test.describe.configure({ mode: 'serial' });

async function addAuthenticator(page) {
	const client = await page.context().newCDPSession(page);
	await client.send('WebAuthn.enable');
	const { authenticatorId } = await client.send('WebAuthn.addVirtualAuthenticator', {
		options: {
			protocol: 'ctap2',
			transport: 'internal',
			hasResidentKey: true,
			hasUserVerification: true,
			isUserVerified: true,
			automaticPresenceSimulation: true
		}
	});
	return { client, authenticatorId };
}

test('register, discoverable login, enroll, remove, last-passkey guard', async ({ page }) => {
	const { client, authenticatorId } = await addAuthenticator(page);

	// --- first-run registration creates the account + a resident passkey ---
	await page.goto('/');
	await page.waitForURL('**/register');
	await page.locator('#pk-username').fill('zach');
	await page.getByRole('button', { name: 'create passkey' }).click();
	await page.waitForURL('**/dashboard');
	await expect(page.getByRole('heading', { name: 'hosts', exact: true })).toBeVisible();

	// The authenticator now holds exactly one (resident) credential.
	const creds = await client.send('WebAuthn.getCredentials', { authenticatorId });
	expect(creds.credentials.length).toBe(1);
	expect(creds.credentials[0].isResidentCredential).toBe(true);

	// The dashboard lists the one enrolled passkey.
	await expect(page.locator('.passkeys__item')).toHaveCount(1);

	// --- logout revokes the session ---
	await page.locator('.logout-btn').first().click();
	await page.waitForURL('**/login');

	// --- usernameless (discoverable) login with the resident passkey ---
	await page.getByRole('button', { name: 'sign in with passkey' }).click();
	await page.waitForURL('**/dashboard');
	await expect(page.getByRole('heading', { name: 'hosts', exact: true })).toBeVisible();

	// --- enroll a second passkey while authenticated ---
	// A real second passkey lives on a different device. The enroll ceremony
	// sends an exclude list for the existing credential, so re-using the first
	// authenticator is (correctly) refused — add a second virtual authenticator
	// to stand in for that other device.
	await client.send('WebAuthn.addVirtualAuthenticator', {
		options: {
			protocol: 'ctap2',
			transport: 'usb', // Chrome allows only one internal authenticator; the 2nd device is cross-platform
			hasResidentKey: true,
			hasUserVerification: true,
			isUserVerified: true,
			automaticPresenceSimulation: true
		}
	});
	await page.getByLabel('new passkey name').fill('Phone');
	await page.getByRole('button', { name: '+ add a passkey' }).click();
	await expect(page.locator('.passkeys__item')).toHaveCount(2);

	// --- removing one of two is allowed ---
	await page.locator('.passkeys__item button', { hasText: 'remove' }).first().click();
	await page.getByRole('dialog').getByRole('button', { name: 'delete' }).click();
	await expect(page.locator('.passkeys__item')).toHaveCount(1);

	// --- removing the last passkey is refused (would lock the account out) ---
	await page.locator('.passkeys__item button', { hasText: 'remove' }).first().click();
	await page.getByRole('dialog').getByRole('button', { name: 'delete' }).click();
	await expect(page.locator('.passkeys .error')).toContainText('last passkey');
	await expect(page.locator('.passkeys__item')).toHaveCount(1);
});
