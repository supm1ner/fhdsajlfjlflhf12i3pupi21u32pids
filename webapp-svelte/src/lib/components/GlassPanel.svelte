<script>
  let { class: className = '', padding = '24px', children } = $props();
</script>

<div class="glass-panel {className}" style="padding: {padding}">
  {#if children}{@render children()}{/if}
</div>

<style>
  .glass-panel {
    background: var(--bg-glass);
    backdrop-filter: blur(20px) saturate(1.4);
    -webkit-backdrop-filter: blur(20px) saturate(1.4);
    border: 1px solid var(--border-glass);
    border-radius: var(--radius-lg);
    transition: var(--transition);
  }
  .glass-panel:hover { border-color: var(--border-hover); }
</style>
