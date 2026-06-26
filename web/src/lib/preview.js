// File-preview classification — which entries the file browser can show inline,
// and how. Pure helpers (no DOM/network) so they're trivial to reason about and
// keep the big extension tables out of the route component.

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

// getExt returns the lowercased extension (without the dot), or '' if none.
export function getExt(name) {
	const i = name.lastIndexOf('.');
	return i >= 0 ? name.slice(i + 1).toLowerCase() : '';
}

// isImageExt reports whether the extension previews as an image (vs. text).
export function isImageExt(ext) {
	return IMAGE_EXTS.has(ext);
}

// isPreviewable reports whether an entry can be previewed inline, honoring the
// per-type size caps (an unknown size is allowed and checked on load).
export function isPreviewable(entry) {
	if (entry.is_dir) return false;
	const ext = getExt(entry.name);
	if (IMAGE_EXTS.has(ext)) return entry.size == null || entry.size <= IMAGE_PREVIEW_MAX;
	if (TEXT_EXTS.has(ext)) return entry.size == null || entry.size <= TEXT_PREVIEW_MAX;
	return false;
}
