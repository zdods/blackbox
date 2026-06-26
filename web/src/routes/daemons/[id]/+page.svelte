<script>
	import { onMount, onDestroy, tick } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { isLoggedIn, clearLoggedIn, apiFetch } from '$lib/auth.js';
	import { hosts } from '$lib/hosts.js';
	import { registerActions } from '$lib/palette-actions.js';
	import Face from '$lib/Face.svelte';
	import FileIcon from '$lib/FileIcon.svelte';
	import ViewToggle from '$lib/ViewToggle.svelte';
	import Checkbox from '$lib/Checkbox.svelte';
	import SelectionBar from '$lib/SelectionBar.svelte';
	import ContextMenu from '$lib/ContextMenu.svelte';
	import Skeleton from '$lib/Skeleton.svelte';
	import { confirmDelete } from '$lib/ConfirmDialog.svelte';

	const daemonId = $page.params.id;
	let path = '';
	let entries = [];
	let loading = true;
	let error = '';
	let offline = false;
	// Resolve the host label reactively from the shared hosts store (the layout
	// owns the 8s poll). Falls back to the id until the store first populates.
	$: daemonLabel = $hosts.find((h) => h.id === daemonId)?.label || daemonId;
	let uploadPath = '';
	let uploading = false;
	let selectedFileName = '';
	let uploadProgress = { current: 0, total: 0, chunkCurrent: 0, chunkTotal: 0 };
	// Generalized from a single path to a Set so bulk delete can disable many rows.
	let deleting = new Set();
	let sortBy = 'name'; // 'name' | 'size' | 'mtime'
	let sortDir = 'asc'; // 'asc' | 'desc'

	// View (list | grid) — ViewToggle persists this to localStorage itself.
	let fileView = 'list';

	// Multi-select. Keyed by full path so a selection survives sorting; reset on
	// every directory change. Files only (dirs and `..` are never selectable).
	let selected = new Set();
	let lastAnchorIndex = -1; // last checkbox/row toggled, for shift-range select.
	let listEl; // the scrolling pane, for sticky-shadow + Cmd/Ctrl+A focus scoping.
	let scrolled = false; // drives the sticky-header scroll shadow.

	// Context menu state.
	let ctxOpen = false;
	let ctxX = 0;
	let ctxY = 0;
	let ctxItems = [];
	let ctxSheet = false;
	let ctxEntry = null; // entry the menu was opened on (null when acting on selection)

	// File input element (for the palette "upload files…" action).
	let fileInput;

	// Toast (reuse the global .toast classes).
	let toast = { show: false, message: '', type: 'success' };
	let toastTimeout;

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

	function fullPathOf(entry) {
		return path ? `${path}/${entry.name}` : entry.name;
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

	// Selectable (file) entries in display order — the universe for shift-range
	// and select-all. Dirs are excluded.
	$: selectableEntries = sortedEntries.filter((e) => !e.is_dir);
	$: allSelected =
		selectableEntries.length > 0 && selectableEntries.every((e) => selected.has(fullPathOf(e)));
	$: someSelected = selectableEntries.some((e) => selected.has(fullPathOf(e)));
	$: headerIndeterminate = someSelected && !allSelected;

	onMount(() => {
		if (!isLoggedIn()) {
			goto('/login');
			return;
		}
		load();

		// Register route-contextual command-palette actions.
		const off = registerActions([
			{
				id: 'fb-upload',
				label: 'Upload files…',
				hint: 'this host',
				run: () => fileInput && fileInput.click()
			},
			{
				id: 'fb-refresh',
				label: 'Refresh files',
				hint: 'this host',
				run: () => load()
			},
			{
				id: 'fb-go-up',
				label: 'Go up a directory',
				when: () => pathSegments.length > 0,
				run: goUp
			},
			{
				id: 'fb-toggle-view',
				label: 'Toggle list/grid view',
				run: () => setView(fileView === 'list' ? 'grid' : 'list')
			},
			{
				id: 'fb-copy-path',
				label: 'Copy current path',
				run: () => copyPath(path || '~')
			},
			{
				id: 'fb-select-all',
				label: 'Select all files',
				when: () => selectableEntries.length > 0,
				run: selectAll
			},
			{
				id: 'fb-clear-selection',
				label: 'Clear selection',
				when: () => selected.size > 0,
				run: clearSelection
			},
			{
				id: 'fb-download-selected',
				label: 'Download selected',
				when: () => selected.size > 0,
				run: bulkDownload
			},
			{
				id: 'fb-delete-selected',
				label: 'Delete selected',
				when: () => selected.size > 0,
				run: bulkDelete
			}
		]);
		return off;
	});

	function showToast(message, type = 'success') {
		toast = { show: true, message, type };
		clearTimeout(toastTimeout);
		toastTimeout = setTimeout(() => (toast = { ...toast, show: false }), 3500);
	}

	async function load() {
		loading = true;
		error = '';
		offline = false;
		try {
			const q = path ? `?path=${encodeURIComponent(path)}` : '';
			const res = await apiFetch(`/api/daemons/${daemonId}/files${q}`);
			if (res.status === 401) {
				clearLoggedIn();
				goto('/login');
				return;
			}
			if (res.status === 503) {
				offline = true;
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

	// Reset selection + sort anchor whenever the directory changes.
	function changeDir(next) {
		path = next;
		selected = new Set();
		lastAnchorIndex = -1;
		closeContextMenu();
		load();
	}

	function goToSegment(idx) {
		changeDir(pathSegments.slice(0, idx + 1).join('/'));
	}

	function goUp() {
		if (pathSegments.length === 0) return;
		changeDir(pathSegments.slice(0, -1).join('/'));
	}

	function openDir(entry) {
		changeDir(path ? `${path}/${entry.name}` : entry.name);
	}

	function onScroll() {
		scrolled = !!(listEl && listEl.scrollTop > 0);
	}

	// ---- Selection -----------------------------------------------------
	function toggleOne(entry) {
		const p = fullPathOf(entry);
		const next = new Set(selected);
		if (next.has(p)) next.delete(p);
		else next.add(p);
		selected = next;
	}

	function selectRange(toIndex) {
		// Inclusive range over selectableEntries from the last anchor to toIndex.
		const from = lastAnchorIndex >= 0 ? lastAnchorIndex : toIndex;
		const lo = Math.min(from, toIndex);
		const hi = Math.max(from, toIndex);
		const next = new Set(selected);
		for (let i = lo; i <= hi; i++) {
			const e = selectableEntries[i];
			if (e) next.add(fullPathOf(e));
		}
		selected = next;
	}

	// Modifier-aware selection off a checkbox/row interaction. `e` is the native
	// event (carries shiftKey/metaKey/ctrlKey). `index` is the position within
	// selectableEntries.
	function handleSelect(entry, index, e) {
		if (e && e.shiftKey && lastAnchorIndex >= 0) {
			selectRange(index);
		} else if (e && (e.metaKey || e.ctrlKey)) {
			toggleOne(entry);
			lastAnchorIndex = index;
		} else {
			toggleOne(entry);
			lastAnchorIndex = index;
		}
	}

	function selectAll() {
		const next = new Set(selected);
		for (const e of selectableEntries) next.add(fullPathOf(e));
		selected = next;
	}

	function toggleSelectAll() {
		if (allSelected) {
			selected = new Set();
		} else {
			selectAll();
		}
		lastAnchorIndex = -1;
	}

	function clearSelection() {
		selected = new Set();
		lastAnchorIndex = -1;
	}

	// Entry objects for the current selection (filtered to what's still present).
	$: selectedEntries = sortedEntries.filter((e) => !e.is_dir && selected.has(fullPathOf(e)));

	// ---- Download ------------------------------------------------------
	async function download(entry) {
		const fullPath = fullPathOf(entry);
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

	// Bulk download = sequential anchor clicks (~200ms stagger), reusing the
	// per-file streaming download. NEVER fetch+blob.
	async function bulkDownload() {
		const list = selectedEntries;
		if (!list.length) return;
		showToast(`downloading ${list.length}…`, 'success');
		for (let i = 0; i < list.length; i++) {
			await download(list[i]);
			if (i < list.length - 1) await new Promise((r) => setTimeout(r, 200));
		}
	}

	async function copyPath(p) {
		try {
			await navigator.clipboard.writeText(p);
			showToast('path copied', 'success');
		} catch {
			showToast('copy failed', 'error');
		}
	}

	// ---- Upload --------------------------------------------------------
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

	// Revoke any open image preview object URL when leaving the page, so a blob
	// isn't leaked for the tab's lifetime on client navigation.
	onDestroy(() => {
		if (previewType === 'image' && previewContent) URL.revokeObjectURL(previewContent);
		clearTimeout(toastTimeout);
	});

	// ---- Preview modal -------------------------------------------------
	let previewEl;
	let previewPrevFocus = null;
	const PREVIEW_FOCUSABLE =
		'a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])';

	// Move focus into the preview when it opens; restore it on close.
	$: if (previewEntry) focusPreview();
	async function focusPreview() {
		await tick();
		if (previewEl) previewEl.focus();
	}
	function trapPreviewFocus(e) {
		if (!previewEl) return;
		const focusable = previewEl.querySelectorAll(PREVIEW_FOCUSABLE);
		if (focusable.length === 0) return;
		const first = focusable[0];
		const last = focusable[focusable.length - 1];
		const active = document.activeElement;
		if (e.shiftKey && (active === first || active === previewEl)) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && active === last) {
			e.preventDefault();
			first.focus();
		}
	}

	async function openPreview(entry) {
		const fullPath = fullPathOf(entry);
		const ext = getExt(entry.name);
		previewPrevFocus = typeof document !== 'undefined' ? document.activeElement : null;
		// Revoke the previous image URL before replacing it.
		if (previewType === 'image' && previewContent) URL.revokeObjectURL(previewContent);
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
		if (previewPrevFocus && typeof previewPrevFocus.focus === 'function') previewPrevFocus.focus();
		previewPrevFocus = null;
	}

	// Esc routing: only the preview consumes Esc here (the layout's Esc-router
	// deliberately doesn't touch the preview). The ContextMenu and ConfirmDialog
	// own their own Esc handling.
	function onWindowKeydown(e) {
		if (e.key === 'Escape') {
			// The context menu owns Esc while it's open (window-level listeners on
			// the same target both fire, so guard here rather than rely on
			// stopPropagation) — one Esc should close one surface.
			if (ctxOpen) return;
			if (previewEntry) {
				closePreview();
				return;
			}
		}
		if (e.key === 'Tab' && previewEntry) {
			trapPreviewFocus(e);
			return;
		}
		// Cmd/Ctrl+A selects all files when focus is inside the list pane and we're
		// not typing in a field.
		if ((e.metaKey || e.ctrlKey) && (e.key === 'a' || e.key === 'A')) {
			const t = e.target;
			const typing =
				t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable);
			if (!typing && listEl && listEl.contains(document.activeElement)) {
				e.preventDefault();
				selectAll();
			}
		}
	}

	function setSort(col) {
		if (sortBy === col) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
		else {
			sortBy = col;
			sortDir = 'asc';
		}
	}

	function setView(v) {
		// ViewToggle owns persistence (it writes blackhaul_fileview on change);
		// here we only mirror the value into local state.
		fileView = v;
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

	// Friendly modified date (e.g. "Mar 3, 2026"); the raw ISO stays in a
	// title tooltip. Falls back to the raw string if it can't be parsed.
	function formatMtime(iso) {
		if (!iso) return '—';
		const d = new Date(iso);
		if (isNaN(d.getTime())) return iso;
		return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
	}

	// ---- Delete (single + bulk) via themed ConfirmDialog ---------------
	async function deleteEntry(entry) {
		const fullPath = fullPathOf(entry);
		if (deleting.has(fullPath)) return; // a DELETE for this path is already in flight
		const ok = await confirmDelete(`${entry.is_dir ? 'directory' : 'file'} "${entry.name}"`, {
			danger: true
		});
		if (!ok) return;
		const next = new Set(deleting);
		next.add(fullPath);
		deleting = next;
		error = '';
		try {
			const res = await apiFetch(
				`/api/daemons/${daemonId}/files?path=${encodeURIComponent(fullPath)}`,
				{ method: 'DELETE' }
			);
			if (!res.ok) throw new Error(await res.text());
			const sel = new Set(selected);
			sel.delete(fullPath);
			selected = sel;
			load();
		} catch (err) {
			error = err.message;
		} finally {
			const done = new Set(deleting);
			done.delete(fullPath);
			deleting = done;
		}
	}

	async function bulkDelete() {
		const list = selectedEntries;
		if (!list.length) return;
		if (list.some((e) => deleting.has(fullPathOf(e)))) return; // bulk delete already running
		const sample = list
			.slice(0, 3)
			.map((e) => e.name)
			.join(', ');
		const more = list.length > 3 ? `, +${list.length - 3} more` : '';
		// The shared ConfirmDialog helper renders only the message (its extra-body
		// slot can't be passed through the module helper), so fold the sample names
		// into the label to satisfy the "show sample names" contract.
		const ok = await confirmDelete(
			`${list.length} file${list.length === 1 ? '' : 's'} (${sample}${more})`,
			{ danger: true, confirmLabel: `delete ${list.length}` }
		);
		if (!ok) return;
		const paths = list.map((e) => fullPathOf(e));
		const next = new Set(deleting);
		for (const p of paths) next.add(p);
		deleting = next;
		error = '';
		let failed = 0;
		for (const p of paths) {
			try {
				const res = await apiFetch(`/api/daemons/${daemonId}/files?path=${encodeURIComponent(p)}`, {
					method: 'DELETE'
				});
				if (!res.ok) failed++;
			} catch {
				failed++;
			}
		}
		deleting = new Set();
		clearSelection();
		if (failed) showToast(`${failed} of ${paths.length} failed to delete`, 'error');
		else showToast(`${paths.length} deleted`, 'success');
		load();
	}

	// ---- Context menu --------------------------------------------------
	function buildMenuItems(entry) {
		// If the right-clicked row is part of a multi-selection, act on the whole
		// selection; otherwise act on the single row.
		const inSelection = entry && !entry.is_dir && selected.has(fullPathOf(entry));
		const n = selectedEntries.length;
		if (inSelection && n > 1) {
			return [
				{ id: 'download', label: `download (${n})`, icon: '↓' },
				{ id: 'copy', label: 'copy path', icon: '⧉' },
				{ divider: true },
				{ id: 'delete', label: `delete (${n})`, icon: '✕', danger: true }
			];
		}
		if (entry.is_dir) {
			return [
				{ id: 'open', label: 'open', icon: '▸' },
				{ id: 'copy', label: 'copy path', icon: '⧉' },
				{ divider: true },
				{ id: 'delete', label: 'delete', icon: '✕', danger: true }
			];
		}
		const items = [{ id: 'download', label: 'download', icon: '↓' }];
		if (isPreviewable(entry)) items.push({ id: 'preview', label: 'preview', icon: '◎' });
		items.push({ id: 'copy', label: 'copy path', icon: '⧉' });
		items.push({ divider: true });
		items.push({ id: 'delete', label: 'delete', icon: '✕', danger: true });
		return items;
	}

	function openContextMenu(entry, ev, fromKebab) {
		ctxEntry = entry;
		ctxItems = buildMenuItems(entry);
		ctxSheet = fromKebab && window.matchMedia('(max-width: 640px)').matches;
		if (ev && typeof ev.clientX === 'number' && !ctxSheet) {
			ctxX = ev.clientX;
			ctxY = ev.clientY;
		} else if (ev && ev.currentTarget && !ctxSheet) {
			const r = ev.currentTarget.getBoundingClientRect();
			ctxX = r.left;
			ctxY = r.bottom;
		}
		ctxOpen = true;
	}

	function onRowContextMenu(entry, ev) {
		ev.preventDefault();
		openContextMenu(entry, ev, false);
	}

	function closeContextMenu() {
		ctxOpen = false;
		ctxEntry = null;
	}

	function onMenuSelect(e) {
		const id = e.detail.id;
		const entry = ctxEntry;
		closeContextMenu();
		if (!entry) return;
		const inSelection = !entry.is_dir && selected.has(fullPathOf(entry));
		const multi = inSelection && selectedEntries.length > 1;
		if (id === 'download') {
			if (multi) bulkDownload();
			else download(entry);
		} else if (id === 'preview') {
			openPreview(entry);
		} else if (id === 'open') {
			openDir(entry);
		} else if (id === 'copy') {
			copyPath(fullPathOf(entry));
		} else if (id === 'delete') {
			if (multi) bulkDelete();
			else deleteEntry(entry);
		}
	}

	// Row-body click behavior (preserved): dir -> navigate; previewable ->
	// preview; otherwise download.
	function onNameClick(entry) {
		if (entry.is_dir) openDir(entry);
		else if (isPreviewable(entry)) openPreview(entry);
		else download(entry);
	}
</script>

<svelte:window on:keydown={onWindowKeydown} />

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
		<h1 class="page-title">{daemonLabel || daemonId || 'files'}</h1>
		<div class="page-header__status" role="status" aria-live="polite">
			{#if offline}
				<Face state="offline" /> <span>offline</span>
			{:else}
				<Face state="ok" /> <span>connected</span>
			{/if}
		</div>
	</header>

	<div class="browser-bar">
		<nav class="breadcrumb" aria-label="file path">
			<button
				type="button"
				class="crumb crumb--root"
				on:click={() => changeDir('')}
				aria-label="go to root">~/</button
			>
			{#each pathSegments as segment, i}
				<span class="breadcrumb__sep" aria-hidden="true">/</span>
				<button
					type="button"
					class="crumb"
					class:crumb--leaf={i === pathSegments.length - 1}
					on:click={() => goToSegment(i)}>{segment}</button
				>
			{/each}
		</nav>

		<div class="browser-bar__tools">
			<ViewToggle bind:value={fileView} on:change={(e) => setView(e.detail)} />
		</div>
	</div>

	{#if error}<p class="error" role="alert"><Face state="error" /> {error}</p>{/if}

	{#if loading}
		<div class="card file-list-wrap" aria-busy="true">
			<p class="sr-status" role="status" aria-live="polite"><Face state="loading" /> loading…</p>
			<table class="file-list">
				<thead>
					<tr>
						<th scope="col" class="col-select"></th>
						<th scope="col" class="col-icon"></th>
						<th scope="col" class="col-name">name</th>
						<th scope="col" class="col-size">size</th>
						<th scope="col" class="col-mtime">modified</th>
						<th scope="col" class="col-actions"></th>
					</tr>
				</thead>
			</table>
			<div class="file-skeletons"><Skeleton variant="row" count={7} /></div>
		</div>
	{:else if offline}
		<div class="card empty-state" role="status">
			<span class="empty-state__face"><Face state="offline" /></span>
			<p class="empty-state__msg">this host is offline</p>
			<p class="empty-state__hint muted">files will load when it reconnects.</p>
		</div>
	{:else}
		<!-- LIST VIEW (default; always renders table.file-list for the e2e contract) -->
		{#if fileView === 'list'}
			<div
				class="card file-list-wrap"
				class:drop-target={draggingOver}
				class:is-scrolled={scrolled}
				bind:this={listEl}
				on:scroll={onScroll}
			>
				{#if draggingOver}
					<div class="drop-overlay" aria-hidden="true">
						<span class="drop-overlay__inner">drop files to upload</span>
					</div>
				{/if}
				<table class="file-list">
					<thead>
						<tr>
							<th scope="col" class="col-select">
								<Checkbox
									checked={allSelected}
									indeterminate={headerIndeterminate}
									ariaLabel="select all files"
									on:change={toggleSelectAll}
								/>
							</th>
							<th scope="col" class="col-icon" aria-hidden="true"></th>
							<th
								scope="col"
								class="col-name sortable"
								class:sort-asc={sortBy === 'name' && sortDir === 'asc'}
								class:sort-desc={sortBy === 'name' && sortDir === 'desc'}
								aria-sort={sortBy === 'name'
									? sortDir === 'asc'
										? 'ascending'
										: 'descending'
									: 'none'}
							>
								<button
									type="button"
									class="th-sort"
									on:click={() => setSort('name')}
									aria-label="sort by name">name</button
								>
							</th>
							<th
								scope="col"
								class="col-size sortable"
								class:sort-asc={sortBy === 'size' && sortDir === 'asc'}
								class:sort-desc={sortBy === 'size' && sortDir === 'desc'}
								aria-sort={sortBy === 'size'
									? sortDir === 'asc'
										? 'ascending'
										: 'descending'
									: 'none'}
							>
								<button
									type="button"
									class="th-sort"
									on:click={() => setSort('size')}
									aria-label="sort by size">size</button
								>
							</th>
							<th
								scope="col"
								class="col-mtime sortable"
								class:sort-asc={sortBy === 'mtime' && sortDir === 'asc'}
								class:sort-desc={sortBy === 'mtime' && sortDir === 'desc'}
								aria-sort={sortBy === 'mtime'
									? sortDir === 'asc'
										? 'ascending'
										: 'descending'
									: 'none'}
							>
								<button
									type="button"
									class="th-sort"
									on:click={() => setSort('mtime')}
									aria-label="sort by modified">modified</button
								>
							</th>
							<th scope="col" class="col-actions"></th>
						</tr>
					</thead>
					<tbody>
						{#if pathSegments.length > 0}
							<tr class="file-row file-row--up">
								<td class="col-select"></td>
								<td class="col-icon"><FileIcon name=".." is_dir={true} /></td>
								<td class="col-name" colspan="4">
									<button
										type="button"
										class="link"
										on:click={goUp}
										aria-label="go to parent directory">..</button
									>
								</td>
							</tr>
						{/if}
						{#each sortedEntries as entry (entry.name)}
							{@const fp = fullPathOf(entry)}
							{@const selIndex = entry.is_dir ? -1 : selectableEntries.indexOf(entry)}
							<tr
								class="file-row"
								class:is-selected={selected.has(fp)}
								on:contextmenu={(e) => onRowContextMenu(entry, e)}
							>
								<td class="col-select">
									{#if !entry.is_dir}
										<Checkbox
											checked={selected.has(fp)}
											ariaLabel="select {entry.name}"
											on:change={(e) => handleSelect(entry, selIndex, e)}
										/>
									{/if}
								</td>
								<td class="col-icon"><FileIcon name={entry.name} is_dir={entry.is_dir} /></td>
								<td class="col-name" data-label="name">
									<button
										type="button"
										class="link"
										class:dir={entry.is_dir}
										title={entry.name}
										on:click={() => onNameClick(entry)}
										>{entry.name}{entry.is_dir ? '/' : ''}</button
									>
								</td>
								<td class="col-size num" data-label="size"
									>{entry.is_dir ? '—' : formatSize(entry.size)}</td
								>
								<td class="col-mtime num" data-label="modified" title={entry.mtime || ''}
									>{formatMtime(entry.mtime)}</td
								>
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
										class="link kebab-btn"
										on:click={(e) => openContextMenu(entry, e, true)}
										title="actions"
										aria-label="actions for {entry.name}"
										aria-haspopup="menu">⋯</button
									>
								</td>
							</tr>
						{/each}
						{#if sortedEntries.length === 0 && pathSegments.length === 0}
							<tr class="file-row file-row--empty">
								<td colspan="6">
									<div class="empty-inline">
										<span class="empty-inline__face"><Face state="ok" /></span>
										<span>this folder is empty — drag files here to upload</span>
									</div>
								</td>
							</tr>
						{/if}
					</tbody>
				</table>
			</div>
		{:else}
			<!-- GRID VIEW -->
			<div class="grid-sort">
				<label class="grid-sort__label microlabel" for="grid-sort-select">sort</label>
				<select
					id="grid-sort-select"
					class="grid-sort__select"
					value={sortBy}
					on:change={(e) => setSort(e.currentTarget.value)}
				>
					<option value="name">name</option>
					<option value="size">size</option>
					<option value="mtime">modified</option>
				</select>
				<button
					type="button"
					class="grid-sort__dir"
					on:click={() => (sortDir = sortDir === 'asc' ? 'desc' : 'asc')}
					aria-label="toggle sort direction">{sortDir === 'asc' ? '↑' : '↓'}</button
				>
			</div>
			<div
				class="card file-grid-wrap"
				class:drop-target={draggingOver}
				bind:this={listEl}
				on:scroll={onScroll}
			>
				{#if draggingOver}
					<div class="drop-overlay" aria-hidden="true">
						<span class="drop-overlay__inner">drop files to upload</span>
					</div>
				{/if}
				{#if sortedEntries.length === 0 && pathSegments.length === 0}
					<div class="empty-inline empty-inline--grid">
						<span class="empty-inline__face"><Face state="ok" /></span>
						<span>this folder is empty — drag files here to upload</span>
					</div>
				{:else}
					<div class="file-grid">
						{#if pathSegments.length > 0}
							<!-- svelte-ignore a11y-click-events-have-key-events -->
							<button
								type="button"
								class="tile tile--up"
								on:click={goUp}
								aria-label="parent directory"
							>
								<span class="tile__icon"><FileIcon name=".." is_dir={true} /></span>
								<span class="tile__name">..</span>
							</button>
						{/if}
						{#each sortedEntries as entry (entry.name)}
							{@const fp = fullPathOf(entry)}
							{@const selIndex = entry.is_dir ? -1 : selectableEntries.indexOf(entry)}
							<div
								class="tile"
								class:is-selected={selected.has(fp)}
								on:contextmenu={(e) => onRowContextMenu(entry, e)}
								role="presentation"
							>
								{#if !entry.is_dir}
									<span class="tile__check">
										<Checkbox
											checked={selected.has(fp)}
											ariaLabel="select {entry.name}"
											on:change={(e) => handleSelect(entry, selIndex, e)}
										/>
									</span>
								{/if}
								<span class="tile__top-actions">
									{#if !entry.is_dir}
										<button
											type="button"
											class="link dl-btn"
											on:click|stopPropagation={() => download(entry)}
											title="download"
											aria-label="download {entry.name}">↓</button
										>
									{/if}
									<button
										type="button"
										class="link kebab-btn"
										on:click|stopPropagation={(e) => openContextMenu(entry, e, true)}
										title="actions"
										aria-label="actions for {entry.name}"
										aria-haspopup="menu">⋯</button
									>
								</span>
								<button
									type="button"
									class="tile__body"
									on:click={() => onNameClick(entry)}
									title={entry.name}
								>
									<span class="tile__icon tile__icon--lg"
										><FileIcon name={entry.name} is_dir={entry.is_dir} size={30} /></span
									>
									<span class="tile__name">{entry.name}{entry.is_dir ? '/' : ''}</span>
									<span class="tile__size num">{entry.is_dir ? '' : formatSize(entry.size)}</span>
								</button>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}

		{#if selected.size > 0}
			<SelectionBar
				count={selected.size}
				on:download={bulkDownload}
				on:delete={bulkDelete}
				on:clear={clearSelection}
			/>
		{/if}

		<!-- Upload drop-zone (clear affordance) -->
		<div class="upload">
			<div class="upload-row" class:upload-row--drag={draggingOver}>
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
						bind:this={fileInput}
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
							{selectedFileName || 'drop files here · or choose files'}
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
			class="preview-modal overlay-surface"
			role="dialog"
			tabindex="-1"
			aria-modal="true"
			aria-labelledby="preview-title"
			bind:this={previewEl}
			on:click|stopPropagation
		>
			<div class="preview-header">
				<span class="preview-filename" id="preview-title" title={previewEntry.name}
					>{previewEntry.name}</span
				>
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

<ContextMenu
	bind:open={ctxOpen}
	bind:x={ctxX}
	bind:y={ctxY}
	items={ctxItems}
	mobileSheet={ctxSheet}
	on:select={onMenuSelect}
	on:close={closeContextMenu}
/>

{#if toast.show}
	<div class="toast toast-{toast.type}" role="status" aria-live="polite">{toast.message}</div>
{/if}

<style>
	.back-link {
		margin: 0 0 var(--space-sm);
		font-size: var(--fs-xs);
	}
	.back-link a {
		color: var(--text-muted);
		text-decoration: none;
	}
	.back-link a:hover {
		color: var(--accent);
	}

	.page-header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: var(--space-md);
		flex-wrap: wrap;
		margin-bottom: var(--space-md);
	}
	.page-header__status {
		display: inline-flex;
		align-items: center;
		gap: var(--space-xs);
		font-size: var(--fs-xs);
		color: var(--text-muted);
	}

	.browser-bar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-md);
		margin: 0 0 var(--space-md);
	}

	.breadcrumb {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-xs);
		min-width: 0;
		font-size: var(--fs-sm);
	}
	.crumb {
		background: none;
		border: none;
		height: auto;
		padding: var(--space-xs) var(--space-sm);
		color: var(--text-muted);
		font-size: var(--fs-sm);
		border-radius: var(--radius-sm);
		max-width: 14rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.crumb--root {
		color: var(--text-faint);
	}
	.crumb--leaf {
		color: var(--text);
		font-weight: 500;
	}
	.crumb:hover {
		color: var(--accent);
		background: var(--accent-soft);
		border: none;
	}
	.crumb:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	.breadcrumb__sep {
		color: var(--text-faint);
	}

	.browser-bar__tools {
		flex-shrink: 0;
	}

	/* ---- List view ----------------------------------------------------- */
	.file-list-wrap {
		position: relative;
		overflow-y: auto;
		min-height: 120px;
		max-height: min(58vh, 480px);
		margin-bottom: var(--space-lg);
	}
	@media (prefers-reduced-motion: no-preference) {
		.file-list-wrap {
			transition:
				border-color var(--dur-fast),
				box-shadow var(--dur-fast);
		}
	}
	.file-list-wrap.drop-target,
	.file-grid-wrap.drop-target {
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
		z-index: 10;
		border-radius: var(--radius);
		pointer-events: none;
		padding: var(--space-md);
	}
	.drop-overlay__inner {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 100%;
		height: 100%;
		border: 2px dashed var(--accent);
		border-radius: var(--radius);
		color: var(--accent);
		font-size: var(--fs-md);
		font-weight: 600;
		letter-spacing: 0.05em;
	}

	.sr-status {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}

	.file-skeletons {
		padding: var(--space-sm) var(--space-md);
	}

	.file-list {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--fs-sm);
		margin: 0;
	}
	.file-list th {
		position: sticky;
		top: 0;
		z-index: 2;
		background: var(--surface);
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--border);
		font-size: var(--fs-2xs);
		font-weight: 500;
		letter-spacing: var(--tracking-label);
		text-transform: uppercase;
		color: var(--text-faint);
	}
	/* Scroll shadow under the sticky header. */
	.file-list-wrap.is-scrolled .file-list th {
		box-shadow: 0 1px 0 var(--border-strong);
	}
	.file-list td {
		padding: var(--space-xs) var(--space-md);
		border-bottom: 1px solid var(--border);
		vertical-align: middle;
		height: var(--row-height);
	}
	.file-list tbody tr:last-child td {
		border-bottom: none;
	}
	.file-row {
		border-left: 2px solid transparent;
	}
	.file-list tbody tr.file-row:hover td {
		background: var(--row-hover);
	}
	.file-row.is-selected td {
		background: var(--select-tint);
	}
	.file-row.is-selected {
		border-left-color: var(--accent);
	}
	/* Dim the row actions at rest (full strength on hover/focus/selection), but
	   keep them perceivable — 0.55 was below a comfortable contrast floor. */
	.file-row .dl-btn,
	.file-row .kebab-btn {
		opacity: 0.75;
	}
	.file-row:hover .dl-btn,
	.file-row:hover .kebab-btn,
	.file-row.is-selected .dl-btn,
	.file-row.is-selected .kebab-btn,
	.dl-btn:focus-visible,
	.kebab-btn:focus-visible {
		opacity: 1;
	}

	.col-select {
		width: var(--checkbox-col);
		padding-left: var(--space-xs);
		padding-right: 0;
	}
	.col-icon {
		width: var(--icon-col);
		padding-left: var(--space-xs);
		padding-right: 0;
	}
	.col-icon :global(.file-icon) {
		display: block;
	}
	.col-name {
		min-width: 0;
		text-align: left;
	}
	.col-size {
		width: 7rem;
		color: var(--text-muted);
		white-space: nowrap;
		text-align: right;
	}
	.col-mtime {
		width: 12rem;
		color: var(--text-muted);
		font-size: var(--fs-xs);
		text-align: right;
	}
	.col-actions {
		width: 6rem;
		text-align: right;
		white-space: nowrap;
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
	.file-list th.sortable .th-sort:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
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
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.file-list .link:hover {
		color: var(--accent);
	}
	.file-list .link:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	.file-list .link.dir {
		color: var(--accent);
		font-weight: 500;
	}
	.file-list .link.dir:hover {
		text-decoration: underline;
		text-underline-offset: 3px;
	}
	.dl-btn {
		color: var(--text-muted);
		padding: var(--space-sm);
	}
	.dl-btn:hover {
		color: var(--accent);
	}
	.kebab-btn {
		color: var(--text-muted);
		padding: var(--space-sm);
		font-size: var(--fs-md);
	}
	.kebab-btn:hover {
		color: var(--text);
	}

	.empty-inline {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-sm);
		padding: var(--space-2xl) var(--space-md);
		color: var(--text-muted);
		font-size: var(--fs-sm);
		text-align: center;
	}
	.empty-inline__face {
		color: var(--text);
	}

	/* ---- Empty / offline state ----------------------------------------- */
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-2xl) var(--space-lg);
		margin-bottom: var(--space-lg);
		text-align: center;
	}
	.empty-state__face {
		font-size: var(--fs-xl);
	}
	.empty-state__msg {
		margin: 0;
		font-size: var(--fs-md);
		font-weight: 500;
	}
	.empty-state__hint {
		margin: 0;
		font-size: var(--fs-xs);
	}

	/* ---- Grid view ----------------------------------------------------- */
	.grid-sort {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		margin-bottom: var(--space-sm);
	}
	.grid-sort__label {
		color: var(--text-faint);
	}
	.grid-sort__select {
		height: 2rem;
		padding: 0 var(--space-sm);
		background: var(--inset);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--text);
		font-family: var(--font-mono);
		font-size: var(--fs-xs);
	}
	.grid-sort__dir {
		height: 2rem;
		width: 2rem;
		padding: 0;
		background: var(--inset);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--accent);
	}
	.file-grid-wrap {
		position: relative;
		overflow-y: auto;
		min-height: 120px;
		max-height: min(58vh, 480px);
		margin-bottom: var(--space-lg);
		padding: var(--space-md);
	}
	.file-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(var(--grid-tile), 1fr));
		gap: var(--space-md);
	}
	.tile {
		position: relative;
		display: flex;
		flex-direction: column;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		background: var(--inset);
		overflow: hidden;
	}
	.tile.is-selected {
		border-color: var(--accent);
		background: var(--select-tint);
	}
	.tile__check {
		position: absolute;
		top: 0;
		left: 0;
		z-index: 2;
	}
	.tile__top-actions {
		position: absolute;
		top: var(--space-xs);
		right: var(--space-xs);
		z-index: 2;
		display: flex;
		gap: var(--space-xs);
	}
	.tile__body {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-xs);
		width: 100%;
		/* Reset the global button height/whitespace so the tile grows to fit
		   its icon + name + size column instead of clamping to --input-height. */
		height: auto;
		white-space: normal;
		padding: var(--space-lg) var(--space-sm) var(--space-md);
		background: none;
		border: none;
		cursor: pointer;
		color: var(--text);
		font-family: var(--font-mono);
	}
	.tile__body:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: -2px;
	}
	.tile--up {
		align-items: center;
		justify-content: center;
		height: auto;
		padding: var(--space-lg) var(--space-sm);
		cursor: pointer;
		color: var(--text-muted);
	}
	.tile__icon--lg {
		margin: var(--space-sm) 0;
		color: var(--text-muted);
	}
	.tile__icon--lg :global(.file-icon--folder) {
		color: var(--accent);
	}
	.tile__name {
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
		font-size: var(--fs-xs);
		line-height: var(--lh-tight);
		text-align: center;
		overflow-wrap: anywhere;
	}
	.tile__size {
		font-size: var(--fs-2xs);
		color: var(--text-muted);
	}
	.empty-inline--grid {
		grid-column: 1 / -1;
	}

	/* ---- Upload drop-zone ---------------------------------------------- */
	.upload {
		margin-top: var(--space-lg);
	}
	.upload-row {
		display: flex;
		align-items: center;
		gap: var(--space-md);
		width: 100%;
		padding: var(--space-md);
		border: 2px dashed var(--border);
		border-radius: var(--radius);
		background: var(--inset);
		box-sizing: border-box;
	}
	@media (prefers-reduced-motion: no-preference) {
		.upload-row {
			transition:
				border-color var(--dur-fast),
				background var(--dur-fast);
		}
	}
	.upload-row--drag {
		border-color: var(--accent);
		background: var(--accent-soft);
	}
	.upload-label {
		flex-shrink: 0;
		color: var(--text-faint);
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
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		cursor: pointer;
		position: relative;
		box-sizing: border-box;
	}
	@media (prefers-reduced-motion: no-preference) {
		.upload-file-wrap {
			transition: border-color var(--dur-fast);
		}
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
		font-size: var(--fs-xs);
		color: var(--text-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		pointer-events: none;
	}

	/* ---- Preview modal ------------------------------------------------- */
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
		width: min(90vw, 56rem);
		max-height: 85vh;
		display: flex;
		flex-direction: column;
		overflow: hidden;
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
		font-size: var(--fs-sm);
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
		font-size: var(--fs-xs);
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
		font-size: var(--fs-xs);
		line-height: var(--lh-body);
		color: var(--text);
		white-space: pre-wrap;
		word-break: break-all;
		margin: 0;
		tab-size: 2;
	}

	/* ---- Mobile (<=640px): table -> cards; col-mtime collapses --------- */
	@media (max-width: 640px) {
		.browser-bar {
			flex-wrap: wrap;
		}

		/* The .col-mtime <th> MUST stay in the DOM but hidden (e2e contract). */
		.file-list th.col-mtime {
			display: none;
		}

		/* Restyle the SAME table.file-list into cards. Keep the element. */
		.file-list thead {
			position: absolute;
			width: 1px;
			height: 1px;
			padding: 0;
			margin: -1px;
			overflow: hidden;
			clip: rect(0, 0, 0, 0);
			white-space: nowrap;
			border: 0;
		}
		.file-list,
		.file-list tbody {
			display: block;
			width: 100%;
		}
		/* Two-line card: name on top, a single "size · date" meta line below.
		   The checkbox and actions top-align with the name (align-items: start)
		   instead of floating centered across the whole card. */
		.file-list tbody tr.file-row {
			display: grid;
			grid-template-columns: var(--touch-min) auto 1fr auto;
			grid-template-areas:
				'select name name actions'
				'select size mtime actions';
			align-items: start;
			gap: 0 var(--space-sm);
			margin-bottom: var(--space-sm);
			padding: var(--space-xs);
			border: 1px solid var(--border);
			border-left-width: 2px;
			border-radius: var(--radius-sm);
		}
		.file-list tbody tr.file-row td {
			display: block;
			height: auto;
			padding: 0;
			border: none;
		}
		.file-list tbody tr.file-row:hover td {
			background: transparent;
		}
		.file-row.is-selected td {
			background: transparent;
		}
		.file-list td.col-select {
			grid-area: select;
			width: auto;
		}
		/* Must out-specify `.file-list tbody tr.file-row td { display:block }`
		   above, or the icon cell reappears at the card's bottom-left. */
		.file-list tbody tr.file-row td.col-icon {
			display: none;
		}
		.file-list td.col-name {
			grid-area: name;
			min-width: 0;
		}
		.file-list td.col-name .link {
			white-space: normal;
			overflow-wrap: anywhere;
			padding: var(--space-xs) 0;
			min-height: var(--touch-min);
			display: flex;
			align-items: center;
		}
		/* size + mtime each get their own meta line in the card (the <th> stays
		   hidden, but the td.col-mtime cell stays visible per the e2e contract). */
		.file-list td.col-size,
		.file-list td.col-mtime {
			width: auto;
			text-align: left;
			font-size: var(--fs-2xs);
			color: var(--text-muted);
			display: block;
		}
		.file-list td.col-size {
			grid-area: size;
			padding-top: 2px;
		}
		.file-list td.col-mtime {
			grid-area: mtime;
			padding-top: 2px;
		}
		/* "size · date" reads as one meta line. */
		.file-list td.col-mtime::before {
			content: '·';
			margin-right: 0.4rem;
			color: var(--text-faint);
		}
		.file-list td.col-actions {
			grid-area: actions;
			width: auto;
			display: flex;
			align-items: center;
		}
		.file-list tr.file-row--up {
			grid-template-columns: 1fr;
			grid-template-areas: 'name';
		}
		.file-list tr.file-row--up td.col-name {
			grid-column: 1;
		}
		.file-list tr.file-row--empty {
			display: block;
			border: none;
			padding: 0;
		}
		.file-list tr.file-row--empty td {
			display: block;
		}
		.dl-btn,
		.kebab-btn {
			min-width: var(--touch-min);
			min-height: var(--touch-min);
			opacity: 1;
		}

		.file-grid {
			grid-template-columns: repeat(2, 1fr);
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
		.preview-header {
			padding: var(--space-md);
		}
		.preview-actions button {
			padding: 0 var(--space-sm);
		}
	}
</style>
