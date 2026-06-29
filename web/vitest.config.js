import { defineConfig } from 'vitest/config';
import { fileURLToPath } from 'node:url';

// Standalone Vitest config (no SvelteKit plugin) for unit-testing the pure
// $lib modules. SvelteKit's `$lib` and `$app/*` imports are resolved via aliases
// — `$app/navigation` points at a tiny stub so modules that import `goto` load
// outside the SvelteKit runtime.
export default defineConfig({
	resolve: {
		alias: {
			$lib: fileURLToPath(new URL('./src/lib', import.meta.url)),
			'$app/navigation': fileURLToPath(
				new URL('./src/test-stubs/app-navigation.js', import.meta.url)
			)
		}
	},
	test: {
		environment: 'jsdom',
		include: ['src/**/*.test.js'],
		clearMocks: true
	}
});
