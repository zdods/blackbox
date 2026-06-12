const TOKEN_KEY = 'blackhaul_token';

export function getToken() {
	if (typeof window === 'undefined') return null;
	return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token) {
	if (typeof window === 'undefined') return;
	localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
	if (typeof window === 'undefined') return;
	localStorage.removeItem(TOKEN_KEY);
}

export function apiFetch(path, options = {}) {
	const headers = { ...options.headers };
	// Rely on httpOnly session cookie set by the server.
	// Send credentials so the browser includes the cookie.
	const fetchOptions = { ...options, headers, cache: 'no-store', credentials: 'same-origin' };
	return fetch(path, fetchOptions);
}
