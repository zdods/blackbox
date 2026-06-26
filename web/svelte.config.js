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
				// www.gravatar.com is allowed so the account screen can render the
				// user's Gravatar avatar (opt-in — only fetched once an email is
				// set; Gravatar receives only a one-way SHA-256 hash of it).
				'img-src': ['self', 'blob:', 'data:', 'https://www.gravatar.com'],
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
