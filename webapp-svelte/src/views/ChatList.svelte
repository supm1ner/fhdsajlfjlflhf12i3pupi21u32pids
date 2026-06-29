<script>
  import { appState } from '../lib/stores.svelte.js';
  import { getClient } from '../lib/tinode.js';
  import GlassPanel from '../lib/components/GlassPanel.svelte';
  import TopicListItem from '../lib/components/TopicListItem.svelte';
  import Avatar from '../lib/components/Avatar.svelte';
  import Button from '../lib/components/Button.svelte';

  let search = $state('');

  function logout() {
    getClient()?.disconnect?.();
    appState.user = null;
    appState.chats = [];
    appState.view = 'login';
  }

  function openChat(topic) {
    appState.currentChat = topic;
    appState.view = 'chat';
  }

  let filtered = $derived(
    search ? appState.chats.filter(t => (t.public?.fn || t.topic || '').toLowerCase().includes(search.toLowerCase())) : appState.chats
  );
</script>

<div class="layout">
  <aside class="sidebar">
    <GlassPanel class="user-card">
      <Avatar name={appState.user?.name || 'U'} size={36} />
      <div class="user-info">
        <div class="user-name">{appState.user?.name || 'User'}</div>
        <div class="user-status">Online</div>
      </div>
      <button class="settings-btn" onclick={() => appState.view = 'settings'}>⚙️</button>
    </GlassPanel>

    <div class="search-box">
      <input type="text" placeholder="Search conversations..." bind:value={search} />
    </div>

    <div class="topics-list">
      {#if filtered.length === 0}
        <div class="empty">
          <span class="empty-icon">💬</span>
          <p>No conversations yet</p>
        </div>
      {:else}
        {#each filtered as topic}
          <TopicListItem {topic} onclick={openChat} />
        {/each}
      {/if}
    </div>

    <div class="sidebar-footer">
      <Button variant="ghost" onclick={logout}>Sign Out</Button>
    </div>
  </aside>

  <main class="main-content">
    <div class="welcome">
      <GlassPanel class="welcome-card">
        <div class="welcome-icon">☀️</div>
        <h2>Welcome to Sunrise</h2>
        <p>Select a conversation or start a new one</p>
      </GlassPanel>
    </div>
  </main>
</div>

<style>
  .layout { height: 100%; display: flex; }
  .sidebar { width: 340px; display: flex; flex-direction: column; gap: 8px; padding: 12px; border-right: 1px solid var(--border-glass); }
  .search-box { padding: 0 4px; }
  .search-box input { width: 100%; background: var(--bg-glass); border: 1px solid var(--border-glass); border-radius: var(--radius-sm); padding: 10px 14px; font-size: 13px; color: var(--text-primary); }
  .search-box input:focus { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }
  .search-box input::placeholder { color: var(--text-tertiary); }
  .topics-list { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 2px; padding: 4px 0; }
  .empty { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 40px 20px; text-align: center; }
  .empty-icon { font-size: 36px; }
  .empty p { font-size: 14px; color: var(--text-secondary); }
  .sidebar-footer { padding: 8px 4px 0; flex-shrink: 0; }
  .main-content { flex: 1; display: flex; align-items: center; justify-content: center; }
</style>
