<script>
	// The blackhaul face — brand mark doubling as a status indicator.
	export let state = 'ok'; // 'ok' | 'loading' | 'error' | 'offline'

	const FACES = {
		ok: '[▪‿▪]',
		loading: '[▪‥▪]',
		error: '[▪︿▪]',
		offline: '[–‿–]'
	};
</script>

<span class="face face-{state}" aria-hidden="true">{FACES[state] ?? FACES.ok}</span>

<style>
	.face {
		font-style: normal;
		white-space: nowrap;
		letter-spacing: -0.04em;
		user-select: none;
	}
	.face-ok {
		color: var(--accent);
	}
	.face-error {
		color: var(--err);
	}
	.face-offline {
		color: var(--text-faint);
	}
	.face-loading {
		color: var(--text-muted);
	}
	@media (prefers-reduced-motion: no-preference) {
		.face-loading {
			animation: face-blink 1.2s steps(2, start) infinite;
		}
	}
	@keyframes face-blink {
		to {
			opacity: 0.35;
		}
	}
</style>
