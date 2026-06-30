<script>
  let { class: className = '', padding = '24px', children } = $props();
</script>

<div class="glass-panel {className}" style="padding: {padding}">
  {#if children}{@render children()}{/if}
</div>

<style>
  .glass-panel {
    background: var(--bg-card);
    border: 1px solid var(--border-glass);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow);
    transition: var(--transition);
  }
</style>
