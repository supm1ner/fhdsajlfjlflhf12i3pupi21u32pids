<script>
  import { appState } from '../lib/stores.svelte.js';
  import { getClient } from '../lib/tinode.js';
  import GlassPanel from '../lib/components/GlassPanel.svelte';
  import Avatar from '../lib/components/Avatar.svelte';
  import Button from '../lib/components/Button.svelte';

  function goBack() { appState.view = 'chats'; }

  function logout() {
    getClient()?.disconnect?.();
    appState.user = null; appState.chats = []; appState.view = 'login';
  }
</script>

<div class="settings-page">
  <header class="settings-header">
    <button class="back" onclick={goBack}>← Back</button>
    <h2>Settings</h2>
  </header>

  <div class="settings-body">
    <GlassPanel class="profile-card">
      <Avatar name={appState.user?.name || 'U'} size={64} />
      <div class="profile-info">
        <div class="profile-name">{appState.user?.name || 'User'}</div>
        <div class="profile-id">ID: {appState.user?.id || 'N/A'}</div>
      </div>
    </GlassPanel>

    <GlassPanel class="actions-card">
      <h3>Account</h3>
      <Button variant="danger" fullWidth onclick={logout}>Sign Out</Button>
    </GlassPanel>
  </div>
</div>

<style>
  .settings-page { height: 100%; display: flex; flex-direction: column; max-width: 480px; margin: 0 auto; width: 100%; }
  .settings-header { display: flex; align-items: center; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--border-glass); flex-shrink: 0; }
  .back { background: none; border: none; color: var(--text-secondary); font-size: 14px; cursor: pointer; padding: 4px 8px; border-radius: var(--radius-sm); }
  .back:hover { background: var(--bg-glass-hover); color: var(--text-primary); }
  .settings-header h2 { font-size: 16px; font-weight: 600; }
  .settings-body { flex: 1; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 16px; }
</style>
