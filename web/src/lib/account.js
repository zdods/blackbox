// Shared account profile: the current user's username/email plus the capability
// flags that decide which credential controls the account screen renders. Loaded
// once when the app shell becomes active (the header avatar reads it) and
// refreshed by the account page after edits.
import { writable } from 'svelte/store';
import { apiFetch } from '$lib/auth.js';

// null until first load; otherwise
// { username, email, has_password, password_enabled, passkey_enabled }.
export const account = writable(null);

export async function loadAccount() {
	const res = await apiFetch('/api/account');
	if (!res.ok) throw new Error(await res.text());
	const data = await res.json();
	account.set(data);
	return data;
}

export function clearAccount() {
	account.set(null);
}

// gravatarURL builds a Gravatar avatar URL from an email. Gravatar accepts a
// SHA-256 hash of the trimmed, lowercased address — which crypto.subtle gives
// us with no md5 dependency. Returns null when there's no email or no Web
// Crypto (e.g. a non-secure context), so callers fall back to initials.
export async function gravatarURL(email, size = 160) {
	const norm = (email || '').trim().toLowerCase();
	if (!norm || typeof crypto === 'undefined' || !crypto.subtle) return null;
	const bytes = new TextEncoder().encode(norm);
	const digest = await crypto.subtle.digest('SHA-256', bytes);
	const hex = [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, '0')).join('');
	// d=identicon: a deterministic geometric avatar when the address has no
	// Gravatar of its own, so the avatar is never a broken image.
	return `https://www.gravatar.com/avatar/${hex}?d=identicon&s=${size}`;
}
