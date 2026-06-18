<script>
	// Shimmer placeholder. Variants: 'row' (file-list row), 'card' (mobile card /
	// grid tile), 'host' (sidebar host row), 'line' (a single bar). Motion is
	// gated by the shared .skeleton rule in app.css; under reduced motion the
	// sheen is static (a dimmed bar) but layout is identical.
	export let variant = 'row';
	export let count = 1;

	$: items = Array.from({ length: count });
</script>

<div class="sk" role="presentation" aria-hidden="true">
	{#each items as _, i (i)}
		{#if variant === 'row'}
			<div class="sk-row">
				<span class="skeleton sk-row__icon"></span>
				<span class="skeleton sk-row__name"></span>
				<span class="skeleton sk-row__size"></span>
			</div>
		{:else if variant === 'host'}
			<div class="sk-host">
				<span class="skeleton sk-host__dot"></span>
				<span class="sk-host__body">
					<span class="skeleton sk-host__label"></span>
					<span class="skeleton sk-host__meta"></span>
				</span>
			</div>
		{:else if variant === 'card'}
			<div class="sk-card">
				<span class="skeleton sk-card__icon"></span>
				<span class="skeleton sk-card__name"></span>
			</div>
		{:else}
			<span class="skeleton sk-line"></span>
		{/if}
	{/each}
</div>

<style>
	.sk {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		width: 100%;
	}

	.sk-row {
		display: flex;
		align-items: center;
		gap: var(--space-md);
		height: var(--row-height);
	}
	.sk-row__icon {
		width: var(--icon-col);
		height: var(--icon-col);
		border-radius: var(--radius-xs);
		flex-shrink: 0;
	}
	.sk-row__name {
		height: 0.7rem;
		flex: 1;
		max-width: 14rem;
	}
	.sk-row__size {
		height: 0.7rem;
		width: 3rem;
		margin-left: auto;
	}

	.sk-host {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		min-height: var(--rail-row-height);
		padding: var(--space-sm);
	}
	.sk-host__dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}
	.sk-host__body {
		display: flex;
		flex-direction: column;
		gap: 4px;
		flex: 1;
	}
	.sk-host__label {
		height: 0.7rem;
		width: 70%;
	}
	.sk-host__meta {
		height: 0.6rem;
		width: 45%;
	}

	.sk-card {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-md);
	}
	.sk-card__icon {
		width: 2.5rem;
		height: 2.5rem;
		border-radius: var(--radius-sm);
	}
	.sk-card__name {
		height: 0.7rem;
		width: 80%;
	}

	.sk-line {
		height: 0.7rem;
		width: 100%;
	}
</style>
