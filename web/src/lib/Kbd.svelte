<script>
	// Tiny keyboard-hint chip. Use the slot for the glyph; `key` is a convenience
	// fallback when no slot content is provided.
	export let key = '';
</script>

<kbd class="kbd"><slot>{key}</slot></kbd>

<style>
	/* Visual styling lives in app.css (.kbd) so chips look identical wherever
	   they appear. This component only fixes the element + scoping. */
	kbd {
		font-family: var(--font-mono);
	}
</style>
