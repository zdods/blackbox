<script>
	// File-type glyph by extension bucket. Inline SVG only (CSP-safe),
	// currentColor, 16px, aria-hidden (the file name carries the label).
	export let name;
	export let is_dir = false;
	export let size = 16;

	// Buckets mirror the file-browser's IMAGE_EXTS/TEXT_EXTS and extend them.
	const IMAGE_EXTS = new Set([
		'png',
		'jpg',
		'jpeg',
		'gif',
		'webp',
		'svg',
		'bmp',
		'ico',
		'avif',
		'tif',
		'tiff',
		'heic'
	]);
	const CODE_EXTS = new Set([
		'js',
		'mjs',
		'cjs',
		'ts',
		'tsx',
		'jsx',
		'go',
		'rs',
		'py',
		'rb',
		'php',
		'java',
		'kt',
		'swift',
		'c',
		'cpp',
		'h',
		'hpp',
		'cs',
		'sh',
		'bash',
		'zsh',
		'sql',
		'html',
		'htm',
		'css',
		'scss',
		'sass',
		'json',
		'yaml',
		'yml',
		'toml',
		'xml',
		'vue',
		'svelte'
	]);
	const DOC_EXTS = new Set([
		'md',
		'txt',
		'csv',
		'tsv',
		'log',
		'rtf',
		'doc',
		'docx',
		'odt',
		'rst',
		'conf',
		'ini',
		'cfg',
		'env'
	]);
	const ARCHIVE_EXTS = new Set([
		'zip',
		'tar',
		'gz',
		'tgz',
		'bz2',
		'xz',
		'7z',
		'rar',
		'zst',
		'lz',
		'lzma'
	]);
	const MEDIA_EXTS = new Set([
		'mp3',
		'wav',
		'flac',
		'aac',
		'ogg',
		'm4a',
		'opus',
		'mp4',
		'mov',
		'mkv',
		'webm',
		'avi',
		'wmv',
		'm4v'
	]);
	const PDF_EXTS = new Set(['pdf']);

	function getExt(n) {
		if (!n) return '';
		const i = n.lastIndexOf('.');
		return i >= 0 ? n.slice(i + 1).toLowerCase() : '';
	}

	function bucketFor(n, dir) {
		if (dir) return 'folder';
		const ext = getExt(n);
		if (IMAGE_EXTS.has(ext)) return 'image';
		if (CODE_EXTS.has(ext)) return 'code';
		if (DOC_EXTS.has(ext)) return 'doc';
		if (ARCHIVE_EXTS.has(ext)) return 'archive';
		if (MEDIA_EXTS.has(ext)) return 'media';
		if (PDF_EXTS.has(ext)) return 'pdf';
		return 'generic';
	}

	$: bucket = bucketFor(name, is_dir);
</script>

<span class="file-icon file-icon--{bucket}" aria-hidden="true" style="--file-icon-size: {size}px">
	{#if bucket === 'folder'}
		<svg
			class="file-icon__svg"
			viewBox="0 0 16 16"
			width="16"
			height="16"
			fill="none"
			stroke="currentColor"
			stroke-width="1.3"
			stroke-linecap="round"
			stroke-linejoin="round"
		>
			<path d="M1.75 4.25v8a1 1 0 0 0 1 1h10.5a1 1 0 0 0 1-1v-6.5a1 1 0 0 0-1-1H7.5L6 3.25H2.75a1 1 0 0 0-1 1Z" />
		</svg>
	{:else if bucket === 'image'}
		<svg
			class="file-icon__svg"
			viewBox="0 0 16 16"
			width="16"
			height="16"
			fill="none"
			stroke="currentColor"
			stroke-width="1.2"
			stroke-linecap="round"
			stroke-linejoin="round"
		>
			<rect x="2" y="2.75" width="12" height="10.5" rx="1.25" />
			<circle cx="5.75" cy="6.25" r="1.1" />
			<path d="m2.5 11.5 3.25-3 2.5 2.25L11 8.25l2.5 2.75" />
		</svg>
	{:else if bucket === 'code'}
		<svg
			class="file-icon__svg"
			viewBox="0 0 16 16"
			width="16"
			height="16"
			fill="none"
			stroke="currentColor"
			stroke-width="1.2"
			stroke-linecap="round"
			stroke-linejoin="round"
		>
			<path d="m6 5-3 3 3 3" />
			<path d="m10 5 3 3-3 3" />
		</svg>
	{:else if bucket === 'doc'}
		<svg
			class="file-icon__svg"
			viewBox="0 0 16 16"
			width="16"
			height="16"
			fill="none"
			stroke="currentColor"
			stroke-width="1.2"
			stroke-linecap="round"
			stroke-linejoin="round"
		>
			<path d="M4 1.75h5l3 3v9.5a.5.5 0 0 1-.5.5h-7.5a.5.5 0 0 1-.5-.5V2.25a.5.5 0 0 1 .5-.5Z" />
			<path d="M9 1.75v3h3" />
			<path d="M5.75 8.25h4.5M5.75 10.5h4.5M5.75 6h2" />
		</svg>
	{:else if bucket === 'archive'}
		<svg
			class="file-icon__svg"
			viewBox="0 0 16 16"
			width="16"
			height="16"
			fill="none"
			stroke="currentColor"
			stroke-width="1.2"
			stroke-linecap="round"
			stroke-linejoin="round"
		>
			<path d="M4 1.75h8a.5.5 0 0 1 .5.5v11.5a.5.5 0 0 1-.5.5H4a.5.5 0 0 1-.5-.5V2.25a.5.5 0 0 1 .5-.5Z" />
			<path d="M8 1.75v1.5M8 4.75v1.5M8 7.75v1.5" />
			<rect x="6.75" y="10" width="2.5" height="2.75" rx="0.4" />
		</svg>
	{:else if bucket === 'media'}
		<svg
			class="file-icon__svg"
			viewBox="0 0 16 16"
			width="16"
			height="16"
			fill="none"
			stroke="currentColor"
			stroke-width="1.2"
			stroke-linecap="round"
			stroke-linejoin="round"
		>
			<rect x="2" y="3" width="12" height="10" rx="1.25" />
			<path d="m6.75 6 3 2-3 2V6Z" fill="currentColor" stroke="none" />
		</svg>
	{:else if bucket === 'pdf'}
		<svg
			class="file-icon__svg"
			viewBox="0 0 16 16"
			width="16"
			height="16"
			fill="none"
			stroke="currentColor"
			stroke-width="1.2"
			stroke-linecap="round"
			stroke-linejoin="round"
		>
			<path d="M4 1.75h5l3 3v9.5a.5.5 0 0 1-.5.5h-7.5a.5.5 0 0 1-.5-.5V2.25a.5.5 0 0 1 .5-.5Z" />
			<path d="M9 1.75v3h3" />
			<path d="M5.5 11.75c1.5-.5 2.75-2.5 2.5-4-.2-1.2-1-1-1 .25 0 1.5 1.5 3.25 3.5 3.25" />
		</svg>
	{:else}
		<svg
			class="file-icon__svg"
			viewBox="0 0 16 16"
			width="16"
			height="16"
			fill="none"
			stroke="currentColor"
			stroke-width="1.2"
			stroke-linecap="round"
			stroke-linejoin="round"
		>
			<path d="M4 1.75h5l3 3v9.5a.5.5 0 0 1-.5.5h-7.5a.5.5 0 0 1-.5-.5V2.25a.5.5 0 0 1 .5-.5Z" />
			<path d="M9 1.75v3h3" />
		</svg>
	{/if}
</span>

<style>
	.file-icon {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: var(--file-icon-size, 16px);
		height: var(--file-icon-size, 16px);
		flex-shrink: 0;
		color: var(--text-muted);
	}
	.file-icon__svg {
		display: block;
		width: var(--file-icon-size, 16px);
		height: var(--file-icon-size, 16px);
	}
	/* Folder reads as the primary affordance — tinted accent. */
	.file-icon--folder {
		color: var(--accent);
	}
</style>
