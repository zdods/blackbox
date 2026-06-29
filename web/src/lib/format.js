// Shared display formatters. Kept tiny and dependency-free so any component can
// import them instead of re-deriving the same byte/date logic locally.

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'];

// scaleBytes reduces a byte count to a { value, unit, exact } triple, stepping
// by 1024 up to TB. `exact` is true at the bytes step (no decimals needed).
// Exposed so call sites with their own rounding (e.g. the compact host rail)
// can reuse the scaling without duplicating the loop.
export function scaleBytes(n) {
	let value = n;
	let i = 0;
	while (value >= 1024 && i < BYTE_UNITS.length - 1) {
		value /= 1024;
		i += 1;
	}
	return { value, unit: BYTE_UNITS[i], exact: i === 0 };
}

// formatBytes renders a byte count as "12.3 GB" (one decimal, whole numbers for
// raw bytes). Unknown/negative renders as an em dash.
export function formatBytes(n) {
	if (n == null || n < 0 || Number.isNaN(n)) return '—';
	const { value, unit, exact } = scaleBytes(n);
	return (exact ? value : value.toFixed(1)) + ' ' + unit;
}

// formatDate renders an ISO timestamp as a localized "Mar 3, 2026". Empty or
// unparseable input renders as an empty string (callers supply their own
// placeholder when they want one).
export function formatDate(iso) {
	if (!iso) return '';
	const d = new Date(iso);
	if (isNaN(d.getTime())) return '';
	return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}
