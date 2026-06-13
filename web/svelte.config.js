import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: 'index.html',
			precompress: false,
			strict: false
		}),
		// Content-Security-Policy delivered as a <meta> tag in the prerendered
		// HTML (adapter-static has no server to set a header). 'hash' mode emits
		// hashes for the inline scripts SvelteKit controls — including the theme
		// bootstrap in app.html — so script execution stays locked to 'self' plus
		// those hashes. style-src keeps 'unsafe-inline' because Svelte emits inline
		// style attributes; styles are a far smaller XSS risk than scripts.
		csp: {
			mode: 'hash',
			directives: {
				'default-src': ['self'],
				'script-src': ['self'],
				'style-src': ['self', 'unsafe-inline'],
				'img-src': ['self', 'blob:', 'data:'],
				'font-src': ['self'],
				'connect-src': ['self'],
				'object-src': ['none'],
				'base-uri': ['none'],
				'frame-ancestors': ['none']
			}
		}
	}
};

export default config;
