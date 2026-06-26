<script>
	// User avatar: the email's Gravatar when one can be derived, otherwise a
	// monogram built from the username. The Gravatar hash is computed async via
	// Web Crypto (see $lib/account.js), so the URL arrives via an {#await}.
	import { gravatarURL } from '$lib/account.js';

	export let email = '';
	export let username = '';
	export let size = 32; // rendered px (the image is fetched at 2x for retina)

	// Recompute whenever the inputs change; the {#await} below tracks this
	// promise directly, so there's no async state-assignment to miss.
	$: urlPromise = gravatarURL(email, size * 2);

	// A failed image load (offline, blocked) falls back to the monogram. Reset
	// the flag whenever the source changes so a new email gets a fresh attempt.
	let failed = false;
	$: if (email || size) failed = false;

	$: initial = (username || '?').trim().charAt(0).toUpperCase() || '?';
</script>

<span class="avatar" style="--avatar-size: {size}px" aria-hidden="true">
	{#await urlPromise then url}
		{#if url && !failed}
			<img src={url} alt="" width={size} height={size} on:error={() => (failed = true)} />
		{:else}
			<span class="avatar__mono">{initial}</span>
		{/if}
	{:catch}
		<span class="avatar__mono">{initial}</span>
	{/await}
</span>

<style>
	.avatar {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: var(--avatar-size);
		height: var(--avatar-size);
		border-radius: 50%;
		overflow: hidden;
		flex-shrink: 0;
		background: var(--accent-soft);
		color: var(--accent);
		border: 1px solid var(--border);
		line-height: 1;
	}
	.avatar img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
	}
	.avatar__mono {
		font-weight: 600;
		font-size: calc(var(--avatar-size) * 0.45);
		text-transform: uppercase;
		user-select: none;
	}
</style>
