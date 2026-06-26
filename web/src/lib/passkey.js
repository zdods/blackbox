// Passkey (WebAuthn) ceremony helpers. The browser library handles all
// base64url <-> ArrayBuffer encoding; the server (go-webauthn) emits options as
// { publicKey: {...} }, so we hand the inner `publicKey` object to the library.
import {
	startRegistration,
	startAuthentication,
	browserSupportsWebAuthn
} from '@simplewebauthn/browser';
import { apiFetch } from '$lib/auth.js';

export { browserSupportsWebAuthn };

// AbortError / NotAllowedError are thrown when the user dismisses the native
// passkey prompt — surface a friendly message, not a stack trace.
function isUserCancel(err) {
	return err && (err.name === 'NotAllowedError' || err.name === 'AbortError');
}

async function post(path, body) {
	return apiFetch(path, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body ?? {})
	});
}

async function errorFrom(res) {
	const data = await res.json().catch(() => ({}));
	return new Error(data.error || res.statusText || 'request failed');
}

// Run a begin -> native ceremony -> finish round trip. `ceremony` is the
// @simplewebauthn function (startRegistration / startAuthentication).
async function run(beginPath, ceremony, finishPath, beginBody) {
	const beginRes = await post(beginPath, beginBody);
	if (!beginRes.ok) throw await errorFrom(beginRes);
	const options = await beginRes.json();
	let credential;
	try {
		credential = await ceremony({ optionsJSON: options.publicKey });
	} catch (err) {
		if (isUserCancel(err)) throw new Error('Passkey prompt was dismissed.');
		throw err;
	}
	const finishRes = await post(finishPath, credential);
	if (!finishRes.ok) throw await errorFrom(finishRes);
	return finishRes.json().catch(() => ({}));
}

// Usernameless (discoverable) login.
export function passkeyLogin() {
	return run('/api/passkey/login/begin', startAuthentication, '/api/passkey/login/finish');
}

// First-run account creation with a passkey.
export function passkeyRegister(username, name) {
	return run('/api/passkey/register/begin', startRegistration, '/api/passkey/register/finish', {
		username,
		name
	});
}

// Enroll an additional passkey for the already-authenticated user.
export function passkeyEnroll(name) {
	return run('/api/passkeys/enroll/begin', startRegistration, '/api/passkeys/enroll/finish', {
		name
	});
}
