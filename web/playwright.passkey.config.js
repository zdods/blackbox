import { defineConfig } from '@playwright/test';

// Passkey end-to-end tests run against a real, already-running bastion in
// AUTH_MODE=passkey (not the static `npm run preview` server), driving WebAuthn
// via a Chrome DevTools virtual authenticator. Point E2E_BASE_URL at that
// server (default http://localhost:8090). There is no managed webServer here —
// the Go backend is started out of band.
export default defineConfig({
	testDir: 'e2e-passkey',
	fullyParallel: false,
	retries: 0,
	reporter: [['list']],
	timeout: 30000,
	use: {
		baseURL: process.env.E2E_BASE_URL || 'http://localhost:8090',
		trace: 'retain-on-failure'
	},
	projects: [{ name: 'chromium', use: { browserName: 'chromium' } }]
});
