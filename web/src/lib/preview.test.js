import { describe, it, expect } from 'vitest';
import { getExt, isImageExt, isPreviewable } from './preview.js';

describe('getExt', () => {
	it('returns the lowercased extension', () => {
		expect(getExt('photo.PNG')).toBe('png');
		expect(getExt('archive.tar.gz')).toBe('gz');
	});
	it('treats a leading-dot name as all-extension (dotfile)', () => {
		expect(getExt('.gitignore')).toBe('gitignore');
	});
	it('returns empty string when there is no extension', () => {
		expect(getExt('Makefile')).toBe('');
		expect(getExt('trailingdot.')).toBe('');
	});
});

describe('isImageExt', () => {
	it('recognizes image extensions and rejects others', () => {
		expect(isImageExt('png')).toBe(true);
		expect(isImageExt('svg')).toBe(true);
		expect(isImageExt('txt')).toBe(false);
	});
});

describe('isPreviewable', () => {
	it('never previews directories', () => {
		expect(isPreviewable({ name: 'docs', is_dir: true })).toBe(false);
	});
	it('previews images under the 20MB cap and rejects over it', () => {
		expect(isPreviewable({ name: 'a.png', size: 20 * 1024 * 1024 })).toBe(true);
		expect(isPreviewable({ name: 'a.png', size: 20 * 1024 * 1024 + 1 })).toBe(false);
	});
	it('previews text under the 1MB cap and rejects over it', () => {
		expect(isPreviewable({ name: 'a.md', size: 1024 * 1024 })).toBe(true);
		expect(isPreviewable({ name: 'a.md', size: 1024 * 1024 + 1 })).toBe(false);
	});
	it('allows an unknown (null) size, deferred to load time', () => {
		expect(isPreviewable({ name: 'a.txt', size: null })).toBe(true);
	});
	it('rejects unknown extensions', () => {
		expect(isPreviewable({ name: 'a.bin', size: 10 })).toBe(false);
	});
});
