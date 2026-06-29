import { describe, it, expect } from 'vitest';
import { gravatarURL } from './account.js';

// SHA-256 of "test@example.com" (precomputed) — gravatarURL hashes the trimmed,
// lowercased address with Web Crypto.
const HASH = '973dfe463ec85785f5f95af5ba3906eedb2d931c24e69824a89ea65dba4e813b';

describe('gravatarURL', () => {
	it('builds a deterministic URL from the SHA-256 of the normalized email', async () => {
		const url = await gravatarURL('  TEST@Example.com ', 64);
		expect(url).toBe(`https://www.gravatar.com/avatar/${HASH}?d=identicon&s=64`);
	});

	it('defaults the size to 160', async () => {
		const url = await gravatarURL('test@example.com');
		expect(url).toContain('s=160');
	});

	it('returns null for an empty or missing email', async () => {
		expect(await gravatarURL('')).toBeNull();
		expect(await gravatarURL(null)).toBeNull();
		expect(await gravatarURL('   ')).toBeNull();
	});
});
