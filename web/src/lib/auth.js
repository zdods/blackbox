import { goto } from '$app/navigation';

// The real session credential is an httpOnly cookie set by the server and never
// readable by JS. This flag is only a client-side "probably logged in" hint used
// to decide whether to render the app or redirect to /login — it holds no secret.
const AUTH_FLAG_KEY = 'blackhaul_authed';

export function isLoggedIn() {
	if (typeof window === 'undefined') return false;
	return localStorage.getItem(AUTH_FLAG_KEY) === 'true';
}

export function setLoggedIn() {
	if (typeof window === 'undefined') return;
	localStorage.setItem(AUTH_FLAG_KEY, 'true');
}

export function clearLoggedIn() {
	if (typeof window === 'undefined') return;
	localStorage.removeItem(AUTH_FLAG_KEY);
}

export function apiFetch(path, options = {}) {
	const headers = { ...options.headers };
	// The httpOnly session cookie authenticates the request; send credentials so
	// the browser includes it.
	const fetchOptions = { ...options, headers, cache: 'no-store', credentials: 'same-origin' };
	return fetch(path, fetchOptions);
}

// redirectIfUnauthorized handles the one response every authenticated screen
// reacts to the same way: a 401 means the session is gone, so clear the local
// flag and send the user to /login. Returns true when it acted, so callers can
//   if (redirectIfUnauthorized(res)) return;
export function redirectIfUnauthorized(res) {
	if (res.status === 401) {
		clearLoggedIn();
		goto('/login');
		return true;
	}
	return false;
}
