<script>
  import { appState } from '../lib/stores.svelte.js';
  import { connect, createAccount, login, myUID } from '../lib/tinode.js';
  import GlassPanel from '../lib/components/GlassPanel.svelte';
  import Input from '../lib/components/Input.svelte';
  import Button from '../lib/components/Button.svelte';

  let loginVal = $state('');
  let password = $state('');
  let displayName = $state('');
  let email = $state('');
  let error = $state('');
  let loading = $state(false);

  async function handleRegister() {
    if (!loginVal || !password || !displayName) { error = 'Fill in all required fields'; return; }
    error = ''; loading = true;
    try {
      await connect();
      await createAccount(loginVal, password, displayName, email || undefined);
      await login(loginVal, password);
      appState.user = { id: myUID(), name: displayName };
      appState.connected = true;
      appState.view = 'app';
    } catch (e) {
      error = e?.message || 'Registration failed';
    } finally { loading = false; }
  }
</script>

<div class="reg-page">
  <div class="bg-glow"></div>
  <GlassPanel class="reg-card">
    <div class="logo">☀️</div>
    <h1 class="title">Create Account</h1>
    <p class="subtitle">Join cotton Talk</p>

    {#if error}
      <div class="error">{error}</div>
    {/if}

    <Input label="Login" value={loginVal} oninput={(e) => loginVal = e.target.value} placeholder="Choose a login" icon="👤" onenter={handleRegister} />
    <Input label="Password" type="password" value={password} oninput={(e) => password = e.target.value} placeholder="Choose a password" icon="🔒" onenter={handleRegister} />
    <Input label="Display Name" value={displayName} oninput={(e) => displayName = e.target.value} placeholder="Your name" icon="✏️" onenter={handleRegister} />
    <Input label="Email (optional)" type="email" value={email} oninput={(e) => email = e.target.value} placeholder="email@example.com" icon="📧" onenter={handleRegister} />

    <Button fullWidth disabled={loading} onclick={handleRegister}>{loading ? 'Creating...' : 'Create Account'}</Button>

    <p class="switch">
      Already have an account? <button class="link" onclick={() => appState.view = 'login'}>Sign in</button>
    </p>
  </GlassPanel>
</div>

<style>
  .reg-page { height: 100%; display: flex; align-items: center; justify-content: center; position: relative; overflow: hidden; }
  .bg-glow { position: absolute; width: 600px; height: 600px; border-radius: 50%; background: radial-gradient(circle, rgba(139,92,246,0.15), transparent 70%); top: -200px; left: -200px; pointer-events: none; }
  :global(.reg-card) { width: 380px; display: flex; flex-direction: column; gap: 14px; padding: 32px; animation: fadeIn 400ms ease; }
  .logo { font-size: 48px; text-align: center; }
  .title { font-size: 24px; font-weight: 600; text-align: center; }
  .subtitle { font-size: 14px; color: var(--text-secondary); text-align: center; margin-bottom: 8px; }
  .error { font-size: 13px; color: var(--danger); background: rgba(239,68,68,0.1); padding: 8px 12px; border-radius: var(--radius-sm); border: 1px solid rgba(239,68,68,0.2); }
  .switch { font-size: 13px; color: var(--text-secondary); text-align: center; }
  .link { background: none; border: none; color: var(--accent); cursor: pointer; font-size: 13px; font-weight: 500; }
  .link:hover { text-decoration: underline; }
</style>
