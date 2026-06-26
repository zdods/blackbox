// Chunked file upload to the file-proxy API. Kept DOM- and store-free: apiFetch
// is injected and progress is reported via an onChunk callback, so the route
// component owns only the UI state, not the transfer mechanics.

export const CHUNK_SIZE = 5 * 1024 * 1024; // 5 MB

export function generateUploadId() {
	return crypto.randomUUID
		? crypto.randomUUID()
		: Math.random().toString(36).slice(2) + Date.now().toString(36);
}

// uploadFile uploads one file to targetPath: a single PUT when it fits in a
// chunk, otherwise a sequence of chunk PUTs sharing one upload_id. onChunk(done,
// total) is called for chunked transfers so callers can render progress. Throws
// an Error (message prefixed with the file name) on any failed request.
export async function uploadFile({ apiFetch, daemonId, file, targetPath, onChunk }) {
	if (file.size <= CHUNK_SIZE) {
		const res = await apiFetch(
			`/api/daemons/${daemonId}/files?path=${encodeURIComponent(targetPath)}`,
			{
				method: 'PUT',
				body: file
			}
		);
		if (!res.ok) {
			const msg = await res.text();
			throw new Error(msg ? `${file.name}: ${msg}` : `Upload failed for ${file.name}`);
		}
		return;
	}

	const totalChunks = Math.ceil(file.size / CHUNK_SIZE);
	const uploadId = generateUploadId();
	onChunk?.(0, totalChunks);

	for (let i = 0; i < totalChunks; i++) {
		const start = i * CHUNK_SIZE;
		const end = Math.min(start + CHUNK_SIZE, file.size);
		const chunk = file.slice(start, end);
		const params = new URLSearchParams({
			path: targetPath,
			upload_id: uploadId,
			chunk_index: String(i),
			total_chunks: String(totalChunks)
		});
		const res = await apiFetch(`/api/daemons/${daemonId}/files?${params}`, {
			method: 'PUT',
			body: chunk
		});
		if (!res.ok) {
			const msg = await res.text();
			throw new Error(
				msg ? `${file.name}: ${msg}` : `Chunk ${i + 1}/${totalChunks} failed for ${file.name}`
			);
		}
		onChunk?.(i + 1, totalChunks);
	}
}
