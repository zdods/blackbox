import { describe, it, expect } from 'vitest';
import { scaleBytes, formatBytes, formatDate } from './format.js';

describe('scaleBytes', () => {
	it('keeps small counts in bytes and marks them exact', () => {
		expect(scaleBytes(0)).toEqual({ value: 0, unit: 'B', exact: true });
		expect(scaleBytes(512)).toEqual({ value: 512, unit: 'B', exact: true });
	});
	it('steps up by 1024 at each boundary', () => {
		expect(scaleBytes(1024)).toEqual({ value: 1, unit: 'KB', exact: false });
		expect(scaleBytes(1024 * 1024)).toEqual({ value: 1, unit: 'MB', exact: false });
		expect(scaleBytes(1024 ** 3)).toEqual({ value: 1, unit: 'GB', exact: false });
	});
	it('caps at TB and does not overflow the unit table', () => {
		const r = scaleBytes(1024 ** 5); // beyond TB
		expect(r.unit).toBe('TB');
		expect(r.value).toBe(1024);
	});
});

describe('formatBytes', () => {
	it('renders whole bytes without decimals', () => {
		expect(formatBytes(0)).toBe('0 B');
		expect(formatBytes(999)).toBe('999 B');
	});
	it('renders one decimal for scaled units', () => {
		expect(formatBytes(1536)).toBe('1.5 KB');
		expect(formatBytes(5 * 1024 * 1024)).toBe('5.0 MB');
	});
	it('renders an em dash for unknown/negative/NaN', () => {
		expect(formatBytes(null)).toBe('—');
		expect(formatBytes(undefined)).toBe('—');
		expect(formatBytes(-1)).toBe('—');
		expect(formatBytes(NaN)).toBe('—');
	});
});

describe('formatDate', () => {
	it('returns empty string for missing or unparseable input', () => {
		expect(formatDate('')).toBe('');
		expect(formatDate(null)).toBe('');
		expect(formatDate('not-a-date')).toBe('');
	});
	it('formats a valid ISO timestamp to a non-empty localized string', () => {
		const out = formatDate('2026-03-03T12:00:00Z');
		expect(out).not.toBe('');
		expect(out).toContain('2026');
	});
});
