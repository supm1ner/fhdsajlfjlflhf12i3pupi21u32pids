<script>
  import { onMount } from 'svelte';
  import { appState } from '../lib/stores.svelte.js';
  import { getClient, subscribeMe, getMe, mapContacts, myUID, searchUsers, installSession, setConnectionListener, messagePreview, logout as doLogout } from '../lib/tinode.js';
  import { ensureNotifyPermission, notifyMessage } from '../lib/notify.js';
  import { callState, handleIncoming } from '../lib/calls.svelte.js';
  import { groupCall, handleSignal as handleGroupSignal } from '../lib/groupcall.svelte.js';
  import { liveKit } from '../lib/livekit.svelte.js';
  import GlassPanel from '../lib/components/GlassPanel.svelte';
  import GroupCallPanel from '../lib/components/GroupCallPanel.svelte';
  import LiveKitPanel from '../lib/components/LiveKitPanel.svelte';
  import TopicListItem from '../lib/components/TopicListItem.svelte';
  import Avatar from '../lib/components/Avatar.svelte';
  import Button from '../lib/components/Button.svelte';
  import Conversation from './Conversation.svelte';
  import CallPanel from '../lib/components/CallPanel.svelte';
  import IncomingCall from '../lib/components/IncomingCall.svelte';

  let contacts = $state([]);
  let search = $state('');
  let selected = $state(null); // contact descriptor

  // New-chat search state.
  let showNewChat = $state(false);
  let query = $state('');
  let results = $state([]);
  let searching = $state(false);
  let searchError = $state('');
  let searchTimer = null;

  function refresh() { contacts = mapContacts(); }

  function onQuery(v) {
    query = v;
    clearTimeout(searchTimer);
    if (!v.trim()) { results = []; return; }
    searchTimer = setTimeout(runSearch, 350);
  }

  async function runSearch() {
    searching = true; searchError = '';
    try {
      results = await searchUsers(query);
    } catch (e) {
      searchError = e?.message || 'Search failed';
      results = [];
    } finally { searching = false; }
  }

  function openResult(r) {
    showNewChat = false; query = ''; results = [];
    openChat({ topic: r.topic, name: r.name, online: r.online, lastMsg: '', unread: 0 });
  }

  onMount(async () => {
    try {
      appState.connected = true;
      setConnectionListener((state) => { appState.connected = state === 'online'; });
      installSession();
      const me = await subscribeMe();
      me.onMetaSub = () => refresh();
      me.onSubsUpdated = () => refresh();
      me.onContactUpdate = () => refresh();
      refresh();
      ensureNotifyPermission();

      // Global handler for incoming data messages.
      getClient().onDataMessage = (data) => {
        // Group-call mesh signaling.
        if (data?.head?.mcall) {
          handleGroupSignal(data.head, data.from);
          return;
        }
        // 1:1 incoming call.
        if (data?.head?.webrtc === 'started' && data.from !== myUID()) {
          const peer = contacts.find((c) => c.topic === data.topic);
          handleIncoming(data.topic, data.seq, !!data.head.aonly, peer?.name || data.topic);
          return;
        }
        // Desktop notification for an inbound message (only fires when the tab is backgrounded).
        if (data.from && data.from !== myUID() && !data.head?.webrtc) {
          const peer = contacts.find((c) => c.topic === data.topic);
          const name = peer?.name || data.topic;
          notifyMessage(name, messagePreview({ content: data.content, head: data.head }),
            () => openChat({ topic: data.topic, name, online: peer?.online, lastMsg: '', unread: 0 }));
        }
      };
    } catch (e) {
      console.error('me subscribe failed', e);
    }
  });

  function openChat(contact) {
    selected = contact;
    appState.currentTopic = contact.topic;
  }

  function logout() {
    doLogout();
    appState.user = null;
    appState.currentTopic = null;
    appState.view = 'login';
  }

  let filtered = $derived(
    search
      ? contacts.filter((t) => (t.name || t.topic || '').toLowerCase().includes(search.toLowerCase()))
      : contacts
  );

  let showCallPanel = $derived(callState.active && ['dialing', 'ringing', 'connecting', 'active'].includes(callState.status) && !(callState.direction === 'incoming' && callState.status === 'incoming'));
  let showIncoming = $derived(callState.active && callState.direction === 'incoming' && callState.status === 'incoming');
</script>

<div class="layout">
  <aside class="sidebar">
    <GlassPanel class="user-card">
      <Avatar name={appState.user?.name || 'U'} size={36} src={appState.user?.avatar || ''} />
      <div class="user-info">
        <div class="user-name">{appState.user?.name || 'User'}</div>
        <div class="user-status">{appState.connected ? 'Online' : 'Connecting…'}</div>
      </div>
      <button class="settings-btn" title="Settings" onclick={() => appState.view = 'settings'}>⚙️</button>
    </GlassPanel>

    <div class="search-box">
      <input type="text" placeholder="Search conversations…" bind:value={search} />
      <button class="new-chat-btn" title="New chat" onclick={() => { showNewChat = true; query = ''; results = []; }}>＋</button>
    </div>

    {#if showNewChat}
      <div class="newchat">
        <div class="newchat-head">
          <input
            type="text"
            placeholder="Find people by name or email…"
            value={query}
            oninput={(e) => onQuery(e.target.value)}
          />
          <button class="close" onclick={() => { showNewChat = false; }}>✕</button>
        </div>
        <div class="newchat-results">
          {#if searching}
            <div class="nc-info">Searching…</div>
          {:else if searchError}
            <div class="nc-info err">{searchError}</div>
          {:else if query.trim() && results.length === 0}
            <div class="nc-info">No matches</div>
          {:else}
            {#each results as r (r.topic)}
              <button class="nc-item" onclick={() => openResult(r)}>
                <Avatar name={r.name} size={36} />
                <span class="nc-name">{r.name}</span>
              </button>
            {/each}
          {/if}
        </div>
      </div>
    {/if}

    <div class="topics-list">
      {#if filtered.length === 0}
        <div class="empty"><span class="empty-icon">💬</span><p>No conversations yet</p></div>
      {:else}
        {#each filtered as contact (contact.topic)}
          <TopicListItem
            name={contact.name}
            lastMsg={contact.lastMsg}
            time={contact.lastTs ? new Date(contact.lastTs).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : ''}
            unread={contact.unread}
            online={contact.online}
            avatar={contact.avatar}
            active={selected?.topic === contact.topic}
            onclick={() => openChat(contact)}
          />
        {/each}
      {/if}
    </div>

    <div class="sidebar-footer">
      <Button variant="ghost" onclick={logout}>Sign Out</Button>
    </div>
  </aside>

  <main class="main-content">
    {#if selected}
      {#key selected.topic}
        <Conversation topicName={selected.topic} peerName={selected.name} online={selected.online} />
      {/key}
    {:else}
      <div class="welcome">
        <GlassPanel class="welcome-card">
          <div class="welcome-icon">☀️</div>
          <h2>Welcome to Sunrise</h2>
          <p>Select a conversation to start messaging</p>
        </GlassPanel>
      </div>
    {/if}

    {#if showCallPanel}<CallPanel />{/if}
    {#if showIncoming}<IncomingCall />{/if}
    {#if groupCall.active}<GroupCallPanel />{/if}
    {#if liveKit.active}<LiveKitPanel />{/if}
  </main>
</div>

<style>
  .layout { height: 100%; display: flex; }
  .sidebar { width: 340px; display: flex; flex-direction: column; gap: 8px; padding: 12px; border-right: 1px solid var(--border-glass); flex-shrink: 0; }
  .user-info { flex: 1; min-width: 0; }
  .user-name { font-size: 14px; font-weight: 600; }
  .user-status { font-size: 11px; color: var(--text-tertiary); }
  .settings-btn { background: none; border: none; cursor: pointer; font-size: 18px; }
  .search-box { padding: 0 4px; display: flex; gap: 6px; align-items: center; }
  .search-box input { flex: 1; background: var(--bg-glass); border: 1px solid var(--border-glass); border-radius: var(--radius-sm); padding: 10px 14px; font-size: 13px; color: var(--text-primary); }
  .new-chat-btn { flex-shrink: 0; width: 38px; height: 38px; border-radius: var(--radius-sm); background: var(--accent); color: #fff; font-size: 20px; cursor: pointer; border: none; }
  .new-chat-btn:hover { background: var(--accent-hover); }
  .newchat { margin: 6px 4px 0; border: 1px solid var(--border-glass); border-radius: var(--radius-md); background: var(--bg-card); box-shadow: var(--shadow); overflow: hidden; }
  .newchat-head { display: flex; gap: 6px; padding: 8px; align-items: center; }
  .newchat-head input { flex: 1; background: var(--bg-glass); border: 1px solid var(--border-glass); border-radius: var(--radius-sm); padding: 9px 12px; font-size: 13px; color: var(--text-primary); }
  .newchat-head .close { background: none; border: none; color: var(--text-secondary); font-size: 15px; cursor: pointer; padding: 0 6px; }
  .newchat-results { max-height: 280px; overflow-y: auto; }
  .nc-info { padding: 12px 14px; font-size: 13px; color: var(--text-secondary); }
  .nc-info.err { color: var(--danger); }
  .nc-item { display: flex; align-items: center; gap: 10px; width: 100%; padding: 8px 12px; background: none; border: none; cursor: pointer; text-align: left; }
  .nc-item:hover { background: var(--bg-glass-hover); }
  .nc-name { font-size: 14px; color: var(--text-primary); }
  .search-box input:focus { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }
  .topics-list { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 2px; padding: 4px 0; }
  .empty { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 40px 20px; text-align: center; }
  .empty-icon { font-size: 36px; }
  .empty p { font-size: 14px; color: var(--text-secondary); }
  .sidebar-footer { padding: 8px 4px 0; flex-shrink: 0; }
  .main-content { flex: 1; display: flex; align-items: stretch; justify-content: stretch; position: relative; min-width: 0; }
  .welcome { flex: 1; display: flex; align-items: center; justify-content: center; }
  .welcome-icon { font-size: 48px; text-align: center; }
</style>
