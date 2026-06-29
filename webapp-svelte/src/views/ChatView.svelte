<script>
  import { appState } from '../lib/stores.svelte.js';
  import { publish, subscribe, getClient } from '../lib/tinode.js';
  import GlassPanel from '../lib/components/GlassPanel.svelte';
  import MessageBubble from '../lib/components/MessageBubble.svelte';
  import Button from '../lib/components/Button.svelte';

  let input = $state('');
  let messages = $state([]);
  let loading = $state(true);
  let topicName = $state('');

  $effect(() => {
    const t = appState.currentChat;
    if (!t) return;
    topicName = t.topic;
    loading = true;
    subscribe(t.topic)
      .then(() => getClient().getMessages(t.topic))
      .then((msgs) => { messages = msgs || []; })
      .catch(() => { messages = []; })
      .finally(() => { loading = false; });
  });

  async function send() {
    const t = input.trim();
    if (!t) return;
    input = '';
    try {
      await publish(topicName, t);
    } catch (e) { console.error(e); }
  }

  function goBack() {
    appState.currentChat = null;
    appState.view = 'chats';
  }

  let chatName = $derived(appState.currentChat?.public?.fn || appState.currentChat?.topic || 'Chat');
</script>

<div class="chat-view">
  <header class="chat-header">
    <button class="back" onclick={goBack}>← Back</button>
    <div class="chat-info">
      <div class="chat-name">{chatName}</div>
      <div class="chat-status">{messages.length} messages</div>
    </div>
  </header>

  <div class="messages-area">
    {#if loading}
      <div class="loading">Loading messages...</div>
    {:else if messages.length === 0}
      <div class="empty-msg">
        <span>💬</span>
        <p>No messages yet. Say hello!</p>
      </div>
    {:else}
      {#each messages as msg}
        <MessageBubble msg={msg} isOwn={msg.from === getClient()?.myUID} />
      {/each}
    {/if}
  </div>

  <div class="input-area">
    <div class="input-row">
      <input
        type="text"
        placeholder="Type a message..."
        value={input}
        oninput={(e) => input = e.target.value}
        onkeydown={(e) => e.key === 'Enter' && send()}
      />
      <Button onclick={send} disabled={!input.trim()}>Send</Button>
    </div>
  </div>
</div>

<style>
  .chat-view { height: 100%; display: flex; flex-direction: column; }
  .chat-header { display: flex; align-items: center; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--border-glass); flex-shrink: 0; }
  .back { background: none; border: none; color: var(--text-secondary); font-size: 20px; cursor: pointer; padding: 4px 8px; border-radius: var(--radius-sm); }
  .back:hover { background: var(--bg-glass-hover); color: var(--text-primary); }
  .chat-info { flex: 1; }
  .chat-name { font-size: 15px; font-weight: 600; }
  .chat-status { font-size: 11px; color: var(--text-tertiary); }
  .messages-area { flex: 1; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 4px; }
  .loading, .empty-msg { display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100%; gap: 8px; color: var(--text-secondary); font-size: 14px; }
  .empty-msg span { font-size: 36px; }
  .input-area { padding: 12px 16px; border-top: 1px solid var(--border-glass); flex-shrink: 0; }
  .input-row { display: flex; gap: 8px; }
  .input-row input {
    flex: 1; background: var(--bg-glass); border: 1px solid var(--border-glass);
    border-radius: var(--radius-sm); padding: 10px 14px; font-size: 14px;
    color: var(--text-primary);
  }
  .input-row input:focus { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }
  .input-row input::placeholder { color: var(--text-tertiary); }
</style>
