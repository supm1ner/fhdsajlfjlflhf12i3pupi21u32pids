<script>
  import Avatar from './Avatar.svelte';
  let { name = 'Unknown', lastMsg = '', time = '', unread = 0, online = false, avatar = '', active = false, onclick } = $props();
</script>

<button class="topic-item" class:active onclick={() => onclick?.()}>
  <div class="ava-wrap">
    <Avatar {name} src={avatar} size={44} />
    {#if online}<span class="online-dot"></span>{/if}
  </div>
  <div class="info">
    <div class="top">
      <span class="name">{name}</span>
      {#if time}<span class="time">{time}</span>{/if}
    </div>
    <div class="bottom">
      <span class="preview">{lastMsg || 'No messages yet'}</span>
      {#if unread > 0}<span class="badge">{unread > 99 ? '99+' : unread}</span>{/if}
    </div>
  </div>
</button>

<style>
  .topic-item { display: flex; align-items: center; gap: 12px; width: 100%; padding: 12px 16px; background: transparent; border: none; cursor: pointer; text-align: left; transition: var(--transition); border-radius: var(--radius-md); }
  .topic-item:hover { background: var(--bg-glass-hover); }
  .topic-item.active { background: var(--accent-soft); }
  .ava-wrap { position: relative; flex-shrink: 0; }
  .online-dot { position: absolute; right: 0; bottom: 0; width: 12px; height: 12px; border-radius: 50%; background: var(--success, #22c55e); border: 2px solid var(--bg-primary, #14141c); }
  .info { flex: 1; min-width: 0; }
  .top { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
  .name { font-size: 14px; font-weight: 500; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .time { font-size: 11px; color: var(--text-tertiary); flex-shrink: 0; }
  .bottom { display: flex; justify-content: space-between; align-items: center; gap: 8px; margin-top: 2px; }
  .preview { font-size: 13px; color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex: 1; }
  .badge { background: var(--accent); color: #fff; font-size: 11px; font-weight: 600; min-width: 18px; height: 18px; border-radius: 9px; display: flex; align-items: center; justify-content: center; padding: 0 5px; flex-shrink: 0; }
</style>
