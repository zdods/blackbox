<script>
	// The single shared toast surface, mounted once in +layout.svelte. Reads the
	// $lib/toast.js store; styling comes from the global .toast classes.
	import { toast } from '$lib/toast.js';
</script>

{#if $toast.show}
	<div class="toast toast-{$toast.type}" role="status" aria-live="polite">{$toast.message}</div>
{/if}
