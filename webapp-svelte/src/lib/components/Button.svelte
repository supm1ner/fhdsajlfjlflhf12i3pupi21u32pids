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
  /* cotton button conventions: mono-ish weight, B/W primary, violet accent. */
  .btn {
    display: inline-flex; align-items: center; justify-content: center; gap: 9px;
    height: 44px; padding: 0 20px; border-radius: var(--radius-sm);
    font-size: 15px; font-weight: 600; line-height: 1; cursor: pointer;
    border: 1px solid transparent;
    transition: background 0.18s var(--ease), border-color 0.18s var(--ease), color 0.18s var(--ease), transform 0.12s var(--ease);
    white-space: nowrap;
  }
  .btn:active:not(:disabled) { transform: translateY(1px); }
  .btn:disabled { opacity: 0.4; cursor: not-allowed; }
  /* primary = high-contrast black/white (cotton .btn-primary) */
  .btn-primary { background: var(--text-primary); color: var(--bg-primary); }
  .btn-primary:hover:not(:disabled) { filter: brightness(1.1); }
  /* accent = violet (cotton .btn-accent) */
  .btn-accent { background: var(--accent); color: #fff; }
  .btn-accent:hover:not(:disabled) { background: var(--accent-hover); }
  .btn-ghost { background: transparent; color: var(--text-primary); border-color: var(--border-hover); }
  .btn-ghost:hover:not(:disabled) { background: var(--bg-glass-hover); border-color: var(--text-tertiary); }
  .btn-danger { background: transparent; color: var(--danger); border-color: var(--border-hover); }
  .btn-danger:hover:not(:disabled) { background: var(--accent-soft); border-color: var(--danger); }
  .full-width { width: 100%; }
</style>
