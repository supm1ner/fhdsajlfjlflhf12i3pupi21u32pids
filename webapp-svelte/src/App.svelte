<script>
  import { appState } from './lib/stores.svelte.js';
  import Login from './views/Login.svelte';
  import Register from './views/Register.svelte';
  import ChatList from './views/ChatList.svelte';
  import ChatView from './views/ChatView.svelte';
  import Settings from './views/Settings.svelte';
</script>

{#if appState.view === 'login'}
  <Login />
{:else if appState.view === 'register'}
  <Register />
{:else if appState.view === 'chats'}
  <ChatList />
{:else if appState.view === 'chat'}
  <ChatView />
{:else if appState.view === 'settings'}
  <Settings />
{/if}
