<script>
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { isLoggedIn, clearLoggedIn, apiFetch } from '$lib/auth.js';
	import { account, loadAccount } from '$lib/account.js';
	import Avatar from '$lib/Avatar.svelte';
	import Face from '$lib/Face.svelte';
	import Skeleton from '$lib/Skeleton.svelte';
	import PasskeysSection from '$lib/PasskeysSection.svelte';

	let loading = true;
	let loadError = '';

	// Profile (email / avatar).
	let email = '';
	let savingEmail = false;
	let profileError = '';

	// Password.
	let currentPassword = '';
	let newPassword = '';
	let confirmPassword = '';
	let savingPassword = false;
	let passwordError = '';
	let showPasswords = false;

	let toast = { show: false, message: '', type: 'success' };
	let toastTimeout = null;

	$: acct = $account;
	$: hasPassword = acct?.has_password === true;
	$: passwordEnabled = acct?.password_enabled === true;
	$: passkeyEnabled = acct?.passkey_enabled === true;
	// Whether the typed email differs from what's saved (enables Save).
	$: emailDirty = email.trim() !== (acct?.email ?? '');

	function handle401(res) {
		if (res.status === 401) {
			clearLoggedIn();
			goto('/login');
			return true;
		}
		return false;
	}

	onMount(async () => {
		if (!isLoggedIn()) {
			goto('/login');
			return;
		}
		try {
			const data = await loadAccount();
			email = data.email ?? '';
		} catch (e) {
			loadError = e?.message || 'Could not load account';
		} finally {
			loading = false;
		}
	});

	onDestroy(() => {
		if (toastTimeout) clearTimeout(toastTimeout);
	});

	function showToast(message, type = 'success', duration = 3000) {
		if (toastTimeout) clearTimeout(toastTimeout);
		toast = { show: true, message, type };
		toastTimeout = setTimeout(() => {
			toast = { ...toast, show: false };
			toastTimeout = null;
		}, duration);
	}

	async function saveEmail(e) {
		e.preventDefault();
		profileError = '';
		savingEmail = true;
		try {
			const res = await apiFetch('/api/account', {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email: email.trim() })
			});
			if (handle401(res)) return;
			if (!res.ok) {
				const data = await res.json().catch(() => ({}));
				throw new Error(data.error || 'Could not save email');
			}
			// Refresh the shared store so the header avatar updates immediately.
			await loadAccount();
			email = $account?.email ?? '';
			showToast(email ? 'email saved' : 'email cleared');
		} catch (err) {
			profileError = err?.message || 'Could not save email';
		} finally {
			savingEmail = false;
		}
	}

	async function changePassword(e) {
		e.preventDefault();
		passwordError = '';
		if (newPassword.length < 8) {
			passwordError = 'New password must be at least 8 characters';
			return;
		}
		if (newPassword !== confirmPassword) {
			passwordError = 'New passwords do not match';
			return;
		}
		savingPassword = true;
		try {
			const res = await apiFetch('/api/account/password', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					current_password: currentPassword,
					new_password: newPassword
				})
			});
			// A 401 from this endpoint is the wrong current password (the session
			// is still valid — it was just re-issued server-side), so show it
			// inline rather than bouncing to /login.
			if (res.status === 401) {
				const data = await res.json().catch(() => ({}));
				throw new Error(data.error || 'Current password is incorrect');
			}
			if (!res.ok) {
				const data = await res.json().catch(() => ({}));
				throw new Error(data.error || 'Could not change password');
			}
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';
			await loadAccount();
			showToast(hasPassword ? 'password changed' : 'password set');
		} catch (err) {
			passwordError = err?.message || 'Could not change password';
		} finally {
			savingPassword = false;
		}
	}
</script>

<header class="page-header">
	<div>
		<h1 class="page-title">account</h1>
		<p class="page-sub">Manage your profile and sign-in credentials.</p>
	</div>
</header>

{#if loading}
	<p class="muted" role="status" aria-live="polite"><Face state="loading" /> loading account…</p>
	<div class="card section-card"><Skeleton variant="line" count={3} /></div>
{:else if loadError}
	<p class="error" role="alert"><Face state="error" /> {loadError}</p>
{:else}
	<!-- Profile -->
	<section class="settings-section" aria-labelledby="profile-heading">
		<h2 class="microlabel" id="profile-heading">profile</h2>
		<div class="card section-card">
			<div class="profile-head">
				<Avatar email={acct?.email ?? ''} username={acct?.username ?? ''} size={64} />
				<div class="profile-id">
					<span class="profile-name">{acct?.username}</span>
					<span class="profile-hint muted">
						{#if acct?.email}
							avatar from <a href="https://gravatar.com" target="_blank" rel="noopener noreferrer"
								>Gravatar</a
							>
						{:else}
							add an email to use a Gravatar avatar
						{/if}
					</span>
				</div>
			</div>

			{#if profileError}
				<p class="error" role="alert"><Face state="error" /> {profileError}</p>
			{/if}

			<form class="settings-form" on:submit={saveEmail}>
				<div class="form-field">
					<label class="field-label" for="email">email</label>
					<input
						id="email"
						name="email"
						type="email"
						autocomplete="email"
						autocapitalize="none"
						spellcheck="false"
						bind:value={email}
						placeholder="you@example.com"
					/>
					<p class="field-note muted">
						Used only to fetch your Gravatar avatar. Leave blank to use a monogram.
					</p>
				</div>
				<div class="form-actions">
					<button type="submit" class="primary" disabled={savingEmail || !emailDirty}>
						{savingEmail ? 'saving…' : 'save'}
					</button>
				</div>
			</form>
		</div>
	</section>

	<!-- Password -->
	{#if passwordEnabled}
		<section class="settings-section" aria-labelledby="password-heading">
			<h2 class="microlabel" id="password-heading">password</h2>
			<div class="card section-card">
				<p class="muted section-lead">
					{#if hasPassword}
						Change the password you use to sign in. Other signed-in sessions are logged out.
					{:else}
						You sign in with a passkey. Set a password to enable password sign-in too.
					{/if}
				</p>

				{#if passwordError}
					<p class="error" role="alert"><Face state="error" /> {passwordError}</p>
				{/if}

				<form class="settings-form" on:submit={changePassword}>
					{#if hasPassword}
						<div class="form-field">
							<label class="field-label" for="current-password">current password</label>
							<input
								id="current-password"
								name="current-password"
								type={showPasswords ? 'text' : 'password'}
								autocomplete="current-password"
								bind:value={currentPassword}
								required
							/>
						</div>
					{/if}
					<div class="form-field">
						<label class="field-label" for="new-password">new password</label>
						<input
							id="new-password"
							name="new-password"
							type={showPasswords ? 'text' : 'password'}
							autocomplete="new-password"
							bind:value={newPassword}
							placeholder="at least 8 characters"
							required
						/>
					</div>
					<div class="form-field">
						<label class="field-label" for="confirm-password">confirm new password</label>
						<input
							id="confirm-password"
							name="confirm-password"
							type={showPasswords ? 'text' : 'password'}
							autocomplete="new-password"
							bind:value={confirmPassword}
							required
						/>
					</div>
					<label class="show-toggle">
						<input type="checkbox" bind:checked={showPasswords} />
						<span>show passwords</span>
					</label>
					<div class="form-actions">
						<button
							type="submit"
							class="primary"
							disabled={savingPassword ||
								!newPassword ||
								!confirmPassword ||
								(hasPassword && !currentPassword)}
						>
							{savingPassword ? 'saving…' : hasPassword ? 'change password' : 'set password'}
						</button>
					</div>
				</form>
			</div>
		</section>
	{/if}

	<!-- Passkeys (only when a relying party is configured) -->
	{#if passkeyEnabled}
		<PasskeysSection />
	{/if}
{/if}

{#if toast.show}
	<div class="toast toast-{toast.type}" role="status" aria-live="polite">
		{toast.message}
	</div>
{/if}

<style>
	.settings-section {
		margin-bottom: var(--space-2xl);
	}
	.section-card {
		margin-top: var(--space-md);
		padding: var(--space-lg);
		display: flex;
		flex-direction: column;
		gap: var(--space-lg);
	}

	.profile-head {
		display: flex;
		align-items: center;
		gap: var(--space-lg);
	}
	.profile-id {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		min-width: 0;
	}
	.profile-name {
		font-weight: 600;
		font-size: var(--fs-md);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.profile-hint {
		font-size: var(--fs-xs);
	}

	.settings-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}
	.form-field {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}
	.field-note {
		margin: 0;
		font-size: var(--fs-xs);
	}
	.section-lead {
		margin: 0;
		font-size: var(--fs-sm);
	}
	.show-toggle {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		font-size: var(--fs-xs);
		color: var(--text-muted);
		cursor: pointer;
	}
	.show-toggle input {
		width: auto;
		height: auto;
		margin: 0;
	}
	.form-actions {
		display: flex;
		justify-content: flex-end;
	}

	@media (max-width: 640px) {
		.form-actions .primary {
			width: 100%;
			min-height: var(--touch-min);
		}
	}
</style>
