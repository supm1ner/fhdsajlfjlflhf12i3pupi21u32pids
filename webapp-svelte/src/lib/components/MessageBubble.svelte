<script>
  let { msg, isOwn = false, senderName = '' } = $props();
  let content = $derived(msg.content?.txt || '');
  let time = $derived(new Date(msg.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
  let showName = $derived(!isOwn && senderName);
</script>

<div class="message" class:own={isOwn}>
  {#if showName}
    <div class="sender">{senderName}</div>
  {/if}
  <div class="bubble-wrap">
    <div class="bubble">
      <div class="text">{content}</div>
      <div class="time-label">{time}</div>
    </div>
  </div>
</div>

<style>
  .message { display: flex; flex-direction: column; gap: 2px; margin-bottom: 4px; animation: fadeIn 200ms ease; }
  .message.own { align-items: flex-end; }
  .sender { font-size: 11px; color: var(--accent); font-weight: 500; margin-bottom: 2px; padding: 0 4px; }
  .bubble-wrap { max-width: 75%; }
  .bubble {
    padding: 10px 14px; border-radius: var(--radius-md);
    background: var(--bg-glass); border: 1px solid var(--border-glass);
    position: relative;
  }
  .own .bubble {
    background: var(--accent-soft); border-color: rgba(139, 92, 246, 0.2);
  }
  .text { font-size: 14px; line-height: 1.45; color: var(--text-primary); word-wrap: break-word; }
  .time-label { font-size: 11px; color: var(--text-tertiary); text-align: right; margin-top: 4px; }
</style>
