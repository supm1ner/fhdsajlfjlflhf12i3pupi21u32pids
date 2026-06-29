<script>
  let { class: className = '', label = '', type = 'text', value = '', placeholder = '', icon = '', onenter, oninput } = $props();

  function handleKeydown(e) {
    if (e.key === 'Enter' && onenter) onenter();
  }
</script>

<div class="input-wrap {className}">
  {#if label}
    <label class="label">{label}</label>
  {/if}
  <div class="input-inner">
    {#if icon}
      <span class="icon">{icon}</span>
    {/if}
    <input {type} {placeholder} value={value} oninput={oninput} onkeydown={handleKeydown} />
  </div>
</div>

<style>
  .input-wrap { display: flex; flex-direction: column; gap: 6px; }
  .label { font-size: 12px; font-weight: 500; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.04em; }
  .input-inner {
    display: flex; align-items: center; gap: 10px;
    background: var(--bg-glass); border: 1px solid var(--border-glass);
    border-radius: var(--radius-sm); padding: 0 14px;
    transition: var(--transition);
  }
  .input-inner:focus-within { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }
  .icon { font-size: 16px; color: var(--text-tertiary); user-select: none; }
  input {
    flex: 1; background: transparent; color: var(--text-primary);
    font-size: 14px; padding: 12px 0; width: 100%;
  }
  input::placeholder { color: var(--text-tertiary); }
</style>
