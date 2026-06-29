<script>
  import { appState } from './lib/stores.svelte.js';
  import Login from './views/Login.svelte';
  import Register from './views/Register.svelte';
  import Messenger from './views/Messenger.svelte';
  import Settings from './views/Settings.svelte';
</script>

{#if appState.view === 'login'}
  <Login />
{:else if appState.view === 'register'}
  <Register />
{:else if appState.view === 'app' || appState.view === 'chats' || appState.view === 'chat'}
  <Messenger />
{:else if appState.view === 'settings'}
  <Settings />
{/if}
