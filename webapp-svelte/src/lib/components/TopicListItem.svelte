<script>
  import Avatar from './Avatar.svelte';
  let { topic, onclick, active = false } = $props();
  let name = $derived(topic.public?.fn || topic.topic || 'Unknown');
  let lastMsg = $derived(topic.seq > 0 ? topic.lastMsg?.content?.txt || '' : '');
  let time = $derived(topic.lastMsg?.ts ? new Date(topic.lastMsg.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '');
</script>

<button class="topic-item" class:active onclick={() => onclick?.(topic)}>
  <Avatar {name} size={44} />
  <div class="info">
    <div class="top">
      <span class="name">{name}</span>
      {#if time}<span class="time">{time}</span>{/if}
    </div>
    <div class="preview">{lastMsg || 'No messages yet'}</div>
  </div>
</button>

<style>
  .topic-item {
    display: flex; align-items: center; gap: 12px; width: 100%;
    padding: 12px 16px; background: transparent; border: none;
    cursor: pointer; text-align: left; transition: var(--transition); border-radius: var(--radius-md);
  }
  .topic-item:hover { background: var(--bg-glass-hover); }
  .topic-item.active { background: var(--accent-soft); }
  .info { flex: 1; min-width: 0; }
  .top { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
  .name { font-size: 14px; font-weight: 500; color: var(--text-primary); }
  .time { font-size: 11px; color: var(--text-tertiary); flex-shrink: 0; }
  .preview { font-size: 13px; color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-top: 2px; }
</style>
