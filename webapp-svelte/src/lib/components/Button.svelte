<script>
  let { class: className = '', variant = 'primary', fullWidth = false, disabled = false, onclick, children, ...rest } = $props();
</script>

<button
  class="btn btn-{variant} {className}"
  class:full-width={fullWidth}
  {disabled}
  {onclick}
  {...rest}
>
  {#if children}{@render children()}{/if}
</button>

<style>
  .btn {
    display: inline-flex; align-items: center; justify-content: center; gap: 8px;
    padding: 10px 20px; border-radius: var(--radius-sm);
    font-size: 14px; font-weight: 500; cursor: pointer;
    transition: var(--transition); white-space: nowrap;
  }
  .btn:disabled { opacity: 0.4; cursor: not-allowed; }
  .btn-primary {
    background: var(--accent); color: white;
    box-shadow: 0 2px 12px var(--accent-glow);
  }
  .btn-primary:hover:not(:disabled) { background: var(--accent-hover); box-shadow: 0 4px 20px var(--accent-glow); }
  .btn-ghost {
    background: transparent; color: var(--text-secondary);
    border: 1px solid var(--border-glass);
  }
  .btn-ghost:hover:not(:disabled) { background: var(--bg-glass-hover); color: var(--text-primary); }
  .btn-danger { background: var(--danger); color: white; }
  .btn-danger:hover:not(:disabled) { filter: brightness(1.1); }
  .full-width { width: 100%; }
</style>
