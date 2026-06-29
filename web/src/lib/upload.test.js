import { describe, it, expect, vi } from 'vitest';
import { uploadFile, generateUploadId, CHUNK_SIZE } from './upload.js';

// fakeFile mimics the File API surface uploadFile touches: name, size, slice.
function fakeFile(name, size) {
	return {
		name,
		size,
		slice(start, end) {
			return { start, end, _isChunk: true };
		}
	};
}

// recordingFetch returns an apiFetch double that records calls and yields a
// configurable response per call.
function recordingFetch(responder = () => ({ ok: true, text: async () => '' })) {
	const calls = [];
	const fn = vi.fn(async (url, opts) => {
		calls.push({ url, opts });
		return responder(calls.length - 1, url, opts);
	});
	return { fn, calls };
}

function parse(url) {
	return new URL(url, 'http://localhost');
}

describe('generateUploadId', () => {
	it('returns a non-empty unique-ish id', () => {
		const a = generateUploadId();
		const b = generateUploadId();
		expect(typeof a).toBe('string');
		expect(a.length).toBeGreaterThan(0);
		expect(a).not.toBe(b);
	});
});

describe('uploadFile — single-shot path', () => {
	it('sends exactly one PUT without an upload_id for a small file', async () => {
		const { fn, calls } = recordingFetch();
		await uploadFile({
			apiFetch: fn,
			daemonId: 'd1',
			file: fakeFile('small.txt', 10),
			targetPath: 'dir/small.txt'
		});
		expect(calls).toHaveLength(1);
		const u = parse(calls[0].url);
		expect(calls[0].opts.method).toBe('PUT');
		expect(u.searchParams.get('path')).toBe('dir/small.txt');
		expect(u.searchParams.has('upload_id')).toBe(false);
	});

	it('throws with the server message prefixed by file name', async () => {
		const { fn } = recordingFetch(() => ({ ok: false, text: async () => 'disk full' }));
		await expect(
			uploadFile({ apiFetch: fn, daemonId: 'd1', file: fakeFile('a.txt', 5), targetPath: 'a.txt' })
		).rejects.toThrow('a.txt: disk full');
	});

	it('throws a generic message when the server body is empty', async () => {
		const { fn } = recordingFetch(() => ({ ok: false, text: async () => '' }));
		await expect(
			uploadFile({ apiFetch: fn, daemonId: 'd1', file: fakeFile('a.txt', 5), targetPath: 'a.txt' })
		).rejects.toThrow('Upload failed for a.txt');
	});
});

describe('uploadFile — chunked path', () => {
	it('splits into ceil(size/CHUNK_SIZE) chunks sharing one upload_id', async () => {
		const { fn, calls } = recordingFetch();
		const size = CHUNK_SIZE * 2 + 1; // 3 chunks
		const onChunk = vi.fn();
		await uploadFile({
			apiFetch: fn,
			daemonId: 'd1',
			file: fakeFile('big.bin', size),
			targetPath: 'big.bin',
			onChunk
		});

		expect(calls).toHaveLength(3);
		const ids = new Set();
		calls.forEach((c, i) => {
			const u = parse(c.url);
			expect(u.searchParams.get('chunk_index')).toBe(String(i));
			expect(u.searchParams.get('total_chunks')).toBe('3');
			ids.add(u.searchParams.get('upload_id'));
		});
		expect(ids.size).toBe(1); // one shared upload_id

		// Correct byte ranges on each slice.
		expect(calls[0].opts.body).toEqual({ start: 0, end: CHUNK_SIZE, _isChunk: true });
		expect(calls[2].opts.body).toEqual({ start: 2 * CHUNK_SIZE, end: size, _isChunk: true });

		// Progress: initial (0,3) then after each chunk up to (3,3).
		expect(onChunk.mock.calls).toEqual([
			[0, 3],
			[1, 3],
			[2, 3],
			[3, 3]
		]);
	});

	it('throws "Chunk i/total failed" when a chunk fails with an empty body', async () => {
		const { fn } = recordingFetch((i) =>
			i === 1 ? { ok: false, text: async () => '' } : { ok: true, text: async () => '' }
		);
		await expect(
			uploadFile({
				apiFetch: fn,
				daemonId: 'd1',
				file: fakeFile('big.bin', CHUNK_SIZE * 2 + 1),
				targetPath: 'big.bin'
			})
		).rejects.toThrow('Chunk 2/3 failed for big.bin');
	});
});
