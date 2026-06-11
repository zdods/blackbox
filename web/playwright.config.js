import { defineConfig } from '@playwright/test';

export default defineConfig({
	testDir: 'e2e',
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 1 : 0,
	reporter: process.env.CI ? 'list' : [['list'], ['html', { open: 'never' }]],
	use: {
		baseURL: 'http://localhost:4173',
		trace: 'on-first-retry'
	},
	projects: [
		{
			name: 'desktop',
			use: { browserName: 'chromium', viewport: { width: 1280, height: 800 } }
		},
		{
			name: 'mobile',
			use: {
				browserName: 'chromium',
				viewport: { width: 375, height: 667 },
				deviceScaleFactor: 2,
				isMobile: true,
				hasTouch: true
			}
		}
	],
	webServer: {
		command: 'npm run preview',
		port: 4173,
		reuseExistingServer: !process.env.CI
	}
});
