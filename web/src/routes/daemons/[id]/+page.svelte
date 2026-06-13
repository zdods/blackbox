<script>
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { getToken, clearToken, apiFetch } from '$lib/auth.js';
	import Face from '$lib/Face.svelte';

	const daemonId = $page.params.id;
	let path = '';
	let entries = [];
	let loading = true;
	let error = '';
	let daemonLabel = '';
	let uploadPath = '';
	let uploading = false;
	let selectedFileName = '';
	let uploadProgress = { current: 0, total: 0, chunkCurrent: 0, chunkTotal: 0 };
	let deletingPath = '';
	let sortBy = 'name'; // 'name' | 'size' | 'mtime'
	let sortDir = 'asc'; // 'asc' | 'desc'

	// Drag-and-drop
	let dragCounter = 0;
	let draggingOver = false;

	// Preview modal
	let previewEntry = null;
	let previewContent = null;
	let previewType = null; // 'image' | 'text'
	let previewLoading = false;
	let previewError = '';

	const IMAGE_EXTS = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'ico', 'avif']);
	const TEXT_EXTS = new Set([
		'txt',
		'md',
		'json',
		'yaml',
		'yml',
		'toml',
		'xml',
		'csv',
		'log',
		'sh',
		'bash',
		'zsh',
		'py',
		'js',
		'ts',
		'jsx',
		'tsx',
		'css',
		'html',
		'htm',
		'sql',
		'go',
		'rs',
		'java',
		'c',
		'cpp',
		'h',
		'rb',
		'php',
		'swift',
		'kt',
		'env',
		'conf',
		'ini',
		'cfg',
		'gitignore',
		'dockerignore'
	]);
	const TEXT_PREVIEW_MAX = 1024 * 1024; // 1 MB
	const IMAGE_PREVIEW_MAX = 20 * 1024 * 1024; // 20 MB

	function getExt(name) {
		const i = name.lastIndexOf('.');
		return i >= 0 ? name.slice(i + 1).toLowerCase() : '';
	}

	function isPreviewable(entry) {
		if (entry.is_dir) return false;
		const ext = getExt(entry.name);
		if (IMAGE_EXTS.has(ext)) return entry.size == null || entry.size <= IMAGE_PREVIEW_MAX;
		if (TEXT_EXTS.has(ext)) return entry.size == null || entry.size <= TEXT_PREVIEW_MAX;
		return false;
	}

	$: pathSegments = path ? path.split('/').filter(Boolean) : [];
	$: sortedEntries = (() => {
		const list = [...entries];
		list.sort((a, b) => {
			let cmp = 0;
			if (sortBy === 'name') {
				const an = (a.name || '').toLowerCase();
				const bn = (b.name || '').toLowerCase();
				cmp = an.localeCompare(bn, undefined, { sensitivity: 'base' });
				if (a.is_dir !== b.is_dir) cmp = a.is_dir ? -1 : 1;
			} else if (sortBy === 'size') {
				const as = a.is_dir ? -1 : (a.size ?? 0);
				const bs = b.is_dir ? -1 : (b.size ?? 0);
				cmp = as - bs;
			} else if (sortBy === 'mtime') {
				const at = a.mtime || '';
				const bt = b.mtime || '';
				cmp = at.localeCompare(bt);
			}
			return sortDir === 'asc' ? cmp : -cmp;
		});
		return list;
	})();

	onMount(() => {
		if (!getToken()) {
			goto('/login');
			return;
		}
		load();
	});

	async function load() {
		loading = true;
		error = '';
		try {
			if (!daemonLabel) {
				const listRes = await apiFetch('/api/daemons');
				if (listRes.ok) {
					const list = await listRes.json();
					const a = list.find((x) => x.id === daemonId);
					if (a) daemonLabel = a.label;
				}
			}
			const q = path ? `?path=${encodeURIComponent(path)}` : '';
			const res = await apiFetch(`/api/daemons/${daemonId}/files${q}`);
			if (res.status === 401) {
				clearToken();
				goto('/login');
				return;
			}
			if (res.status === 503) {
				error = 'host not connected';
				entries = [];
				loading = false;
				return;
			}
			if (!res.ok) throw new Error(await res.text());
			entries = await res.json();
		} catch (e) {
			error = e.message;
			entries = [];
		} finally {
			loading = false;
		}
	}

	function goToSegment(segment) {
		const idx = pathSegments.indexOf(segment);
		path = pathSegments.slice(0, idx + 1).join('/');
		load();
	}

	function goUp() {
		if (pathSegments.length === 0) return;
		path = pathSegments.slice(0, -1).join('/');
		load();
	}

	async function download(entry) {
		const fullPath = path ? `${path}/${entry.name}` : entry.name;
		// Preflight with the metadata endpoint so we silently no-op on error
		// (file gone, daemon offline) instead of navigating to a junk response —
		// preserves the prior behavior without buffering the file to check it.
		const meta = await apiFetch(
			`/api/daemons/${daemonId}/meta?path=${encodeURIComponent(fullPath)}`
		);
		if (!meta.ok) return;
		// Navigate to the download URL via a download anchor so the browser
		// streams the response straight to disk. The same-origin httpOnly session
		// cookie rides along; the server's Content-Disposition supplies the
		// filename. Avoids buffering the whole file in a Blob in tab memory.
		const url = `/api/daemons/${daemonId}/files?path=${encodeURIComponent(fullPath)}&download=1`;
		const a = document.createElement('a');
		a.href = url;
		a.download = entry.name;
		a.rel = 'noopener';
		document.body.appendChild(a);
		a.click();
		a.remove();
	}

	const CHUNK_SIZE = 5 * 1024 * 1024;

	function generateUploadId() {
		return crypto.randomUUID
			? crypto.randomUUID()
			: Math.random().toString(36).slice(2) + Date.now().toString(36);
	}

	async function uploadFile(file, targetPath) {
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
		uploadProgress = { ...uploadProgress, chunkCurrent: 0, chunkTotal: totalChunks };

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
			uploadProgress = { ...uploadProgress, chunkCurrent: i + 1 };
		}
	}

	async function uploadFiles(files) {
		const total = files.length;
		uploading = true;
		uploadProgress = { current: 0, total, chunkCurrent: 0, chunkTotal: 0 };
		error = '';
		try {
			for (let i = 0; i < total; i++) {
				const file = files[i];
				uploadProgress = { ...uploadProgress, current: i + 1, chunkCurrent: 0, chunkTotal: 0 };
				selectedFileName = total > 1 ? `Uploading ${i + 1} of ${total}…` : file.name;
				const subPath = uploadPath ? `${uploadPath}/${file.name}` : file.name;
				const targetPath = path ? `${path}/${subPath}` : subPath;
				await uploadFile(file, targetPath);
			}
			uploadPath = '';
			selectedFileName = '';
			load();
		} catch (err) {
			error = err.message || 'Upload failed';
		} finally {
			uploading = false;
			uploadProgress = { current: 0, total: 0, chunkCurrent: 0, chunkTotal: 0 };
			selectedFileName = '';
		}
	}

	async function handleUpload(e) {
		const files = Array.from(e.target.files || []);
		e.target.value = '';
		if (!files.length) return;
		await uploadFiles(files);
	}

	// Drag-and-drop
	function handleDragEnter(e) {
		if (!e.dataTransfer?.types?.includes('Files')) return;
		e.preventDefault();
		dragCounter++;
		draggingOver = true;
	}

	function handleDragLeave() {
		dragCounter--;
		if (dragCounter === 0) draggingOver = false;
	}

	function handleDragOver(e) {
		if (!e.dataTransfer?.types?.includes('Files')) return;
		e.preventDefault();
	}

	async function handleDrop(e) {
		e.preventDefault();
		dragCounter = 0;
		draggingOver = false;
		if (uploading) return;
		const files = Array.from(e.dataTransfer?.files || []);
		if (!files.length) return;
		await uploadFiles(files);
	}

	// Preview modal
	async function openPreview(entry) {
		const fullPath = path ? `${path}/${entry.name}` : entry.name;
		const ext = getExt(entry.name);
		previewEntry = entry;
		previewContent = null;
		previewType = null;
		previewLoading = true;
		previewError = '';
		try {
			const url = `/api/daemons/${daemonId}/files?path=${encodeURIComponent(fullPath)}&download=1`;
			const res = await apiFetch(url);
			if (!res.ok) throw new Error(await res.text());
			if (IMAGE_EXTS.has(ext)) {
				previewContent = URL.createObjectURL(await res.blob());
				previewType = 'image';
			} else {
				previewContent = await res.text();
				previewType = 'text';
			}
		} catch (e) {
			previewError = e.message;
		} finally {
			previewLoading = false;
		}
	}

	function closePreview() {
		if (previewType === 'image' && previewContent) URL.revokeObjectURL(previewContent);
		previewEntry = null;
		previewContent = null;
		previewType = null;
		previewError = '';
	}

	function setSort(col) {
		if (sortBy === col) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
		else {
			sortBy = col;
			sortDir = 'asc';
		}
	}

	function formatSize(bytes) {
		if (bytes === 0) return '0 B';
		const k = 1024;
		const units = ['B', 'KB', 'MB', 'GB'];
		let i = 0;
		let n = bytes;
		while (n >= k && i < units.length - 1) {
			n /= k;
			i += 1;
		}
		return (i === 0 ? n : n.toFixed(1)) + ' ' + units[i];
	}

	async function deleteEntry(entry) {
		const fullPath = path ? `${path}/${entry.name}` : entry.name;
		if (!confirm(`Delete ${entry.is_dir ? 'directory' : 'file'} "${entry.name}"?`)) return;
		deletingPath = fullPath;
		error = '';
		try {
			const res = await apiFetch(
				`/api/daemons/${daemonId}/files?path=${encodeURIComponent(fullPath)}`,
				{
					method: 'DELETE'
				}
			);
			if (!res.ok) throw new Error(await res.text());
			load();
		} catch (err) {
			error = err.message;
		} finally {
			deletingPath = '';
		}
	}
</script>

<svelte:window
	on:keydown={(e) => {
		if (e.key === 'Escape' && previewEntry) closePreview();
	}}
/>

<!-- svelte-ignore a11y-no-static-element-interactions -->
<div
	class="files-page"
	on:dragenter={handleDragEnter}
	on:dragleave={handleDragLeave}
	on:dragover={handleDragOver}
	on:drop={handleDrop}
>
	<p class="back-link"><a href="/dashboard">← hosts</a></p>
	<header class="page-header">
		<h1 class="page-title">
			{daemonLabel || 'files'}
		</h1>
		<p class="page-sub">Browse, upload, and download files on this host.</p>
	</header>

	<nav class="breadcrumb" aria-label="file path">
		<button
			type="button"
			class="crumb"
			on:click={() => {
				path = '';
				load();
			}}>root</button
		>
		{#each pathSegments as segment}
			<span class="breadcrumb-sep" aria-hidden="true">/</span>
			<button type="button" class="crumb" on:click={() => goToSegment(segment)}>{segment}</button>
		{/each}
	</nav>

	{#if error}<p class="error" role="alert"><Face state="error" /> {error}</p>{/if}

	{#if loading}
		<p class="muted" role="status" aria-live="polite"><Face state="loading" /> loading…</p>
	{:else}
		<div class="card file-list-wrap" class:drop-target={draggingOver}>
			{#if draggingOver}
				<div class="drop-overlay" aria-hidden="true">drop files to upload</div>
			{/if}
			<table class="file-list">
				<thead>
					<tr>
						<th
							scope="col"
							class="col-name sortable"
							class:sort-asc={sortBy === 'name' && sortDir === 'asc'}
							class:sort-desc={sortBy === 'name' && sortDir === 'desc'}
						>
							<button
								type="button"
								class="th-sort"
								on:click={() => setSort('name')}
								aria-label="sort by name{sortBy === 'name'
									? sortDir === 'asc'
										? ', sorted ascending'
										: ', sorted descending'
									: ''}">name</button
							>
						</th>
						<th
							scope="col"
							class="col-size sortable"
							class:sort-asc={sortBy === 'size' && sortDir === 'asc'}
							class:sort-desc={sortBy === 'size' && sortDir === 'desc'}
						>
							<button
								type="button"
								class="th-sort"
								on:click={() => setSort('size')}
								aria-label="sort by size{sortBy === 'size'
									? sortDir === 'asc'
										? ', sorted ascending'
										: ', sorted descending'
									: ''}">size</button
							>
						</th>
						<th
							scope="col"
							class="col-mtime sortable"
							class:sort-asc={sortBy === 'mtime' && sortDir === 'asc'}
							class:sort-desc={sortBy === 'mtime' && sortDir === 'desc'}
						>
							<button
								type="button"
								class="th-sort"
								on:click={() => setSort('mtime')}
								aria-label="sort by modified{sortBy === 'mtime'
									? sortDir === 'asc'
										? ', sorted ascending'
										: ', sorted descending'
									: ''}">modified</button
							>
						</th>
						<th scope="col" class="col-actions"></th>
					</tr>
				</thead>
				<tbody>
					{#if pathSegments.length > 0}
						<tr>
							<td colspan="4"
								><button
									type="button"
									class="link"
									on:click={goUp}
									aria-label="go to parent directory">..</button
								></td
							>
						</tr>
					{/if}
					{#each sortedEntries as entry}
						<tr>
							<td class="col-name">
								{#if entry.is_dir}
									<button
										type="button"
										class="link dir"
										on:click={() => {
											path = path ? `${path}/${entry.name}` : entry.name;
											load();
										}}>{entry.name}/</button
									>
								{:else if isPreviewable(entry)}
									<button class="link" on:click={() => openPreview(entry)}>{entry.name}</button>
								{:else}
									<button class="link" on:click={() => download(entry)}>{entry.name}</button>
								{/if}
							</td>
							<td class="col-size">{entry.is_dir ? '—' : formatSize(entry.size)}</td>
							<td class="col-mtime">{entry.mtime || '—'}</td>
							<td class="col-actions">
								{#if !entry.is_dir}
									<button
										type="button"
										class="link dl-btn"
										on:click={() => download(entry)}
										title="download"
										aria-label="download {entry.name}">↓</button
									>
								{/if}
								<button
									type="button"
									class="link delete-btn"
									on:click={() => deleteEntry(entry)}
									disabled={deletingPath !== ''}
									title="delete"
									aria-label="delete {entry.name}">delete</button
								>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<div class="upload">
			<div class="upload-row">
				<span class="upload-label microlabel">upload</span>
				<input
					type="text"
					name="upload-subpath"
					autocomplete="off"
					aria-label="optional upload subpath"
					bind:value={uploadPath}
					placeholder="optional subpath"
					class="upload-path"
				/>
				<label class="upload-file-wrap">
					<input
						type="file"
						multiple
						on:change={handleUpload}
						disabled={uploading}
						class="upload-file-input"
						aria-label="choose files to upload"
					/>
					<span class="upload-file-text" role="status" aria-live="polite">
						{#if uploading && uploadProgress.total > 0}
							uploading {uploadProgress.current} of {uploadProgress.total}{#if uploadProgress.chunkTotal > 0}
								(chunk {uploadProgress.chunkCurrent}/{uploadProgress.chunkTotal}){/if}…
						{:else}
							{selectedFileName || 'choose files or drag & drop…'}
						{/if}
					</span>
				</label>
			</div>
		</div>
	{/if}
</div>

{#if previewEntry}
	<!-- svelte-ignore a11y-click-events-have-key-events -->
	<!-- svelte-ignore a11y-no-static-element-interactions -->
	<div class="preview-backdrop" on:click={closePreview}>
		<!-- svelte-ignore a11y-click-events-have-key-events -->
		<!-- svelte-ignore a11y-no-static-element-interactions -->
		<div
			class="preview-modal"
			role="dialog"
			tabindex="-1"
			aria-modal="true"
			aria-label="preview {previewEntry.name}"
			on:click|stopPropagation
		>
			<div class="preview-header">
				<span class="preview-filename">{previewEntry.name}</span>
				<div class="preview-actions">
					<button type="button" class="secondary" on:click={() => download(previewEntry)}
						>download</button
					>
					<button type="button" class="secondary" on:click={closePreview}>close</button>
				</div>
			</div>
			<div class="preview-body">
				{#if previewLoading}
					<p class="muted" role="status"><Face state="loading" /> loading…</p>
				{:else if previewError}
					<p class="error" role="alert"><Face state="error" /> {previewError}</p>
				{:else if previewType === 'image'}
					<img src={previewContent} alt={previewEntry.name} class="preview-image" />
				{:else if previewType === 'text'}
					<pre class="preview-text">{previewContent}</pre>
				{/if}
			</div>
		</div>
	</div>
{/if}

<style>
	.back-link {
		margin: 0 0 var(--space-sm);
		font-size: 0.8rem;
	}
	.back-link a {
		color: var(--text-muted);
		text-decoration: none;
	}
	.back-link a:hover {
		color: var(--accent);
	}
	.page-header {
		margin-bottom: var(--space-lg);
	}
	.breadcrumb {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-xs);
		margin: 0 0 var(--space-md);
		font-size: 0.85rem;
	}
	.crumb {
		background: none;
		border: none;
		height: auto;
		padding: var(--space-xs) var(--space-sm);
		color: var(--text-muted);
		font-size: 0.85rem;
		border-radius: var(--radius-sm);
	}
	.crumb:hover {
		color: var(--accent);
		background: var(--accent-soft);
		border: none;
	}
	.breadcrumb-sep {
		color: var(--text-faint);
	}

	.file-list-wrap {
		position: relative;
		overflow-y: auto;
		min-height: 120px;
		max-height: min(58vh, 480px);
		margin-bottom: var(--space-lg);
		transition:
			border-color 0.15s,
			box-shadow 0.15s;
	}
	.file-list-wrap.drop-target {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-soft);
	}
	.drop-overlay {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--backdrop);
		color: var(--accent);
		font-size: 0.95rem;
		font-weight: 600;
		letter-spacing: 0.05em;
		z-index: 10;
		border-radius: var(--radius);
		pointer-events: none;
	}
	.file-list {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.875rem;
		margin: 0;
	}
	.file-list th {
		position: sticky;
		top: 0;
		z-index: 2;
		background: var(--surface);
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--border);
		font-size: 0.7rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--text-faint);
	}
	.file-list td {
		padding: var(--space-xs) var(--space-md);
		border-bottom: 1px solid var(--border);
		vertical-align: middle;
	}
	.file-list tbody tr:last-child td {
		border-bottom: none;
	}
	.file-list tbody tr:hover td {
		background: var(--accent-soft);
	}
	.col-name {
		min-width: 0;
	}
	.file-list th.col-name,
	.file-list td.col-name {
		text-align: left;
	}
	.col-size {
		width: 7rem;
		color: var(--text-muted);
	}
	.file-list th.col-size,
	.file-list td.col-size {
		text-align: right;
	}
	.col-mtime {
		width: 12rem;
		color: var(--text-muted);
		font-size: 0.8rem;
	}
	.file-list th.col-mtime,
	.file-list td.col-mtime {
		text-align: right;
	}
	.col-actions {
		width: 8rem;
	}
	.file-list th.col-actions,
	.file-list td.col-actions {
		text-align: center;
	}
	.file-list th.sortable .th-sort {
		background: none;
		border: none;
		color: inherit;
		font: inherit;
		letter-spacing: inherit;
		text-transform: inherit;
		cursor: pointer;
		padding: 0;
		height: auto;
		width: 100%;
		text-align: inherit;
	}
	.file-list th.sortable .th-sort:hover {
		color: var(--text);
	}
	.file-list th.sort-asc .th-sort::after {
		content: ' ↑';
		color: var(--accent);
	}
	.file-list th.sort-desc .th-sort::after {
		content: ' ↓';
		color: var(--accent);
	}
	.file-list .link {
		display: inline-block;
		background: none;
		border: none;
		height: auto;
		color: var(--text);
		cursor: pointer;
		padding: var(--space-sm) 0;
		text-align: left;
		font-family: var(--font-mono);
		font-size: inherit;
	}
	.file-list .link:hover {
		color: var(--accent);
	}
	.file-list .link.dir {
		color: var(--accent);
		font-weight: 500;
	}
	.file-list .link.dir:hover {
		text-decoration: underline;
		text-underline-offset: 3px;
	}
	.file-list .dl-btn {
		color: var(--text-muted);
		padding: var(--space-sm);
	}
	.file-list .dl-btn:hover {
		color: var(--accent);
	}
	.file-list .delete-btn {
		color: var(--text-faint);
		font-size: 0.8rem;
		padding: var(--space-sm);
	}
	.file-list .delete-btn:hover:not(:disabled) {
		color: var(--err);
	}

	.upload {
		margin-top: var(--space-lg);
	}
	.upload-row {
		display: flex;
		align-items: center;
		gap: var(--space-md);
		width: 100%;
	}
	.upload-label {
		flex-shrink: 0;
	}
	.upload-path {
		flex: 1;
		min-width: 0;
	}
	.upload-file-wrap {
		display: flex;
		align-items: center;
		flex: 0 0 auto;
		min-width: 14rem;
		height: var(--input-height);
		background: var(--inset);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		cursor: pointer;
		position: relative;
		box-sizing: border-box;
		transition: border-color 0.15s;
	}
	.upload-file-wrap:hover {
		border-color: var(--accent);
	}
	.upload-file-input {
		position: absolute;
		inset: 0;
		opacity: 0;
		cursor: pointer;
		width: 100%;
		height: 100%;
		margin: 0;
		padding: 0;
	}
	.upload-file-text {
		padding: 0 var(--space-md);
		font-size: 0.8rem;
		color: var(--text-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		pointer-events: none;
	}

	/* Preview modal */
	.preview-backdrop {
		position: fixed;
		inset: 0;
		background: var(--backdrop);
		z-index: 200;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-lg);
	}
	.preview-modal {
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		width: min(90vw, 56rem);
		max-height: 85vh;
		display: flex;
		flex-direction: column;
		overflow: hidden;
		box-shadow: var(--shadow);
	}
	.preview-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-md) var(--space-lg);
		border-bottom: 1px solid var(--border);
		gap: var(--space-md);
		flex-shrink: 0;
	}
	.preview-filename {
		font-size: 0.875rem;
		font-weight: 500;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.preview-actions {
		display: flex;
		gap: var(--space-sm);
		flex-shrink: 0;
	}
	.preview-actions button {
		height: 2rem;
		font-size: 0.8rem;
		padding: 0 var(--space-md);
	}
	.preview-body {
		flex: 1;
		overflow: auto;
		padding: var(--space-lg);
		min-height: 0;
	}
	.preview-image {
		max-width: 100%;
		display: block;
		margin: 0 auto;
	}
	.preview-text {
		font-family: var(--font-mono);
		font-size: 0.8rem;
		line-height: 1.6;
		color: var(--text);
		white-space: pre-wrap;
		word-break: break-all;
		margin: 0;
		tab-size: 2;
	}

	/* Mobile: drop the modified column, tighten paddings, wrap the upload row */
	@media (max-width: 640px) {
		.file-list th.col-mtime,
		.file-list td.col-mtime {
			display: none;
		}
		.col-size {
			width: 4.5rem;
		}
		.col-actions {
			width: 4.5rem;
		}
		.file-list th,
		.file-list td {
			padding: var(--space-sm);
		}
		/* Wrap long names, but never mid-word on the action buttons */
		.file-list td.col-name .link {
			word-break: break-all;
		}
		.file-list .dl-btn,
		.file-list .delete-btn {
			white-space: nowrap;
		}
		.upload-row {
			flex-wrap: wrap;
		}
		.upload-path {
			flex-basis: 100%;
		}
		.upload-file-wrap {
			min-width: 0;
			flex: 1 1 auto;
		}
		.preview-backdrop {
			padding: var(--space-sm);
		}
		.preview-modal {
			width: 100%;
			max-height: 92vh;
		}
	}
</style>
