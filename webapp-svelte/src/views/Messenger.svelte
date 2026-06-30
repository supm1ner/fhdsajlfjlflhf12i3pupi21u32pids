<script>
  import { onMount } from 'svelte';
  import { appState } from '../lib/stores.svelte.js';
  import { getClient, subscribeMe, getMe, mapContacts, myUID, logout as doLogout } from '../lib/tinode.js';
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

  function refresh() { contacts = mapContacts(); }

  onMount(async () => {
    try {
      const me = await subscribeMe();
      me.onMetaSub = () => refresh();
      me.onSubsUpdated = () => refresh();
      me.onContactUpdate = () => refresh();
      refresh();

      // Global incoming-call detection: an invite arrives as a data message with head.webrtc='started'.
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
    </div>

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
  .search-box { padding: 0 4px; }
  .search-box input { width: 100%; background: var(--bg-glass); border: 1px solid var(--border-glass); border-radius: var(--radius-sm); padding: 10px 14px; font-size: 13px; color: var(--text-primary); }
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
